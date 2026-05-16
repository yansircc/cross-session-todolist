package cst

import (
	"os"
	"testing"
)

// withTempStore swaps cwd to a fresh temp directory for the duration of the
// subtest so the store paths resolve into isolation.
func withTempStore(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
	return dir
}

func applyEvents(t *testing.T, state *State, events ...*Event) {
	t.Helper()
	for _, ev := range events {
		if err := state.applyOne(ev); err != nil {
			t.Fatalf("apply %s: %v", ev.Type, err)
		}
	}
}

func replayEvents(t *testing.T) []*Event {
	t.Helper()
	events, err := Replay()
	if err != nil {
		t.Fatal(err)
	}
	return events
}

func replayState(t *testing.T) *State {
	t.Helper()
	state, err := Apply(replayEvents(t))
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func containsID(ids []int64, want int64) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
