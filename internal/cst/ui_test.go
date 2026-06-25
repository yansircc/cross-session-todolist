package cst

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderHTML_BasicStructure(t *testing.T) {
	withTempStore(t)
	buildSampleStore(t)

	state := replayState(t)
	v := uiViewFrom(state, 0, EventsPath(), "sample", 0, state.Nodes[1].LastEvent)
	html := renderHTML(v)

	wantSubstrings := []string{
		`<!doctype html>`,
		`CST Next Briefing`,
		`id="phase-2"`,
		`class="steps"`,
		`type="radio" name="phase-2-task"`,
		`#phase-2-task-6:checked ~ .phase-head label`,
		`rules of the phase`,
		`Pending task`,
		`Finished task`,
		`Stuck task`,
		`block reason text`,
		`did the thing`,
		`checks <span class="count">1</span>`,
		`evidence <span class="count">1</span>`,
		`rules <span class="count">1</span>`,
		`edges <span class="count">0</span>`,
		`<style>`,
		`color-scheme:light dark`,
		`@media (prefers-color-scheme:dark)`,
		`--accent-soft:#172554`,
		`--step-empty:#e5e7eb`,
		`--step-empty:#263241`,
		`--step-selected:#93c5fd`,
		`background:var(--step-empty)`,
		`background:var(--step-done)`,
		`background:var(--step-now)`,
		`background:var(--step-hover)`,
		`background:var(--step-selected);transform:scale(1.25)`,
		`background:var(--success-soft)`,
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(html, want) {
			t.Errorf("html missing %q\n--- generated ---\n%s", want, head(html, 2000))
		}
	}
	if strings.Contains(html, "<script") {
		t.Errorf("html contains unexpected <script> tag")
	}
	if strings.Contains(html, `background:var(--ink);transform:scale`) {
		t.Errorf("selected progress step must not derive fill color from text color")
	}
	for _, unwanted := range []string{`<aside`, `<table`, `Recent Evidence`, `Progress Equation`, `worker-status`, `cst events --attempt`, `cmd-pill`} {
		if strings.Contains(html, unwanted) {
			t.Errorf("html contains removed dashboard surface %q\n--- generated ---\n%s", unwanted, head(html, 3000))
		}
	}
}

func TestRenderHTML_AttemptNamedChecksAndInheritedRules(t *testing.T) {
	withTempStore(t)
	t.Setenv("CST_ACTOR", "alice")
	mustDoAdd(t, AddArgs{Intent: "Root"})
	mustDoAdd(t, AddArgs{Parent: 1, Rule: "root invariant"})
	mustDoAdd(t, AddArgs{Parent: 1, Goal: true, Intent: "Phase"})
	mustDoAdd(t, AddArgs{
		Parent: 3,
		Intent: "Checked task",
		VerifyChecks: []VerifyCheck{
			{Name: "unit", Cmd: "true"},
			{Name: "lint", Cmd: "false"},
		},
	})
	mustDoTake(t, 4)
	mustDoRun(t, 4, "unit", false)
	mustDoRun(t, 4, "lint", true)

	state := replayState(t)
	attemptID := state.Nodes[4].Claim.AttemptID
	if attemptID == "" {
		t.Fatal("claim did not mint attempt_id")
	}
	v := uiViewFrom(state, 0, EventsPath(), "sample", 0, state.Nodes[1].LastEvent)
	html := renderHTML(v)

	wantSubstrings := []string{
		`CST Next Briefing`,
		`<span class="tag active">run acceptance</span>`,
		`#4 Checked task`,
		`root invariant`,
		`unit`,
		`lint`,
		`checks <span class="count">2</span>`,
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(html, want) {
			t.Errorf("html missing %q\n--- generated ---\n%s", want, head(html, 3000))
		}
	}
	if strings.Contains(html, "次重试") {
		t.Errorf("named checks must not be labeled as retries")
	}
	for _, unwanted := range []string{`Recent Evidence`, `latest run:`, `cst show 4`, `cst worker-status`, `cst events --attempt`, attemptID} {
		if strings.Contains(html, unwanted) {
			t.Errorf("html leaked execution trivia %q\n--- generated ---\n%s", unwanted, head(html, 3000))
		}
	}
}

func TestRenderHTML_AfterDependencyIsWaitingNotReady(t *testing.T) {
	withTempStore(t)
	mustDoAdd(t, AddArgs{Intent: "Root"})
	mustDoAdd(t, AddArgs{Parent: 1, Goal: true, Intent: "Phase"})
	mustDoAdd(t, AddArgs{Parent: 2, Intent: "Prerequisite task", AcceptanceVerify: "true"})
	mustDoAdd(t, AddArgs{Parent: 2, Intent: "Dependent task", AcceptanceVerify: "true", After: []int64{3}})

	state := replayState(t)
	if state.IsReadyTask(4) {
		t.Fatal("dependent task should not be ready before prerequisite completes")
	}
	v := uiViewFrom(state, 0, EventsPath(), "sample", 0, state.Nodes[1].LastEvent)
	html := renderHTML(v)
	article := htmlArticleContaining(t, html, "Dependent task")

	wantSubstrings := []string{
		`after=3`,
		`edges <span class="count">1</span>`,
		`<span>from</span><span>3</span>`,
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(article, want) {
			t.Errorf("dependent article missing %q\n--- article ---\n%s", want, article)
		}
	}
	if strings.Contains(article, `legal take action`) || strings.Contains(article, `ready`) {
		t.Errorf("dependent article was projected as ready\n--- article ---\n%s", article)
	}
	if strings.Contains(html, "待领取") {
		t.Errorf("html must not use raw-open ready wording")
	}
}

func TestRenderHTML_ReviewReadyIsNotDoubleCountedAsReady(t *testing.T) {
	withTempStore(t)
	mustDoAdd(t, AddArgs{Intent: "Root"})
	mustDoAdd(t, AddArgs{Parent: 1, Goal: true, Intent: "Phase"})
	mustDoAdd(t, AddArgs{Parent: 2, Intent: "Review task", AcceptanceReview: "self"})

	state := replayState(t)
	if !state.IsReadyTask(3) {
		t.Fatal("review task should be ready")
	}
	v := uiViewFrom(state, 0, EventsPath(), "sample", 0, state.Nodes[1].LastEvent)
	html := renderHTML(v)
	article := htmlArticleContaining(t, html, `#3 Review task`)

	if !strings.Contains(article, `<div class="state-line review">review ready</div>`) {
		t.Errorf("review article missing review-ready state\n--- article ---\n%s", article)
	}
	if !strings.Contains(article, `<span class="check">review</span>`) {
		t.Errorf("review article missing review check\n--- article ---\n%s", article)
	}
	if !strings.Contains(html, `<span class="tag active">review</span>`) {
		t.Errorf("top procedure action should be review\n--- html ---\n%s", head(html, 3000))
	}
	if strings.Contains(html, `<span class="badge ready">1 ready</span>`) {
		t.Errorf("review-ready task was double-counted as ready")
	}
}

func TestRenderHTML_PhasesUseLedgerOrderNotLastActivity(t *testing.T) {
	withTempStore(t)
	mustDoAdd(t, AddArgs{Intent: "Root"})
	mustDoAdd(t, AddArgs{Parent: 1, Goal: true, Intent: "Phase 1"})
	mustDoAdd(t, AddArgs{Parent: 1, Goal: true, Intent: "Phase 2"})
	mustDoAdd(t, AddArgs{Parent: 2, Intent: "Phase 1 task", AcceptanceVerify: "true"})
	mustDoAdd(t, AddArgs{Parent: 3, Intent: "Phase 2 task", AcceptanceVerify: "true"})

	state := replayState(t)
	v := uiViewFrom(state, 0, EventsPath(), "sample", 0, state.Nodes[1].LastEvent)
	if len(v.ActivePhases) != 2 {
		t.Fatalf("expected 2 active phases, got %d", len(v.ActivePhases))
	}
	if v.ActivePhases[0].Node.ID != 2 || v.ActivePhases[1].Node.ID != 3 {
		t.Fatalf("phases should follow ledger order, got #%d then #%d", v.ActivePhases[0].Node.ID, v.ActivePhases[1].Node.ID)
	}

	html := renderHTML(v)
	first := strings.Index(html, `id="phase-2"`)
	second := strings.Index(html, `id="phase-3"`)
	if first < 0 || second < 0 || first > second {
		t.Errorf("rendered phases should follow ledger order\n%s", head(html, 3000))
	}
}

func TestRenderHTML_ProgressStepsUseTaskOrderNotActionPriority(t *testing.T) {
	withTempStore(t)
	mustDoAdd(t, AddArgs{Intent: "Root"})
	mustDoAdd(t, AddArgs{Parent: 1, Goal: true, Intent: "Phase"})
	mustDoAdd(t, AddArgs{Parent: 2, Intent: "Done first", AcceptanceVerify: "true"})
	mustDoAdd(t, AddArgs{Parent: 2, Intent: "Done second", AcceptanceVerify: "true"})
	mustDoAdd(t, AddArgs{Parent: 2, Intent: "Ready now", AcceptanceVerify: "true"})

	mustDoTake(t, 3)
	mustDoDone(t, 3)
	mustDoTake(t, 4)
	mustDoDone(t, 4)

	state := replayState(t)
	v := uiViewFrom(state, 0, EventsPath(), "sample", 0, state.Nodes[1].LastEvent)
	html := renderHTML(v)
	steps := htmlStepsForPhase(t, html, 2)

	first := strings.Index(steps, `for="phase-2-task-3"`)
	second := strings.Index(steps, `for="phase-2-task-4"`)
	third := strings.Index(steps, `for="phase-2-task-5"`)
	if first < 0 || second < 0 || third < 0 || !(first < second && second < third) {
		t.Fatalf("progress steps must follow task order, got\n%s", steps)
	}
	if !strings.Contains(html, `<input class="task-radio" type="radio" name="phase-2-task" id="phase-2-task-5" checked>`) {
		t.Fatalf("default selected detail should still be action-priority ready task\n%s", head(html, 3000))
	}
}

func TestRenderHTML_EscapesIntent(t *testing.T) {
	withTempStore(t)
	// Root goal with a script tag in its intent
	mustDoAdd(t, AddArgs{Intent: "Project <script>alert(1)</script>"})
	// Goal + open task to ensure a scope card renders
	mustDoAdd(t, AddArgs{Parent: 1, Goal: true, Intent: "Phase <script>alert(2)</script>"})
	mustDoAdd(t, AddArgs{Parent: 2, Intent: "Task <img onerror=x>", AcceptanceVerify: "true"})

	state := replayState(t)
	v := uiViewFrom(state, 0, EventsPath(), "sample", 0, state.Nodes[1].LastEvent)
	html := renderHTML(v)

	if strings.Contains(html, "<script>alert(1)</script>") {
		t.Errorf("raw <script> intent leaked into html")
	}
	if !strings.Contains(html, "&lt;script&gt;alert(2)&lt;/script&gt;") {
		t.Errorf("expected escaped script tag in html")
	}
	if strings.Contains(html, "<img onerror=x>") {
		t.Errorf("raw <img> intent leaked into html")
	}
}

func TestRenderHTML_EmptyState(t *testing.T) {
	withTempStore(t)
	mustDoAdd(t, AddArgs{Intent: "Lonely goal"})
	state := replayState(t)
	v := uiViewFrom(state, 0, EventsPath(), "sample", 0, state.Nodes[1].LastEvent)
	html := renderHTML(v)
	if !strings.Contains(html, "所有 goal 已收敛") {
		t.Errorf("empty state message missing\n%s", head(html, 1500))
	}
}

func TestDoUI_WritesFile(t *testing.T) {
	withTempStore(t)
	buildSampleStore(t)

	out := &bytes.Buffer{}
	err := DoUI(out, UIArgs{NoOpen: true}, true)
	if err != nil {
		t.Fatalf("DoUI: %v", err)
	}

	// stdout should be JSON metadata
	var meta uiMeta
	if err := json.Unmarshal(out.Bytes(), &meta); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out.String())
	}
	if !strings.HasSuffix(meta.Output, filepath.Join(".cst", "ui.html")) {
		t.Errorf("output path unexpected: %s", meta.Output)
	}
	if meta.ActiveScopes == 0 {
		t.Errorf("expected at least one active scope")
	}

	body, err := os.ReadFile(meta.Output)
	if err != nil {
		t.Fatalf("read %s: %v", meta.Output, err)
	}
	if !bytes.HasPrefix(body, []byte("<!doctype html>")) {
		t.Errorf("file is not html: %s", head(string(body), 200))
	}
}

func TestDoUI_Stdout(t *testing.T) {
	withTempStore(t)
	buildSampleStore(t)

	out := &bytes.Buffer{}
	err := DoUI(out, UIArgs{Stdout: true, NoOpen: true}, true)
	if err != nil {
		t.Fatalf("DoUI: %v", err)
	}
	if !bytes.HasPrefix(out.Bytes(), []byte("<!doctype html>")) {
		t.Errorf("stdout is not html: %s", head(out.String(), 200))
	}
	// File must not exist when --stdout
	if _, err := os.Stat(filepath.Join(StoreDir(), "ui.html")); !os.IsNotExist(err) {
		t.Errorf("expected no file when --stdout, got %v", err)
	}
}

func TestDoUI_ProjectNameUsesConfiguredStoreRoot(t *testing.T) {
	cwdStore := withTempStore(t)
	otherStore := filepath.Join(t.TempDir(), "agentOS")
	if err := SetStoreRoot(otherStore); err != nil {
		t.Fatalf("SetStoreRoot: %v", err)
	}
	t.Cleanup(func() {
		if err := SetStoreRoot(""); err != nil {
			t.Fatalf("reset store root: %v", err)
		}
	})
	mustDoAdd(t, AddArgs{Intent: "Root"})
	mustDoAdd(t, AddArgs{Parent: 1, Goal: true, Intent: "Phase"})
	mustDoAdd(t, AddArgs{Parent: 2, Intent: "Task", AcceptanceVerify: "true"})

	out := &bytes.Buffer{}
	err := DoUI(out, UIArgs{Stdout: true, NoOpen: true}, true)
	if err != nil {
		t.Fatalf("DoUI: %v", err)
	}
	html := out.String()
	if !strings.Contains(html, `<h1>agentOS CST</h1>`) {
		t.Errorf("html did not use configured store root name\n%s", head(html, 1500))
	}
	if strings.Contains(html, `<h1>`+filepath.Base(cwdStore)+` CST</h1>`) {
		t.Errorf("html used cwd store name instead of configured store root")
	}
}

func TestDoUI_UnknownScope(t *testing.T) {
	withTempStore(t)
	mustDoAdd(t, AddArgs{Intent: "root"})
	out := &bytes.Buffer{}
	err := DoUI(out, UIArgs{Within: 999, NoOpen: true}, true)
	if err == nil {
		t.Fatal("expected error for unknown scope")
	}
	var he *HandlerError
	if !errorsAs(err, &he) {
		t.Fatalf("expected HandlerError, got %T", err)
	}
	if he.Code != ExitNotFound {
		t.Errorf("expected ExitNotFound, got %d", he.Code)
	}
}

func TestInnermostGoal(t *testing.T) {
	withTempStore(t)
	mustDoAdd(t, AddArgs{Intent: "root"})                                       // #1 goal
	mustDoAdd(t, AddArgs{Parent: 1, Goal: true, Intent: "phase"})               // #2 goal
	mustDoAdd(t, AddArgs{Parent: 2, Intent: "outer", AcceptanceVerify: "true"}) // #3 task
	mustDoAdd(t, AddArgs{Parent: 3, Intent: "inner", AcceptanceVerify: "true"}) // #4 sub-task

	state := replayState(t)
	g := innermostGoal(state, 4) // sub-task's innermost goal should be phase #2
	if g == nil || g.ID != 2 {
		t.Errorf("expected innermost goal #2, got %v", g)
	}
	g = innermostGoal(state, 0) // walking from no parent yields nil
	if g != nil {
		t.Errorf("expected nil when starting from 0, got #%d", g.ID)
	}
}

// buildSampleStore constructs:
//
//	#1 goal: root
//	#2 goal: phase (active scope)
//	#3 task: open verify task
//	#4 task: completed verify task
//	#5 rule: scope rule
//	#6 task: held task
func buildSampleStore(t *testing.T) {
	t.Helper()
	mustDoAdd(t, AddArgs{Intent: "Root"})
	mustDoAdd(t, AddArgs{Parent: 1, Goal: true, Intent: "Phase"})
	mustDoAdd(t, AddArgs{Parent: 2, Intent: "Pending task", AcceptanceVerify: "true"})
	mustDoAdd(t, AddArgs{Parent: 2, Intent: "Finished task", AcceptanceVerify: "true"})
	mustDoAdd(t, AddArgs{Parent: 2, Rule: "rules of the phase"})
	mustDoAdd(t, AddArgs{Parent: 2, Intent: "Stuck task", AcceptanceVerify: "true"})

	// Finish #4
	mustDoTake(t, 4)
	// Record evidence (note) so the completed task has Agent note
	mustDoEvidence(t, 4, EvidenceArgs{Kind: "note", Summary: "did the thing"})
	mustDoDone(t, 4)

	// Hold #6
	mustDoHold(t, 6, "waiting", "block reason text")
}

func mustDoAdd(t *testing.T, args AddArgs) {
	t.Helper()
	if err := DoAdd(discardWriter{}, args, true); err != nil {
		t.Fatalf("DoAdd: %v", err)
	}
}

func mustDoTake(t *testing.T, id int64) {
	t.Helper()
	if err := DoTake(discardWriter{}, id, true); err != nil {
		t.Fatalf("DoTake #%d: %v", id, err)
	}
}

func mustDoEvidence(t *testing.T, id int64, args EvidenceArgs) {
	t.Helper()
	if err := DoEvidence(discardWriter{}, id, args, true); err != nil {
		t.Fatalf("DoEvidence #%d: %v", id, err)
	}
}

func mustDoRun(t *testing.T, id int64, checkName string, wantFail bool) {
	t.Helper()
	err := DoRun(io.Discard, id, "", checkName, false)
	if wantFail {
		var he *HandlerError
		if err == nil || !errorsAs(err, &he) || he.Code != ExitAcceptanceFail {
			t.Fatalf("DoRun #%d check=%s: expected acceptance failure, got %v", id, checkName, err)
		}
		return
	}
	if err != nil {
		t.Fatalf("DoRun #%d check=%s: %v", id, checkName, err)
	}
}

func mustDoDone(t *testing.T, id int64) {
	t.Helper()
	if err := DoDone(discardWriter{}, id, DoneArgs{}, true); err != nil {
		t.Fatalf("DoDone #%d: %v", id, err)
	}
}

func mustDoHold(t *testing.T, id int64, kind, reason string) {
	t.Helper()
	if err := DoHold(discardWriter{}, id, kind, reason, false, true); err != nil {
		t.Fatalf("DoHold #%d: %v", id, err)
	}
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func head(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func htmlArticleContaining(t *testing.T, html, needle string) string {
	t.Helper()
	idx := strings.Index(html, needle)
	if idx < 0 {
		t.Fatalf("html missing article needle %q\n%s", needle, head(html, 2000))
	}
	start := strings.LastIndex(html[:idx], "<article")
	endRel := strings.Index(html[idx:], "</article>")
	if start < 0 || endRel < 0 {
		t.Fatalf("could not isolate article containing %q", needle)
	}
	return html[start : idx+endRel+len("</article>")]
}

func htmlStepsForPhase(t *testing.T, html string, phaseID int64) string {
	t.Helper()
	phaseStart := strings.Index(html, fmt.Sprintf(`id="phase-%d"`, phaseID))
	if phaseStart < 0 {
		t.Fatalf("html missing phase #%d\n%s", phaseID, head(html, 2000))
	}
	startRel := strings.Index(html[phaseStart:], `<div class="steps"`)
	if startRel < 0 {
		t.Fatalf("html missing steps for phase #%d\n%s", phaseID, head(html[phaseStart:], 2000))
	}
	start := phaseStart + startRel
	endRel := strings.Index(html[start:], `</div>`)
	if endRel < 0 {
		t.Fatalf("could not isolate steps for phase #%d", phaseID)
	}
	return html[start : start+endRel+len(`</div>`)]
}

// errorsAs is a tiny std-free errors.As to keep test deps unchanged.
func errorsAs(err error, target **HandlerError) bool {
	for err != nil {
		if he, ok := err.(*HandlerError); ok {
			*target = he
			return true
		}
		break
	}
	return false
}
