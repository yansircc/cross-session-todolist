package cst

import (
	"io"
	"strings"
	"testing"
)

func TestArchiveRequiresTerminalSubtreeAndIsReversible(t *testing.T) {
	withTempStore(t)
	if err := DoAdd(io.Discard, AddArgs{Intent: "root"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 1, Goal: true, Intent: "old stream"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 2, Intent: "old task", AcceptanceReview: "self"}, false); err != nil {
		t.Fatal(err)
	}

	err := WithStore(TxOpts{Mutating: true}, func(tx *Tx) error {
		_, err := tx.ArchiveNode(2, "fold old stream")
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "active descendant task #3") {
		t.Fatalf("archive of active subtree should fail, got %v", err)
	}

	if err := DoTake(io.Discard, 3, false); err != nil {
		t.Fatal(err)
	}
	if err := DoDone(io.Discard, 3, DoneArgs{Note: "reviewed"}, false); err != nil {
		t.Fatal(err)
	}

	if err := WithStore(TxOpts{Mutating: true}, func(tx *Tx) error {
		_, err := tx.ArchiveNode(2, "fold old stream")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	state := replayState(t)
	if !state.Nodes[2].Archived || !state.IsArchived(3) {
		t.Fatalf("archive marker should apply to subtree: node2=%+v childArchived=%v", state.Nodes[2], state.IsArchived(3))
	}
	if state.Nodes[3].CompletedEvidence == "" {
		t.Fatalf("archive must not clear completion evidence: %+v", state.Nodes[3])
	}

	if err := WithStore(TxOpts{Mutating: true}, func(tx *Tx) error {
		_, err := tx.UnarchiveNode(2, "inspect old stream")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	state = replayState(t)
	if state.Nodes[2].Archived || state.IsArchived(3) {
		t.Fatalf("unarchive should only clear visibility marker: node2=%+v childArchived=%v", state.Nodes[2], state.IsArchived(3))
	}
}

func TestArchiveRejectsRootAndOpenTask(t *testing.T) {
	withTempStore(t)
	if err := DoAdd(io.Discard, AddArgs{Intent: "root"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 1, Intent: "open task", AcceptanceReview: "self"}, false); err != nil {
		t.Fatal(err)
	}

	err := WithStore(TxOpts{Mutating: true}, func(tx *Tx) error {
		_, err := tx.ArchiveNode(1, "fold root")
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "cannot archive root") {
		t.Fatalf("archive root should fail, got %v", err)
	}

	err = WithStore(TxOpts{Mutating: true}, func(tx *Tx) error {
		_, err := tx.ArchiveNode(2, "fold task")
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "not terminal") {
		t.Fatalf("archive open task should fail, got %v", err)
	}
}

func TestRulePromotionCreatesNormalAncestorRuleWithOrigin(t *testing.T) {
	withTempStore(t)
	if err := DoAdd(io.Discard, AddArgs{Intent: "root"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 1, Goal: true, Intent: "old stream"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 2, Rule: "future work keeps one fact in one location"}, false); err != nil {
		t.Fatal(err)
	}

	var promoted *Event
	if err := WithStore(TxOpts{Mutating: true}, func(tx *Tx) error {
		ev, err := tx.PromoteRule(3, 1, "still applies to future work")
		promoted = ev
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if promoted == nil || promoted.Type != EvNodeCreated || promoted.Kind != KindRule {
		t.Fatalf("promotion should create a normal rule node, got %+v", promoted)
	}

	state := replayState(t)
	rule := state.Nodes[promoted.NodeID]
	if rule == nil || rule.Kind != KindRule || rule.ParentID != 1 || rule.RuleText != state.Nodes[3].RuleText {
		t.Fatalf("promoted rule shape wrong: %+v", rule)
	}
	if rule.RuleOrigin == nil || rule.RuleOrigin.SourceRuleID != 3 || rule.RuleOrigin.Reason == "" {
		t.Fatalf("promoted rule missing origin: %+v", rule.RuleOrigin)
	}

	if err := DoAdd(io.Discard, AddArgs{Parent: 1, Goal: true, Intent: "future stream"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 5, Intent: "future task", AcceptanceReview: "self"}, false); err != nil {
		t.Fatal(err)
	}
	rules := replayState(t).InheritedRules(6)
	if len(rules) != 1 || rules[0].ID != promoted.NodeID {
		t.Fatalf("future sibling should inherit only the promoted ancestor rule, got %+v", rules)
	}
}

func TestRulePromotionRejectsNonAncestorTarget(t *testing.T) {
	withTempStore(t)
	if err := DoAdd(io.Discard, AddArgs{Intent: "root"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 1, Goal: true, Intent: "stream a"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 1, Goal: true, Intent: "stream b"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{Parent: 2, Rule: "local rule"}, false); err != nil {
		t.Fatal(err)
	}

	err := WithStore(TxOpts{Mutating: true}, func(tx *Tx) error {
		_, err := tx.PromoteRule(4, 3, "wrong branch")
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "not an ancestor") {
		t.Fatalf("promotion to sibling branch should fail, got %v", err)
	}
}
