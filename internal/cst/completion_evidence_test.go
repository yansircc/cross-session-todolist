package cst

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const rationaleJSON = `{"invariant":"proof and attestation stay separate","failure":"schema presence alone does not make rationale recoverable","minimal_fix":"bind machine-checkable evidence and project attestations","remaining_risk":"natural-language rationale remains reviewable","not_doing":"claiming reducer can prove rationale truth"}`

func TestVerifyCompletionRecordsEvidenceSet(t *testing.T) {
	withTempStore(t)
	t.Setenv("CST_ACTOR", "alice")
	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{Parent: 1, Intent: "task", AcceptanceVerify: "true"})
	if err := DoTake(io.Discard, 2, false); err != nil {
		t.Fatal(err)
	}
	if err := DoRunWithArgs(io.Discard, 2, RunArgs{Acceptance: true}, false); err != nil {
		t.Fatal(err)
	}
	runSetID := latestEvidenceID(t, 2, EvidenceAcceptanceRunSet)
	if err := DoEvidence(io.Discard, 2, EvidenceArgs{Kind: EvidenceRationale, Summary: "closure rationale", Data: rationaleJSON}, false); err != nil {
		t.Fatal(err)
	}
	rationaleID := latestEvidenceID(t, 2, EvidenceRationale)
	if err := DoDone(io.Discard, 2, DoneArgs{FromAcceptanceID: runSetID, EvidenceIDs: []string{rationaleID}}, false); err != nil {
		t.Fatal(err)
	}

	task := replayState(t).Nodes[2]
	if !task.Completed {
		t.Fatal("task not completed")
	}
	want := []string{runSetID, rationaleID}
	if !reflect.DeepEqual(task.CompletedEvidenceIDs, want) {
		t.Fatalf("completed evidence ids = %v, want %v", task.CompletedEvidenceIDs, want)
	}
	if task.CompletedEvidence != runSetID {
		t.Fatalf("legacy completed evidence = %q, want %q", task.CompletedEvidence, runSetID)
	}
}

func TestBoundaryEvidenceRejectsFalseExclude(t *testing.T) {
	central := t.TempDir()
	worker := t.TempDir()
	withExplicitStore(t, central)
	t.Setenv("CST_ACTOR", "alice")
	initGitRepo(t, worker)
	if err := os.MkdirAll(filepath.Join(worker, "runtime"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeAndCommit(t, worker, "runtime/ts4094.txt", "clean\n")
	if err := os.WriteFile(filepath.Join(worker, "runtime/ts4094.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{
		Parent:           1,
		Intent:           "task",
		Envelope:         &ExecutionEnvelope{ExecCWD: worker},
		AcceptanceVerify: "true",
	})
	if err := DoTake(io.Discard, 2, false); err != nil {
		t.Fatal(err)
	}
	if err := DoRunWithArgs(io.Discard, 2, RunArgs{Acceptance: true}, false); err != nil {
		t.Fatal(err)
	}
	runSetID := latestEvidenceID(t, 2, EvidenceAcceptanceRunSet)
	if err := DoEvidence(io.Discard, 2, EvidenceArgs{Kind: EvidenceBoundary, Summary: "boundary", Data: `{"excludes":["runtime"]}`}, false); err != nil {
		t.Fatal(err)
	}
	boundaryID := latestEvidenceID(t, 2, EvidenceBoundary)
	err := DoDone(io.Discard, 2, DoneArgs{FromAcceptanceID: runSetID, EvidenceIDs: []string{boundaryID}}, false)
	if err == nil || !strings.Contains(err.Error(), "excludes changed path runtime/ts4094.txt") {
		t.Fatalf("expected false exclude rejection, got %v", err)
	}
}

func TestBoundaryEvidenceIncludesMustCoverChangedPaths(t *testing.T) {
	central := t.TempDir()
	worker := t.TempDir()
	withExplicitStore(t, central)
	t.Setenv("CST_ACTOR", "alice")
	initGitRepo(t, worker)
	if err := os.MkdirAll(filepath.Join(worker, "internal/cst"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeAndCommit(t, worker, "internal/cst/file.go", "clean\n")
	if err := os.WriteFile(filepath.Join(worker, "internal/cst/file.go"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{
		Parent:           1,
		Intent:           "task",
		Envelope:         &ExecutionEnvelope{ExecCWD: worker},
		AcceptanceVerify: "true",
	})
	if err := DoTake(io.Discard, 2, false); err != nil {
		t.Fatal(err)
	}
	if err := DoRunWithArgs(io.Discard, 2, RunArgs{Acceptance: true}, false); err != nil {
		t.Fatal(err)
	}
	runSetID := latestEvidenceID(t, 2, EvidenceAcceptanceRunSet)
	if err := DoEvidence(io.Discard, 2, EvidenceArgs{Kind: EvidenceBoundary, Summary: "boundary", Data: `{"includes":["docs"]}`}, false); err != nil {
		t.Fatal(err)
	}
	boundaryID := latestEvidenceID(t, 2, EvidenceBoundary)
	err := DoDone(io.Discard, 2, DoneArgs{FromAcceptanceID: runSetID, EvidenceIDs: []string{boundaryID}}, false)
	if err == nil || !strings.Contains(err.Error(), "includes do not cover changed path internal/cst/file.go") {
		t.Fatalf("expected include coverage rejection, got %v", err)
	}
}

func TestBoundaryEvidenceRejectsNonRepoRelativePaths(t *testing.T) {
	withTempStore(t)
	t.Setenv("CST_ACTOR", "alice")
	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{Parent: 1, Intent: "task", AcceptanceReview: "self"})
	if err := DoTake(io.Discard, 2, false); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		data string
	}{
		{name: "absolute", data: `{"includes":["/tmp/cst"]}`},
		{name: "escape", data: `{"excludes":["../runtime"]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := DoEvidence(io.Discard, 2, EvidenceArgs{Kind: EvidenceBoundary, Summary: "boundary", Data: tc.data}, false)
			if err == nil || !strings.Contains(err.Error(), "boundary evidence") {
				t.Fatalf("expected boundary path rejection, got %v", err)
			}
		})
	}
}

func TestRationaleEvidenceRejectsVacuousFields(t *testing.T) {
	withTempStore(t)
	t.Setenv("CST_ACTOR", "alice")
	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{Parent: 1, Intent: "task", AcceptanceReview: "self"})
	if err := DoTake(io.Discard, 2, false); err != nil {
		t.Fatal(err)
	}
	err := DoEvidence(io.Discard, 2, EvidenceArgs{
		Kind:    EvidenceRationale,
		Summary: "empty rationale",
		Data:    `{"invariant":"n/a","failure":"none","minimal_fix":"nothing","remaining_risk":"none"}`,
	}, false)
	if err == nil || !strings.Contains(err.Error(), "vacuous") {
		t.Fatalf("expected vacuous rationale rejection, got %v", err)
	}
}

func TestContestEvidenceTargetsRationaleOrBoundary(t *testing.T) {
	withTempStore(t)
	t.Setenv("CST_ACTOR", "alice")
	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{Parent: 1, Intent: "task", AcceptanceReview: "self"})
	if err := DoTake(io.Discard, 2, false); err != nil {
		t.Fatal(err)
	}
	if err := DoEvidence(io.Discard, 2, EvidenceArgs{Kind: EvidenceRationale, Summary: "closure rationale", Data: rationaleJSON}, false); err != nil {
		t.Fatal(err)
	}
	rationaleID := latestEvidenceID(t, 2, EvidenceRationale)
	if err := DoEvidence(io.Discard, 2, EvidenceArgs{
		Kind:    EvidenceContest,
		Summary: "contest rationale",
		Data:    `{"target_evidence_id":"` + rationaleID + `","reason":"rationale omits boundary risk"}`,
	}, false); err != nil {
		t.Fatal(err)
	}
	contestID := latestEvidenceID(t, 2, EvidenceContest)
	if contestID == "" {
		t.Fatal("contest evidence not recorded")
	}
}

func TestClosureEvidenceProjectsInBriefShowAndUI(t *testing.T) {
	withTempStore(t)
	mustDoAdd(t, AddArgs{Intent: "root"})
	mustDoAdd(t, AddArgs{Parent: 1, Goal: true, Intent: "phase"})
	mustDoAdd(t, AddArgs{Parent: 2, Intent: "task", AcceptanceReview: "self"})
	if err := DoEvidence(io.Discard, 3, EvidenceArgs{Kind: EvidenceRationale, Summary: "closure rationale", Data: rationaleJSON}, false); err != nil {
		t.Fatal(err)
	}
	rationaleID := latestEvidenceID(t, 3, EvidenceRationale)
	if err := DoEvidence(io.Discard, 3, EvidenceArgs{
		Kind:    EvidenceContest,
		Summary: "contest rationale",
		Data:    `{"target_evidence_id":"` + rationaleID + `","reason":"rationale misses negative space"}`,
	}, false); err != nil {
		t.Fatal(err)
	}

	state := replayState(t)
	brief, err := BuildBrief(state, DefaultConfig(), "agent", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(brief.Ready) != 1 || brief.Ready[0].Closure == nil || len(brief.Ready[0].Closure.Rationale) != 1 {
		t.Fatalf("brief missing closure projection: %+v", brief.Ready)
	}
	if brief.Ready[0].Closure.Rationale[0].Contested == nil {
		t.Fatalf("brief missing contested state: %+v", brief.Ready[0].Closure)
	}

	show, err := BuildShow(state, 3, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if show.Closure == nil || len(show.Closure.Rationale) != 1 || show.Closure.Rationale[0].Contested == nil {
		t.Fatalf("show missing closure projection: %+v", show.Closure)
	}

	ui := uiViewFrom(state, 0, EventsPath(), "sample", 0, state.Nodes[1].LastEvent)
	html := renderHTML(ui)
	for _, want := range []string{
		`closure: rationale=1 contested=1`,
		`closure evidence: rationale · closure rationale · contested`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("html missing %q\n%s", want, head(html, 3000))
		}
	}
}
