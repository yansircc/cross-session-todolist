package cst

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBoundaryPartitionRejectsChildOutsideParent(t *testing.T) {
	now := time.Now()
	events := []*Event{
		{EventID: "root", Timestamp: now, Actor: "a", Type: EvNodeCreated,
			NodeID: 1, Kind: KindGoal, Intent: "root", Boundary: &NodeBoundary{Owned: []string{"src"}}},
		{EventID: "task", Timestamp: now, Actor: "a", Type: EvNodeCreated,
			NodeID: 2, ParentID: 1, Kind: KindTask, Intent: "task", Acceptance: &Acceptance{Kind: AcceptanceReview, Who: "self"}, Boundary: &NodeBoundary{Owned: []string{"docs"}}},
	}
	_, err := Apply(events)
	if err == nil || !strings.Contains(err.Error(), "outside parent") {
		t.Fatalf("expected child boundary subset rejection, got %v", err)
	}
}

func TestBoundaryPartitionRejectsSiblingOverlap(t *testing.T) {
	now := time.Now()
	events := []*Event{
		{EventID: "root", Timestamp: now, Actor: "a", Type: EvNodeCreated,
			NodeID: 1, Kind: KindGoal, Intent: "root", Boundary: &NodeBoundary{Owned: []string{"."}}},
		{EventID: "a", Timestamp: now, Actor: "a", Type: EvNodeCreated,
			NodeID: 2, ParentID: 1, Kind: KindTask, Intent: "a", Acceptance: &Acceptance{Kind: AcceptanceReview, Who: "self"}, Boundary: &NodeBoundary{Owned: []string{"src"}}},
		{EventID: "b", Timestamp: now, Actor: "a", Type: EvNodeCreated,
			NodeID: 3, ParentID: 1, Kind: KindTask, Intent: "b", Acceptance: &Acceptance{Kind: AcceptanceReview, Who: "self"}, Boundary: &NodeBoundary{Owned: []string{"src/lib"}}},
	}
	_, err := Apply(events)
	if err == nil || !strings.Contains(err.Error(), "overlapping owned boundaries") {
		t.Fatalf("expected sibling overlap rejection, got %v", err)
	}
}

func TestDoneRejectsNodeBoundaryOwnedViolation(t *testing.T) {
	dir := withTempStore(t)
	initGitRepo(t, dir)
	writeAndCommit(t, dir, "other.txt", "clean\n")
	if err := DoAdd(io.Discard, AddArgs{Intent: "root"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{
		Parent:           1,
		Intent:           "boundary task",
		AcceptanceVerify: "printf dirty >> other.txt",
		Boundary:         &NodeBoundary{Owned: []string{"owned"}},
	}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoTake(io.Discard, 2, false); err != nil {
		t.Fatal(err)
	}
	err := DoDone(io.Discard, 2, DoneArgs{}, false)
	if err == nil || !strings.Contains(err.Error(), "do not cover changed path other.txt") {
		t.Fatalf("expected owned boundary completion rejection, got %v", err)
	}
}

func TestDoneRejectsNodeBoundaryExcludedViolation(t *testing.T) {
	dir := withTempStore(t)
	initGitRepo(t, dir)
	if err := os.MkdirAll(filepath.Join(dir, "owned", "skip"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeAndCommit(t, dir, filepath.Join("owned", "skip", "file.txt"), "clean\n")
	if err := DoAdd(io.Discard, AddArgs{Intent: "root"}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoAdd(io.Discard, AddArgs{
		Parent:           1,
		Intent:           "boundary task",
		AcceptanceVerify: "printf dirty >> owned/skip/file.txt",
		Boundary:         &NodeBoundary{Owned: []string{"owned"}, Excluded: []string{"owned/skip"}},
	}, false); err != nil {
		t.Fatal(err)
	}
	if err := DoTake(io.Discard, 2, false); err != nil {
		t.Fatal(err)
	}
	err := DoDone(io.Discard, 2, DoneArgs{}, false)
	if err == nil || !strings.Contains(err.Error(), "excludes changed path owned/skip/file.txt") {
		t.Fatalf("expected excluded boundary completion rejection, got %v", err)
	}
}
