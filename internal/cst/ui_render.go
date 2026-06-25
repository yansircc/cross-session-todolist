package cst

import (
	"fmt"
	"html"
	"strings"
	"time"
)

// renderHTML produces a self-contained HTML document for v. No external
// fonts, no scripts, no remote assets. CSS is inlined from uiCSS.
func renderHTML(v uiView) string {
	var sb strings.Builder
	sb.WriteString(`<!doctype html>` + "\n")
	sb.WriteString(`<html lang="zh-CN"><head>` + "\n")
	sb.WriteString(`<meta charset="utf-8">` + "\n")
	sb.WriteString(`<meta name="viewport" content="width=device-width,initial-scale=1">` + "\n")
	sb.WriteString(`<title>`)
	sb.WriteString(html.EscapeString(v.Project + " · CST Progress"))
	sb.WriteString(`</title>` + "\n")
	sb.WriteString(`<style>`)
	sb.WriteString(uiCSS)
	sb.WriteString(`</style>` + "\n</head><body>\n")

	sb.WriteString(`<div class="layout">`)
	renderRail(&sb, v)
	renderMain(&sb, v)
	renderSide(&sb, v)
	sb.WriteString(`</div>`)

	sb.WriteString(`</body></html>` + "\n")
	return sb.String()
}

func renderRail(sb *strings.Builder, v uiView) {
	sb.WriteString(`<aside class="rail">`)
	sb.WriteString(`<div class="brand">`)
	fmt.Fprintf(sb, `<h1>%s</h1>`, html.EscapeString(v.Project+" CST"))
	rootText := "empty store"
	if v.Root != nil {
		rootText = fmt.Sprintf("#%d %s", v.Root.ID, v.Root.Intent)
	}
	fmt.Fprintf(sb, `<div class="root mono">%s</div>`, html.EscapeString(truncate(rootText, 90)))
	sb.WriteString(`</div>`)

	if len(v.ActivePhases) > 0 {
		sb.WriteString(`<nav class="nav-list" aria-label="active phases">`)
		for i, phase := range v.ActivePhases {
			active := ""
			if i == 0 {
				active = " active"
			}
			fmt.Fprintf(sb,
				`<a class="nav-item%s" href="#phase-%d"><span class="mono">#%d</span><span class="name">%s</span><span class="dot %s" aria-hidden="true">•</span></a>`,
				active, phase.Node.ID, phase.Node.ID, html.EscapeString(truncate(phase.Node.Intent, 28)), html.EscapeString(phaseStatusClass(phase)))
		}
		sb.WriteString(`</nav>`)
	} else {
		sb.WriteString(`<div class="empty-rail">No active phase</div>`)
	}

	fmt.Fprintf(sb,
		`<div class="rail-footer mono">events=%d<br>last=%s</div>`,
		v.TotalEvents, html.EscapeString(lastEventID(v)))
	sb.WriteString(`</aside>`)
}

func renderMain(sb *strings.Builder, v uiView) {
	sb.WriteString(`<main class="content">`)
	renderTop(sb, v)
	renderSummary(sb, v)
	renderPhaseSection(sb, v)
	sb.WriteString(`</main>`)
}

func renderTop(sb *strings.Builder, v uiView) {
	title := "No active work. Frontier is closed."
	if v.Summary.OpenTasks > 0 {
		title = fmt.Sprintf("%d%% done. %s.", v.Summary.PercentDone, currentGateSentence(v))
	}
	sb.WriteString(`<header class="top">`)
	sb.WriteString(`<div>`)
	sb.WriteString(`<div class="eyebrow">frontier-first phase dashboard</div>`)
	fmt.Fprintf(sb, `<h2 class="title">%s</h2>`, html.EscapeString(title))
	sb.WriteString(`<div class="meta">`)
	scopeText := "scope none"
	if v.Scope != nil {
		scopeStatus := "closed"
		if v.Summary.OpenTasks > 0 {
			scopeStatus = "active"
		}
		scopeText = fmt.Sprintf("scope #%d %s", v.Scope.ID, scopeStatus)
	}
	fmt.Fprintf(sb, `<span class="chip mono">%s</span>`, html.EscapeString(scopeText))
	fmt.Fprintf(sb, `<span class="chip mono">snapshot %s</span>`, html.EscapeString(v.GeneratedAt.Format("2006-01-02 15:04")))
	fmt.Fprintf(sb, `<span class="chip mono">%s</span>`, html.EscapeString(v.EventsPath))
	sb.WriteString(`</div></div>`)
	sb.WriteString(`<div class="refresh"><code class="cmd-pill">cst brief</code><code class="cmd-pill primary">cst ui</code></div>`)
	sb.WriteString(`</header>`)
}

func renderSummary(sb *strings.Builder, v uiView) {
	sb.WriteString(`<section class="progress-strip" aria-label="summary">`)
	renderMetric(sb, "overall progress", fmt.Sprintf("%d%%", v.Summary.PercentDone), fmt.Sprintf("%d/%d done", v.Summary.CompletedTasks, v.Summary.TotalTasks), "Completed historical subtrees stay out of the frontier view.", true)
	renderMetric(sb, "active phases", fmt.Sprintf("%d", len(v.ActivePhases)), "visible", "One row per active innermost workstream.", false)
	renderMetric(sb, "open tasks", fmt.Sprintf("%d", v.Summary.OpenTasks), "", fmt.Sprintf("%d claimed, %d held.", v.Summary.ClaimedTasks, v.Summary.HeldTasks), false)
	renderMetric(sb, "ready now", fmt.Sprintf("%d", v.Summary.ReadyTasks), "legal", "Waiting tasks are not legal actions.", false)
	renderMetric(sb, "current gate", currentGateValue(v), "", currentGateHint(v), false)
	renderMetric(sb, "hidden history", fmt.Sprintf("%d", v.CompletedSubtreeTotal), "subtrees", "Use history only for audits.", false)
	sb.WriteString(`</section>`)
}

func renderMetric(sb *strings.Builder, label, value, valueMeta, note string, primary bool) {
	cls := "metric"
	if primary {
		cls += " primary"
	}
	fmt.Fprintf(sb, `<div class="%s"><div class="label">%s</div><div class="value">%s`, cls, html.EscapeString(label), html.EscapeString(value))
	if valueMeta != "" {
		fmt.Fprintf(sb, ` <span>%s</span>`, html.EscapeString(valueMeta))
	}
	fmt.Fprintf(sb, `</div><div class="note">%s</div></div>`, html.EscapeString(note))
}

func renderPhaseSection(sb *strings.Builder, v uiView) {
	sb.WriteString(`<section class="phase-section">`)
	sb.WriteString(`<div class="section-head"><div><h2>Stage Completion</h2><div class="sub">Click a phase to inspect progress equation, dependency chain, and task matrix.</div></div><div class="mono">frontier projection</div></div>`)
	if len(v.ActivePhases) == 0 {
		sb.WriteString(`<div class="empty">所有 goal 已收敛，没有进行中的任务。</div>`)
	} else {
		for i, phase := range v.ActivePhases {
			renderPhase(sb, phase, i == 0)
		}
	}
	sb.WriteString(`</section>`)
}

func renderPhase(sb *strings.Builder, phase phaseView, open bool) {
	statusClass := phaseStatusClass(phase)
	statusLabel := phaseStatusLabel(phase)
	blockTitle, blockText := phaseBlocker(phase)
	openAttr := ""
	if open {
		openAttr = " open"
	}
	fmt.Fprintf(sb, `<details class="phase" id="phase-%d"%s>`, phase.Node.ID, openAttr)
	sb.WriteString(`<summary class="phase-row">`)
	fmt.Fprintf(sb, `<div class="pid mono"><span>#%d</span><span class="badge %s">%s</span></div>`, phase.Node.ID, html.EscapeString(statusClass), html.EscapeString(statusLabel))
	sb.WriteString(`<div>`)
	fmt.Fprintf(sb, `<div class="phase-title">%s</div>`, html.EscapeString(phase.Node.Intent))
	sb.WriteString(`<div class="facts">`)
	renderCountBadge(sb, "claimed", phase.ClaimTotal)
	renderCountBadge(sb, "ready", phase.ReadyTotal)
	renderCountBadge(sb, "review", phase.ReviewReadyTotal)
	renderCountBadge(sb, "waiting", phase.WaitingTotal)
	renderCountBadge(sb, "failed", phase.DependencyFailedTotal)
	renderCountBadge(sb, "held", phase.HeldTotal)
	sb.WriteString(`</div></div>`)
	fmt.Fprintf(sb,
		`<div class="ratio"><div class="ratio-line"><strong>%d%%</strong><span>%d/%d</span></div><div class="bar" style="--pct:%d%%"></div></div>`,
		phase.Progress.PercentDone, phase.Progress.CompletedTasks, phase.Progress.TotalTasks, phase.Progress.PercentDone)
	fmt.Fprintf(sb, `<div class="blocker"><strong>%s</strong>%s</div>`, html.EscapeString(blockTitle), html.EscapeString(blockText))
	sb.WriteString(`</summary>`)

	sb.WriteString(`<div class="phase-detail">`)
	renderPhaseDetails(sb, phase)
	renderTaskMatrix(sb, phase)
	sb.WriteString(`</div></details>`)
}

func renderCountBadge(sb *strings.Builder, cls string, n int) {
	if n <= 0 {
		return
	}
	fmt.Fprintf(sb, `<span class="badge %s">%d %s</span>`, html.EscapeString(cls), n, html.EscapeString(cls))
}

func renderPhaseDetails(sb *strings.Builder, phase phaseView) {
	sb.WriteString(`<div class="detail-grid">`)
	fmt.Fprintf(sb,
		`<div class="detail"><h3>Progress Equation</h3><ul><li>Subtree tasks: %d total, %d terminal, %d open.</li><li>Ready is computed by IsReadyTask, not raw open status.</li><li>Last activity %s.</li></ul></div>`,
		phase.Progress.TotalTasks,
		phase.Progress.CompletedTasks+phase.Progress.CanceledTasks,
		phase.Progress.OpenTasks,
		html.EscapeString(humanize(phase.LastActivity, time.Now())))

	sb.WriteString(`<div class="detail"><h3>Dependency Chain</h3><ul>`)
	wrote := false
	for _, row := range phase.TaskRows {
		if len(row.WaitingOn) > 0 || len(row.BlockedBy) > 0 {
			wrote = true
			fmt.Fprintf(sb, `<li>#%d waits on %s%s.</li>`,
				row.Node.ID,
				html.EscapeString(joinIDsBare(row.WaitingOn)),
				html.EscapeString(blockedSuffix(row.BlockedBy)))
		}
	}
	if !wrote {
		sb.WriteString(`<li>No prerequisite wait edge in the shown task rows.</li>`)
	}
	sb.WriteString(`</ul></div>`)

	sb.WriteString(`<div class="detail"><h3>Inherited Rules</h3><ul>`)
	if len(phase.Rules) == 0 {
		sb.WriteString(`<li>No inherited rule for this phase.</li>`)
	} else {
		for _, rule := range phase.Rules {
			fmt.Fprintf(sb, `<li><span class="mono">#%d</span> %s</li>`, rule.ID, html.EscapeString(rule.RuleText))
		}
	}
	sb.WriteString(`</ul></div>`)

	sb.WriteString(`<div class="detail"><h3>Useful Reads</h3>`)
	fmt.Fprintf(sb, `<code class="cmd">cst show %d</code>`, phase.Node.ID)
	fmt.Fprintf(sb, `<code class="cmd">cst brief --within %d --human</code>`, phase.Node.ID)
	if firstClaim := firstRowByClass(phase.TaskRows, "claimed"); firstClaim != nil {
		fmt.Fprintf(sb, `<code class="cmd">cst worker-status %d --human</code>`, firstClaim.Node.ID)
	}
	sb.WriteString(`</div></div>`)
}

func renderTaskMatrix(sb *strings.Builder, phase phaseView) {
	sb.WriteString(`<table><thead><tr><th>Task</th><th>State</th><th>Acceptance</th><th>Human Value</th></tr></thead><tbody>`)
	if len(phase.TaskRows) == 0 {
		sb.WriteString(`<tr><td colspan="4">No task rows in this phase.</td></tr>`)
	}
	for _, row := range phase.TaskRows {
		fmt.Fprintf(sb, `<tr><td class="task-name"><span class="mono">#%d</span> %s`, row.Node.ID, html.EscapeString(row.Node.Intent))
		renderTaskDetail(sb, row)
		fmt.Fprintf(sb, `</td><td><span class="badge %s">%s</span>`, html.EscapeString(row.StateClass), html.EscapeString(row.StateLabel))
		if row.StateDetail != "" {
			fmt.Fprintf(sb, `<div class="state-detail">%s</div>`, html.EscapeString(row.StateDetail))
		}
		fmt.Fprintf(sb, `</td><td>%s</td><td>%s</td></tr>`, html.EscapeString(firstNonEmpty(row.Acceptance, "none")), html.EscapeString(taskHumanValue(row)))
	}
	if phase.TaskRowsTotal > len(phase.TaskRows) {
		fmt.Fprintf(sb, `<tr><td colspan="4" class="table-more">showing %d/%d task rows</td></tr>`, len(phase.TaskRows), phase.TaskRowsTotal)
	}
	sb.WriteString(`</tbody></table>`)
}

func renderTaskDetail(sb *strings.Builder, row taskRowView) {
	sb.WriteString(`<details class="task-detail">`)
	sb.WriteString(`<summary>details</summary>`)
	fmt.Fprintf(sb, `<p>%s</p>`, html.EscapeString(taskHumanValue(row)))
	if len(row.Commands) > 0 {
		sb.WriteString(`<p>`)
		for _, cmd := range row.Commands {
			fmt.Fprintf(sb, `<code class="cmd">%s</code>`, html.EscapeString(cmd))
		}
		sb.WriteString(`</p>`)
	}
	if row.LatestRun != nil {
		fmt.Fprintf(sb, `<p>latest run: %s exit=%d%s · %s</p>`,
			html.EscapeString(row.LatestRun.Trigger),
			row.LatestRun.ExitCode,
			html.EscapeString(checkSuffix(row.LatestRun.CheckName)),
			html.EscapeString(truncate(row.LatestRun.Cmd, 120)))
	}
	if row.Evidence != nil && row.Evidence.Summary != "" {
		fmt.Fprintf(sb, `<p>latest evidence: %s · %s</p>`, html.EscapeString(row.Evidence.Kind), html.EscapeString(row.Evidence.Summary))
	}
	if summary := closureSummary(row.Closure); summary != "" {
		fmt.Fprintf(sb, `<p>closure: %s</p>`, html.EscapeString(summary))
		for _, ev := range append(row.Closure.Boundary, row.Closure.Rationale...) {
			contest := ""
			if ev.Contested != nil {
				contest = " · contested"
			}
			fmt.Fprintf(sb, `<p>closure evidence: %s · %s%s</p>`, html.EscapeString(ev.Kind), html.EscapeString(ev.Summary), html.EscapeString(contest))
		}
	}
	sb.WriteString(`</details>`)
}

func renderSide(sb *strings.Builder, v uiView) {
	sb.WriteString(`<aside class="side">`)
	sb.WriteString(`<div class="side-block"><h2>Reading Model</h2><div class="kv">`)
	fmt.Fprintf(sb, `<div>main signal</div><div class="v">phase completion</div>`)
	fmt.Fprintf(sb, `<div>done</div><div class="v mono">%d / %d</div>`, v.Summary.CompletedTasks, v.Summary.TotalTasks)
	fmt.Fprintf(sb, `<div>remaining</div><div class="v mono">%d open</div>`, v.Summary.OpenTasks)
	fmt.Fprintf(sb, `<div>next unlock</div><div class="v mono">%s</div>`, html.EscapeString(currentGateValue(v)))
	sb.WriteString(`</div></div>`)

	sb.WriteString(`<div class="side-block"><h2>Current Claim</h2>`)
	if len(v.CurrentClaims) == 0 {
		sb.WriteString(`<div class="side-muted">No current claim.</div>`)
	} else {
		c := v.CurrentClaims[0]
		sb.WriteString(`<div class="kv">`)
		fmt.Fprintf(sb, `<div>task</div><div class="v mono">#%d</div>`, c.ID)
		fmt.Fprintf(sb, `<div>actor</div><div class="v">%s</div>`, html.EscapeString(c.Claim.Actor))
		fmt.Fprintf(sb, `<div>attempt</div><div class="v mono">%s</div>`, html.EscapeString(c.Claim.AttemptID))
		fmt.Fprintf(sb, `<div>lease</div><div class="v mono">%s</div>`, html.EscapeString(c.Claim.LeaseExpiresAt.Format(time.RFC3339)))
		sb.WriteString(`</div>`)
	}
	sb.WriteString(`</div>`)

	sb.WriteString(`<div class="side-block"><h2>Recent Evidence</h2><ul class="event-list">`)
	fmt.Fprintf(sb, `<li><strong>last event</strong><span class="mono">%s</span></li>`, html.EscapeString(humanize(v.LastEvent, v.GeneratedAt)))
	if len(v.RecentFailures) == 0 {
		sb.WriteString(`<li><strong>active failures</strong>None shown in the active frontier sample.</li>`)
	} else {
		for _, r := range v.RecentFailures {
			fmt.Fprintf(sb, `<li><strong>#%d failure</strong>%s exit=%d%s</li>`, r.NodeID, html.EscapeString(r.Trigger), r.ExitCode, html.EscapeString(checkSuffix(r.CheckName)))
		}
	}
	if len(v.RecentDone) > 0 {
		fmt.Fprintf(sb, `<li><strong>recent done</strong><span class="mono">%s</span></li>`, html.EscapeString(joinIDs(v.RecentDone)))
	}
	sb.WriteString(`</ul></div>`)

	sb.WriteString(`<div class="side-block"><h2>Invariant</h2><div class="kv">`)
	sb.WriteString(`<div>truth</div><div class="v mono">.cst/events.jsonl</div>`)
	sb.WriteString(`<div>projection</div><div class="v">phase progress plus frontier categories</div>`)
	sb.WriteString(`<div>no state</div><div class="v">native details only; no UI-owned task memory</div>`)
	sb.WriteString(`</div></div>`)
	sb.WriteString(`</aside>`)
}

func currentGateSentence(v uiView) string {
	if len(v.CurrentClaims) > 0 {
		return fmt.Sprintf("Task #%d is holding the next phase unlock", v.CurrentClaims[0].ID)
	}
	for _, phase := range v.ActivePhases {
		if row := firstRowByClass(phase.TaskRows, "ready"); row != nil {
			return fmt.Sprintf("Task #%d is ready", row.Node.ID)
		}
		if row := firstRowByClass(phase.TaskRows, "review"); row != nil {
			return fmt.Sprintf("Review task #%d is ready", row.Node.ID)
		}
	}
	return "No legal action is currently available"
}

func currentGateValue(v uiView) string {
	if len(v.CurrentClaims) > 0 {
		return fmt.Sprintf("#%d", v.CurrentClaims[0].ID)
	}
	for _, phase := range v.ActivePhases {
		if row := firstRowByClass(phase.TaskRows, "ready"); row != nil {
			return fmt.Sprintf("#%d", row.Node.ID)
		}
		if row := firstRowByClass(phase.TaskRows, "review"); row != nil {
			return fmt.Sprintf("#%d", row.Node.ID)
		}
	}
	return "none"
}

func currentGateHint(v uiView) string {
	if len(v.CurrentClaims) > 0 {
		return "Completion should unlock dependent frontier work."
	}
	if v.Summary.ReadyTasks > 0 {
		return "A legal take/review action is available."
	}
	return "No new task can be taken yet."
}

func firstRowByClass(rows []taskRowView, cls string) *taskRowView {
	for i := range rows {
		if rows[i].StateClass == cls {
			return &rows[i]
		}
	}
	return nil
}

func taskHumanValue(row taskRowView) string {
	switch row.StateClass {
	case "claimed":
		return "Explains why dependent phase progress is not moving."
	case "ready", "review":
		return "Legal frontier action is available."
	case "waiting":
		return "Shows the prerequisite that must complete first."
	case "failed":
		return "Canceled or missing prerequisite blocks this task."
	case "held":
		return "External pause reason must be resolved before progress resumes."
	case "done":
		return "Terminal task contributes to phase completion."
	default:
		return row.StateDetail
	}
}

func blockedSuffix(ids []int64) string {
	if len(ids) == 0 {
		return ""
	}
	return " blocked_by=" + joinIDsBare(ids)
}

func lastEventID(v uiView) string {
	if v.LastEvent.IsZero() {
		return "none"
	}
	return humanize(v.LastEvent, v.GeneratedAt)
}

func taskAttemptID(t *Node) string {
	if t.Claim != nil && t.Claim.AttemptID != "" {
		return t.Claim.AttemptID
	}
	for i := len(t.Runs) - 1; i >= 0; i-- {
		if t.Runs[i].AttemptID != "" {
			return t.Runs[i].AttemptID
		}
	}
	for i := len(t.Evidences) - 1; i >= 0; i-- {
		if t.Evidences[i].AttemptID != "" {
			return t.Evidences[i].AttemptID
		}
	}
	return ""
}

func isHumanEvidenceKind(kind string) bool {
	return kind != EvidenceScript && kind != EvidenceAcceptanceRunSet
}

func humanize(t, now time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := now.Sub(t)
	s := int64(d.Seconds())
	switch {
	case s < 60:
		return fmt.Sprintf("%ds 前", s)
	case s < 3600:
		return fmt.Sprintf("%dm 前", s/60)
	case s < 86400:
		return fmt.Sprintf("%dh 前", s/3600)
	case s < 86400*30:
		return fmt.Sprintf("%dd 前", s/86400)
	default:
		return t.Format("2006-01-02")
	}
}
