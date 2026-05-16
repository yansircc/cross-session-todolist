package cst

import (
	"encoding/json"
	"fmt"
	"html"
	"sort"
	"strings"
	"time"
)

// renderHTML produces a self-contained HTML document for v. No external
// fonts, no scripts, no remote assets. CSS is inlined from uiCSS.
func renderHTML(v uiView) string {
	var sb strings.Builder
	sb.WriteString(`<!doctype html>` + "\n")
	sb.WriteString(`<html lang="zh"><head>` + "\n")
	sb.WriteString(`<meta charset="utf-8">` + "\n")
	sb.WriteString(`<meta name="viewport" content="width=device-width,initial-scale=1">` + "\n")
	sb.WriteString(`<title>`)
	sb.WriteString(html.EscapeString(v.Project + " · CST 当前状态"))
	sb.WriteString(`</title>` + "\n")
	sb.WriteString(`<style>`)
	sb.WriteString(uiCSS)
	sb.WriteString(`</style>` + "\n</head><body><main>\n")

	renderHeader(&sb, v)
	renderKPI(&sb, v)
	renderRunDiagnostics(&sb, v)

	if len(v.ActiveScopes) == 0 {
		sb.WriteString(`<div class="empty">所有 goal 已收敛，没有进行中的任务。</div>` + "\n")
	} else {
		renderScopeNav(&sb, v.ActiveScopes)
		for _, sc := range v.ActiveScopes {
			renderScope(&sb, sc)
		}
	}

	renderFooter(&sb, v)
	sb.WriteString(`</main></body></html>` + "\n")
	return sb.String()
}

func renderHeader(sb *strings.Builder, v uiView) {
	fmt.Fprintf(sb, `<h1>%s · CST 当前状态</h1>`+"\n", html.EscapeString(v.Project))
	totalNodes := v.GoalCount + v.RuleCount + v.TotalTasks
	fmt.Fprintf(sb,
		`<div class="report-meta"><span>snapshot %s</span><span>events %d</span><span>nodes %d</span><span style="color:var(--md-ink)">open tasks %d</span></div>`+"\n",
		v.GeneratedAt.Format("2006-01-02 15:04"),
		v.TotalEvents, totalNodes, v.OpenTasks)
	fmt.Fprintf(sb, `<div class="report-meta"><span class="kbd">%s</span></div>`+"\n",
		html.EscapeString(v.EventsPath))
}

func renderKPI(sb *strings.Builder, v uiView) {
	sb.WriteString(`<div class="kpi-grid">`)
	fmt.Fprintf(sb, `<div class="stat-card"><div class="stat-label">active scopes</div><div class="stat-value">%d</div></div>`, len(v.ActiveScopes))
	fmt.Fprintf(sb, `<div class="stat-card"><div class="stat-label">open / total</div><div class="stat-value">%d / %d</div></div>`, v.OpenTasks, v.TotalTasks)
	fmt.Fprintf(sb, `<div class="stat-card"><div class="stat-label">claimed · held</div><div class="stat-value">%d · %d</div></div>`, v.ClaimedTasks, v.HeldTasks)
	fmt.Fprintf(sb, `<div class="stat-card"><div class="stat-label">last event</div><div class="stat-value">%s</div></div>`, html.EscapeString(humanize(v.LastEvent, v.GeneratedAt)))
	sb.WriteString(`</div>` + "\n")
}

func renderRunDiagnostics(sb *strings.Builder, v uiView) {
	if len(v.RecentFailures) == 0 && len(v.RecentRuns) == 0 {
		return
	}
	sb.WriteString(`<section class="run-diagnostics">`)
	if len(v.RecentFailures) > 0 {
		renderRunList(sb, "recent failures", v.RecentFailures)
	}
	if len(v.RecentRuns) > 0 {
		renderRunList(sb, "recent script_runs", v.RecentRuns)
	}
	sb.WriteString(`</section>` + "\n")
}

func renderRunList(sb *strings.Builder, label string, runs []ScriptRunRecord) {
	fmt.Fprintf(sb, `<div class="run-list"><div class="run-list-title">%s · %d</div>`, html.EscapeString(label), len(runs))
	for _, r := range runs {
		renderRunLine(sb, r)
	}
	sb.WriteString(`</div>`)
}

func renderRunLine(sb *strings.Builder, r ScriptRunRecord) {
	status := `<span class="ok">pass</span>`
	if r.ExitCode != 0 {
		status = fmt.Sprintf(`<span class="fail">exit %d</span>`, r.ExitCode)
	}
	check := normalizedCheckName(r.CheckName)
	fmt.Fprintf(sb,
		`<div class="run-line"><span class="run-node">#%d</span>%s <span class="trig">%s</span> <span class="trig">%s</span> <code>%s</code> <span class="run-at">%s</span>`,
		r.NodeID, status, html.EscapeString(r.Trigger), html.EscapeString(check),
		html.EscapeString(truncate(r.Cmd, 120)), html.EscapeString(humanize(r.At, time.Now())))
	if r.AttemptID != "" {
		fmt.Fprintf(sb, ` <code class="cmd">cst events --attempt %s</code>`, html.EscapeString(r.AttemptID))
	}
	sb.WriteString(`</div>`)
}

func renderScopeNav(sb *strings.Builder, scopes []scopeView) {
	sb.WriteString(`<nav class="scope-nav">`)
	for _, sc := range scopes {
		intent := truncate(sc.Goal.Intent, 32)
		fmt.Fprintf(sb,
			`<a href="#scope-%d">#%d %s <span class="nv-bar"><i style="width:%d%%"></i></span> <span class="nv-frac">%d/%d</span></a>`,
			sc.Goal.ID, sc.Goal.ID, html.EscapeString(intent), sc.PctDone, sc.Done, sc.Total)
	}
	sb.WriteString(`</nav>` + "\n")
}

func renderScope(sb *strings.Builder, sc scopeView) {
	now := time.Now()
	fmt.Fprintf(sb, `<section class="scope" id="scope-%d"><div class="callout info">`, sc.Goal.ID)

	// title bar
	fmt.Fprintf(sb,
		`<div class="callout-title"><span class="scope-title"><span class="nid">#%d</span>%s</span><span class="scope-meta">最近活动 %s</span></div>`,
		sc.Goal.ID, html.EscapeString(sc.Goal.Intent), html.EscapeString(humanize(sc.LastActivity, now)))

	sb.WriteString(`<div class="callout-body">`)

	// crumb
	if len(sc.Ancestors) > 0 {
		sb.WriteString(`<div class="crumb">`)
		bits := make([]string, 0, len(sc.Ancestors))
		for _, a := range sc.Ancestors {
			bits = append(bits, fmt.Sprintf(`#%d %s`, a.ID, html.EscapeString(truncate(a.Intent, 40))))
		}
		sb.WriteString(strings.Join(bits, " › "))
		sb.WriteString(` ›</div>`)
	}

	// progress
	openCounts := ""
	statusOrder := []NodeStatus{StatusClaimed, StatusHeld, StatusOpen, StatusCanceled}
	for _, st := range statusOrder {
		if n := sc.OpenByStatus[st]; n > 0 {
			openCounts += fmt.Sprintf(`<span><b>%d</b> %s</span>`, n, statusLabel(st))
		}
	}
	fmt.Fprintf(sb,
		`<div class="progress"><div class="bar"><div style="width:%d%%"></div></div>`+
			`<div class="bar-line"><span><b>%d</b> / %d 任务完成 · %d%%</span><span class="counts">%s</span></div></div>`,
		sc.PctDone, sc.Done, sc.Total, sc.PctDone, openCounts)

	// rules
	if len(sc.Rules) > 0 {
		fmt.Fprintf(sb, `<div class="rules-block"><div class="rules-label">inherited rules · %d</div><ul>`, len(sc.Rules))
		for _, r := range sc.Rules {
			text := r.RuleText
			if text == "" {
				text = r.Intent
			}
			fmt.Fprintf(sb, `<li><span class="rule-id">#%d</span> %s</li>`, r.ID, html.EscapeString(text))
		}
		sb.WriteString(`</ul></div>`)
	}

	// sub-scope nav
	if len(sc.SubGoals) > 0 {
		sb.WriteString(`<div class="sub-scopes">↳ 子 scope · `)
		bits := make([]string, 0, len(sc.SubGoals))
		for _, sg := range sc.SubGoals {
			label := html.EscapeString(truncate(sg.Intent, 30))
			if sg.Canceled {
				bits = append(bits, fmt.Sprintf(`<span class="done-ref">#%d %s</span>`, sg.ID, label))
			} else {
				bits = append(bits, fmt.Sprintf(`<a href="#scope-%d">#%d %s</a>`, sg.ID, sg.ID, label))
			}
		}
		sb.WriteString(strings.Join(bits, " · "))
		sb.WriteString(`</div>`)
	}

	// task groups
	var open, done []*Node
	for _, t := range sc.AllTasks {
		if t.Completed {
			done = append(done, t)
		} else {
			open = append(open, t)
		}
	}
	if len(open) > 0 {
		fmt.Fprintf(sb, `<div class="group-label">未完成 · %d</div>`, len(open))
		for _, t := range open {
			renderTask(sb, t)
		}
	}
	if len(done) > 0 {
		fmt.Fprintf(sb, `<div class="group-label">已完成 · %d · 按完成时间倒序</div>`, len(done))
		for _, t := range done {
			renderTask(sb, t)
		}
	}

	sb.WriteString(`</div></div></section>` + "\n")
}

func renderTask(sb *strings.Builder, t *Node) {
	now := time.Now()
	st := t.Status()
	cls := statusClass(st)
	lbl := statusLabel(st)

	when := ""
	switch st {
	case StatusCompleted:
		when = "完成 " + humanize(t.CompletedAt, now)
	case StatusHeld:
		if t.Hold != nil {
			when = "挂起 " + humanize(t.Hold.At, now)
		}
	case StatusClaimed:
		if t.Claim != nil {
			when = "领取 " + humanize(t.Claim.TakenAt, now)
		}
	case StatusCanceled:
		when = "取消 " + humanize(t.CanceledAt, now)
	case StatusOpen:
		when = "创建 " + humanize(t.CreatedAt, now)
	}

	fmt.Fprintf(sb,
		`<div class="task %s"><div class="task-head"><span class="task-status">%s</span><span class="task-nid">#%d</span><span class="task-title">%s</span><span class="task-when">%s</span></div><div class="task-body">`,
		cls, lbl, t.ID, html.EscapeString(t.Intent), html.EscapeString(when))

	renderTaskNote(sb, t)
	renderTaskHoldOrCancel(sb, t)
	renderTaskCommands(sb, t)
	renderTaskVerify(sb, t)
	renderTaskAcceptance(sb, t)

	sb.WriteString(`</div></div>`)
}

func renderTaskCommands(sb *strings.Builder, t *Node) {
	cmds := []string{fmt.Sprintf("cst show %d", t.ID)}
	if attemptID := taskAttemptID(t); attemptID != "" {
		cmds = append(cmds, fmt.Sprintf("cst events --attempt %s", attemptID))
	}
	sb.WriteString(`<div class="commands"><span class="field-label">read commands</span>`)
	for _, cmd := range cmds {
		fmt.Fprintf(sb, `<code>%s</code>`, html.EscapeString(cmd))
	}
	sb.WriteString(`</div>`)
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

func renderTaskNote(sb *strings.Builder, t *Node) {
	// Pick the evidence referenced by task_completed if it's a real note
	// (review acceptance). For verify acceptance, the completion points at a
	// script_run record that we surface separately in the verify line, so we
	// instead reach for the most recent non-script_run evidence (the Agent's
	// own note, if any).
	var ev *EvidenceRecord
	if t.CompletedEvidence != "" {
		for i := range t.Evidences {
			if t.Evidences[i].EventID == t.CompletedEvidence && t.Evidences[i].Kind != EvidenceScript {
				ev = &t.Evidences[i]
				break
			}
		}
	}
	if ev == nil {
		for i := len(t.Evidences) - 1; i >= 0; i-- {
			if t.Evidences[i].Kind != EvidenceScript {
				ev = &t.Evidences[i]
				break
			}
		}
	}
	if ev == nil {
		return
	}
	if ev.Summary != "" {
		fmt.Fprintf(sb,
			`<div class="note"><span class="field-label">Agent %s</span>%s</div>`,
			html.EscapeString(ev.Kind), html.EscapeString(ev.Summary))
	}
	if len(ev.Data) > 0 && string(ev.Data) != "null" {
		renderEvidenceData(sb, ev.Data)
	}
}

func renderEvidenceData(sb *strings.Builder, raw json.RawMessage) {
	// Try to render as an object via dl/dt/dd; fall back to raw pre.
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err == nil && len(obj) > 0 {
		// preserve a stable order so output is deterministic
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		sb.WriteString(`<details><summary>evidence_data</summary><dl class="ev-data">`)
		for _, k := range keys {
			val := string(obj[k])
			var s string
			if err := json.Unmarshal(obj[k], &s); err == nil {
				val = s
			}
			val = truncate(val, 400)
			fmt.Fprintf(sb, `<dt>%s</dt><dd>%s</dd>`, html.EscapeString(k), html.EscapeString(val))
		}
		sb.WriteString(`</dl></details>`)
		return
	}
	pretty := string(raw)
	if len(pretty) > 800 {
		pretty = pretty[:800] + "…"
	}
	sb.WriteString(`<details><summary>evidence_data</summary><pre>`)
	sb.WriteString(html.EscapeString(pretty))
	sb.WriteString(`</pre></details>`)
}

func renderTaskHoldOrCancel(sb *strings.Builder, t *Node) {
	if t.Hold != nil {
		fmt.Fprintf(sb,
			`<blockquote class="reason"><span class="field-label">挂起 · %s</span>%s</blockquote>`,
			html.EscapeString(t.Hold.Kind), html.EscapeString(t.Hold.Reason))
	}
	if t.Canceled && t.CanceledReason != "" {
		fmt.Fprintf(sb,
			`<blockquote class="reason aban"><span class="field-label">取消原因</span>%s</blockquote>`,
			html.EscapeString(t.CanceledReason))
	}
}

func renderTaskVerify(sb *strings.Builder, t *Node) {
	if len(t.Runs) == 0 {
		return
	}
	last := t.Runs[len(t.Runs)-1]
	cmd := truncate(last.Cmd, 160)
	dur := float64(last.DurationMs) / 1000
	status := `<span class="ok">✓ pass</span>`
	if last.ExitCode != 0 {
		status = fmt.Sprintf(`<span class="fail">✕ exit %d</span>`, last.ExitCode)
	}
	trig := ""
	if last.Trigger != "" {
		trig = fmt.Sprintf(` <span class="trig">%s</span>`, html.EscapeString(last.Trigger))
	}
	check := ""
	if last.CheckName != "" {
		check = fmt.Sprintf(` <span class="trig">%s</span>`, html.EscapeString(last.CheckName))
	}
	count := ""
	if len(t.Runs) > 1 {
		count = fmt.Sprintf(` · %d runs`, len(t.Runs))
	}
	attempt := ""
	if last.AttemptID != "" {
		attempt = fmt.Sprintf(` <code class="cmd">cst events --attempt %s</code>`, html.EscapeString(last.AttemptID))
	}
	trunc := ""
	if last.Truncated {
		trunc = ` <span class="truncated">truncated</span>`
	}
	fmt.Fprintf(sb,
		`<div class="verify"><span class="field-label">latest script_run</span>%s%s%s · <code>%s</code> · %.1fs%s%s%s</div>`,
		status, trig, check, html.EscapeString(cmd), dur, count, trunc, attempt)

	if len(t.Runs) > 1 {
		renderTaskRunDetails(sb, t.Runs)
	}

	if last.StdoutHead != "" {
		s := last.StdoutHead
		if len(s) > 1200 {
			s = s[:1200] + "…"
		}
		fmt.Fprintf(sb, `<details><summary>stdout · %d chars</summary><pre>%s</pre></details>`,
			len(last.StdoutHead), html.EscapeString(s))
	}
	if last.StderrHead != "" {
		s := last.StderrHead
		if len(s) > 600 {
			s = s[:600] + "…"
		}
		fmt.Fprintf(sb, `<details><summary>stderr · %d chars</summary><pre>%s</pre></details>`,
			len(last.StderrHead), html.EscapeString(s))
	}
}

func renderTaskRunDetails(sb *strings.Builder, runs []ScriptRunRecord) {
	sb.WriteString(`<details><summary>script_runs by attempt/check</summary><div class="run-detail">`)
	start := 0
	if len(runs) > 8 {
		start = len(runs) - 8
		fmt.Fprintf(sb, `<div class="run-more">showing latest 8 of %d</div>`, len(runs))
	}
	for i := len(runs) - 1; i >= start; i-- {
		renderRunLine(sb, runs[i])
	}
	sb.WriteString(`</div></details>`)
}

func renderTaskAcceptance(sb *strings.Builder, t *Node) {
	// Only show on open tasks; for done tasks the verify line already covers it.
	if t.Terminal() || t.Acceptance == nil {
		return
	}
	switch t.Acceptance.Kind {
	case AcceptanceVerify:
		for _, check := range t.Acceptance.VerifyChecks() {
			fmt.Fprintf(sb,
				`<div class="acceptance"><span class="field-label">验收条件 verify.%s</span><code>%s</code></div>`,
				html.EscapeString(check.Name), html.EscapeString(truncate(check.Cmd, 200)))
		}
	case AcceptanceReview:
		who := t.Acceptance.Who
		if who == "" {
			who = "?"
		}
		fmt.Fprintf(sb,
			`<div class="acceptance"><span class="field-label">验收条件 review.who</span><code>%s</code></div>`,
			html.EscapeString(who))
	}
}

func renderFooter(sb *strings.Builder, v uiView) {
	fmt.Fprintf(sb,
		`<footer>%d events · %d goal · %d rule · %d task &nbsp;·&nbsp; 重跑 <span class="kbd">cst ui</span> 刷新此快照</footer>`+"\n",
		v.TotalEvents, v.GoalCount, v.RuleCount, v.TotalTasks)
}

// statusLabel returns the Chinese display label for a NodeStatus.
func statusLabel(st NodeStatus) string {
	switch st {
	case StatusCompleted:
		return "已完成"
	case StatusClaimed:
		return "进行中"
	case StatusHeld:
		return "已挂起"
	case StatusCanceled:
		return "已取消"
	case StatusOpen:
		return "待领取"
	}
	return string(st)
}

func statusClass(st NodeStatus) string {
	switch st {
	case StatusCompleted:
		return "done"
	case StatusClaimed:
		return "claimed"
	case StatusHeld:
		return "held"
	case StatusCanceled:
		return "abandoned"
	case StatusOpen:
		return "ready"
	}
	return "ready"
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
