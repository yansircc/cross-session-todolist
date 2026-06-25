package cst

import (
	"fmt"
	"html"
	"sort"
	"strings"
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
	sb.WriteString(html.EscapeString(v.Project + " · CST Next Briefing"))
	sb.WriteString(`</title>` + "\n")
	sb.WriteString(`<style>`)
	sb.WriteString(uiCSS)
	renderTaskSelectorCSS(&sb, v)
	sb.WriteString(`</style>` + "\n</head><body>\n")

	sb.WriteString(`<div class="page">`)
	renderTop(&sb, v)
	renderPhases(&sb, v)
	sb.WriteString(`</div>`)

	sb.WriteString(`</body></html>` + "\n")
	return sb.String()
}

func renderTop(sb *strings.Builder, v uiView) {
	sb.WriteString(`<header class="top">`)
	sb.WriteString(`<div>`)
	fmt.Fprintf(sb, `<h1>%s</h1>`, html.EscapeString(v.Project+" CST"))
	sb.WriteString(`<div class="meta">`)
	fmt.Fprintf(sb, `<span class="tag">%s</span>`, html.EscapeString(uiProcedurePhase(v)))
	if action := uiProcedureAction(v); action != "" {
		fmt.Fprintf(sb, `<span class="tag active">%s</span>`, html.EscapeString(action))
	}
	sb.WriteString(`</div></div>`)
	fmt.Fprintf(sb, `<div class="progress-total">%d%%</div>`, v.Summary.PercentDone)
	sb.WriteString(`</header>`)
}

func renderPhases(sb *strings.Builder, v uiView) {
	if len(v.ActivePhases) == 0 {
		sb.WriteString(`<main class="empty">所有 goal 已收敛，没有进行中的任务。</main>`)
		return
	}
	sb.WriteString(`<main>`)
	for _, phase := range v.ActivePhases {
		renderPhase(sb, phase)
	}
	sb.WriteString(`</main>`)
}

func renderPhase(sb *strings.Builder, phase phaseView) {
	fmt.Fprintf(sb, `<section class="phase" id="phase-%d">`, phase.Node.ID)
	renderPhaseRadios(sb, phase)
	sb.WriteString(`<div class="phase-head">`)
	fmt.Fprintf(sb, `<h2>#%d %s</h2>`, phase.Node.ID, html.EscapeString(phaseTitle(phase.Node.Intent)))
	sb.WriteString(`<div class="progress">`)
	fmt.Fprintf(sb, `<b>%d%% complete</b>`, phase.Progress.PercentDone)
	renderPhaseSteps(sb, phase)
	sb.WriteString(`</div></div>`)
	renderPhasePanels(sb, phase)
	sb.WriteString(`</section>`)
}

func renderPhaseSteps(sb *strings.Builder, phase phaseView) {
	sb.WriteString(`<div class="steps" aria-label="task progress">`)
	for _, row := range progressStepRows(phase.TaskRows) {
		class := "step"
		if stateClass := stepClass(row); stateClass != "" {
			class += " " + stateClass
		}
		fmt.Fprintf(sb,
			`<label class="%s" for="%s" title="#%d" aria-label="#%d"></label>`,
			html.EscapeString(class), html.EscapeString(rowRadioID(phase, row)), row.Node.ID, row.Node.ID)
	}
	sb.WriteString(`</div>`)
}

func progressStepRows(rows []taskRowView) []taskRowView {
	out := append([]taskRowView(nil), rows...)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Node.ID < out[j].Node.ID
	})
	return out
}

func renderPhaseRadios(sb *strings.Builder, phase phaseView) {
	for i, row := range phase.TaskRows {
		checked := ""
		if i == defaultTaskIndex(phase.TaskRows) {
			checked = " checked"
		}
		fmt.Fprintf(sb, `<input class="task-radio" type="radio" name="phase-%d-task" id="%s"%s>`, phase.Node.ID, html.EscapeString(rowRadioID(phase, row)), checked)
	}
}

func renderPhasePanels(sb *strings.Builder, phase phaseView) {
	sb.WriteString(`<div class="detail" aria-live="polite">`)
	for _, row := range phase.TaskRows {
		renderTaskPanel(sb, phase, row)
	}
	if phase.TaskRowsTotal > len(phase.TaskRows) {
		fmt.Fprintf(sb, `<div class="more mono">showing %d/%d task rows</div>`, len(phase.TaskRows), phase.TaskRowsTotal)
	}
	sb.WriteString(`</div>`)
}

func renderTaskPanel(sb *strings.Builder, phase phaseView, row taskRowView) {
	radioID := rowRadioID(phase, row)
	fmt.Fprintf(sb, `<article class="panel panel-for-%s">`, html.EscapeString(radioID))
	fmt.Fprintf(sb, `<h3>#%d %s</h3>`, row.Node.ID, html.EscapeString(taskTitle(row.Node.Intent)))
	if state := taskStateLine(row); state != "" {
		fmt.Fprintf(sb, `<div class="state-line %s">%s</div>`, html.EscapeString(row.StateClass), html.EscapeString(state))
	}
	if brief := taskBrief(row.Node.Intent); brief != "" {
		fmt.Fprintf(sb, `<div class="brief">%s</div>`, html.EscapeString(brief))
	}
	sb.WriteString(`<div class="grid">`)
	renderChecksDetail(sb, row)
	renderEvidenceDetail(sb, row)
	renderRulesDetail(sb, phase)
	renderEdgesDetail(sb, row)
	renderContextDetail(sb, row)
	sb.WriteString(`</div>`)
	sb.WriteString(`</article>`)
}

func renderTaskSelectorCSS(sb *strings.Builder, v uiView) {
	for _, phase := range v.ActivePhases {
		for _, row := range phase.TaskRows {
			id := rowRadioID(phase, row)
			fmt.Fprintf(sb, "\n#%s:checked ~ .phase-head label[for=\"%s\"]{background:var(--ink);transform:scale(1.25);}", id, id)
			fmt.Fprintf(sb, "\n#%s:checked ~ .detail .panel-for-%s{display:block;}", id, id)
		}
	}
}

func renderChecksDetail(sb *strings.Builder, row taskRowView) {
	checks := taskCheckNames(row.Node)
	open := ""
	if row.StateClass == "claimed" || row.StateClass == "ready" || row.StateClass == "review" || row.StateClass == "failed" {
		open = " open"
	}
	fmt.Fprintf(sb, `<details%s><summary><span class="summary-left">checks <span class="count">%d</span></span></summary><div class="detail-body">`, open, len(checks))
	sb.WriteString(`<div class="checks">`)
	if len(checks) == 0 {
		renderCheck(sb, "none", "")
	}
	for i, check := range checks {
		class := ""
		switch {
		case row.StateClass == "done":
			class = "pass"
		case row.StateClass == "claimed" && i == 0:
			class = "next"
		}
		renderCheck(sb, check, class)
	}
	sb.WriteString(`</div></div></details>`)
}

func renderEvidenceDetail(sb *strings.Builder, row taskRowView) {
	evidences := taskEvidenceRecords(row.Node)
	count := len(evidences)
	fmt.Fprintf(sb, `<details><summary><span class="summary-left">evidence <span class="count">%d</span></span></summary><div class="detail-body">`, count)
	wrote := false
	if len(evidences) > 0 {
		limit := len(evidences)
		if limit > 3 {
			limit = 3
		}
		for _, evidence := range evidences[:limit] {
			fmt.Fprintf(sb, `<div>%s</div>`, html.EscapeString(evidence.Summary))
		}
		if len(evidences) > limit {
			fmt.Fprintf(sb, `<div class="muted">+%d more</div>`, len(evidences)-limit)
		}
		wrote = true
	}
	if summary := closureSummary(row.Closure); summary != "" {
		fmt.Fprintf(sb, `<div>closure: %s</div>`, html.EscapeString(summary))
		for _, ev := range append(row.Closure.Boundary, row.Closure.Rationale...) {
			contest := ""
			if ev.Contested != nil {
				contest = " · contested"
			}
			fmt.Fprintf(sb, `<div>closure evidence: %s · %s%s</div>`, html.EscapeString(ev.Kind), html.EscapeString(ev.Summary), html.EscapeString(contest))
		}
		wrote = true
	}
	if !wrote {
		sb.WriteString(`<div>No evidence recorded for this task yet.</div>`)
	}
	sb.WriteString(`</div></details>`)
}

func renderRulesDetail(sb *strings.Builder, phase phaseView) {
	fmt.Fprintf(sb, `<details><summary><span class="summary-left">rules <span class="count">%d</span></span></summary><div class="detail-body">`, len(phase.Rules))
	if len(phase.Rules) == 0 {
		sb.WriteString(`<div>No inherited rules.</div>`)
	} else {
		limit := len(phase.Rules)
		if limit > 3 {
			limit = 3
		}
		for _, rule := range phase.Rules[:limit] {
			fmt.Fprintf(sb, `<div class="row"><span>#%d</span><span>%s</span></div>`, rule.ID, html.EscapeString(rule.RuleText))
		}
		if len(phase.Rules) > limit {
			fmt.Fprintf(sb, `<div class="muted">+%d more</div>`, len(phase.Rules)-limit)
		}
	}
	sb.WriteString(`</div></details>`)
}

func renderEdgesDetail(sb *strings.Builder, row taskRowView) {
	upstream, downstream := taskEdges(row)
	count := len(upstream) + len(downstream)
	fmt.Fprintf(sb, `<details><summary><span class="summary-left">edges <span class="count">%d</span></span></summary><div class="detail-body">`, count)
	if len(upstream) > 0 {
		fmt.Fprintf(sb, `<div class="row"><span>from</span><span>%s</span></div>`, html.EscapeString(joinIDsBare(upstream)))
	}
	if len(downstream) > 0 {
		fmt.Fprintf(sb, `<div class="row"><span>to</span><span>%s</span></div>`, html.EscapeString(joinIDsBare(downstream)))
	}
	if count == 0 {
		sb.WriteString(`<div>No direct edge.</div>`)
	}
	sb.WriteString(`</div></details>`)
}

func renderContextDetail(sb *strings.Builder, row taskRowView) {
	briefing := row.Briefing
	count := contextLineCount(briefing)
	fmt.Fprintf(sb, `<details><summary><span class="summary-left">context <span class="count">%d</span></span></summary><div class="detail-body">`, count)
	if count == 0 {
		sb.WriteString(`<div>No folded context for this task.</div>`)
	} else {
		renderContextRows(sb, briefing)
	}
	sb.WriteString(`</div></details>`)
}

func renderCheck(sb *strings.Builder, label string, class string) {
	cls := "check"
	if class != "" {
		cls += " " + class
	}
	fmt.Fprintf(sb, `<span class="%s">%s</span>`, html.EscapeString(cls), html.EscapeString(label))
}

func uiProcedurePhase(v uiView) string {
	switch {
	case v.Root == nil:
		return NextPhaseInit
	case v.Summary.OpenTasks == 0:
		return NextPhaseNoOp
	default:
		return NextPhaseWork
	}
}

func uiProcedureAction(v uiView) string {
	row := currentProcedureRow(v)
	if row == nil {
		if v.Summary.OpenTasks == 0 {
			return ""
		}
		return "inspect frontier"
	}
	switch row.StateClass {
	case "claimed":
		if row.Node.Acceptance != nil && row.Node.Acceptance.Kind == AcceptanceReview {
			return "record review"
		}
		return "run acceptance"
	case "ready":
		return "take task"
	case "review":
		return "review"
	case "failed", "held":
		return "repair"
	default:
		return "inspect frontier"
	}
}

func currentProcedureRow(v uiView) *taskRowView {
	for _, phase := range v.ActivePhases {
		for i := range phase.TaskRows {
			switch phase.TaskRows[i].StateClass {
			case "claimed", "ready", "review", "failed", "held":
				return &phase.TaskRows[i]
			}
		}
	}
	return nil
}

func defaultTaskIndex(rows []taskRowView) int {
	for i := range rows {
		switch rows[i].StateClass {
		case "claimed", "ready", "review", "failed", "held":
			return i
		}
	}
	return 0
}

func rowRadioID(phase phaseView, row taskRowView) string {
	return fmt.Sprintf("phase-%d-task-%d", phase.Node.ID, row.Node.ID)
}

func stepClass(row taskRowView) string {
	switch row.StateClass {
	case "done":
		return "done"
	case "claimed", "ready", "review", "failed", "held":
		return "now"
	default:
		return ""
	}
}

func phaseTitle(intent string) string {
	return titleAfterDash(intent)
}

func taskTitle(intent string) string {
	title := titleAfterDash(intent)
	if idx := strings.Index(title, ":"); idx > 0 {
		title = strings.TrimSpace(title[:idx])
	}
	return truncate(title, 86)
}

func taskBrief(intent string) string {
	title := titleAfterDash(intent)
	if idx := strings.Index(title, ":"); idx > 0 && strings.TrimSpace(title[idx+1:]) != "" {
		return truncate(strings.TrimSpace(title[idx+1:]), 420)
	}
	return ""
}

func titleAfterDash(intent string) string {
	parts := strings.SplitN(intent, " - ", 2)
	if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
		return strings.TrimSpace(parts[1])
	}
	return strings.TrimSpace(intent)
}

func taskCheckNames(n *Node) []string {
	if n.Acceptance == nil {
		return nil
	}
	switch n.Acceptance.Kind {
	case AcceptanceVerify:
		checks := n.Acceptance.VerifyChecks()
		out := make([]string, 0, len(checks))
		for _, check := range checks {
			out = append(out, normalizedCheckName(check.Name))
		}
		return out
	case AcceptanceReview:
		return []string{"review"}
	default:
		return []string{n.Acceptance.Kind}
	}
}

func taskEvidenceRecords(n *Node) []EvidenceRecord {
	var out []EvidenceRecord
	for _, evidence := range n.Evidences {
		if isHumanEvidenceKind(evidence.Kind) {
			out = append(out, evidence)
		}
	}
	return out
}

func taskEdges(row taskRowView) ([]int64, []int64) {
	if row.Briefing != nil {
		return row.Briefing.Upstream, row.Briefing.Downstream
	}
	return row.WaitingOn, nil
}

func taskStateLine(row taskRowView) string {
	switch row.StateClass {
	case "done":
		return "completed"
	case "claimed":
		return "in progress"
	case "ready":
		return "ready"
	case "review":
		return "review ready"
	case "held", "failed", "waiting":
		return firstNonEmpty(row.StateDetail, row.StateLabel)
	default:
		return row.StateLabel
	}
}

func contextLineCount(briefing *DeveloperBriefing) int {
	if briefing == nil {
		return 0
	}
	count := 0
	if briefing.ContextFold != nil {
		if briefing.ContextFold.Invariant != "" {
			count++
		}
		if len(briefing.ContextFold.NonGoals) > 0 {
			count++
		}
		if len(briefing.ContextFold.SuccessObligations) > 0 {
			count++
		}
	}
	if briefing.Boundary != nil && boundarySummary(briefing.Boundary) != "" {
		count++
	}
	if len(briefing.ObligationClaims) > 0 {
		count++
	}
	if briefing.ObligationCoverage != nil {
		count++
	}
	if len(briefing.Warnings) > 0 {
		count++
	}
	return count
}

func renderContextRows(sb *strings.Builder, briefing *DeveloperBriefing) {
	if briefing.ContextFold != nil {
		if briefing.ContextFold.Invariant != "" {
			renderContextRow(sb, "invariant", strings.ReplaceAll(briefing.ContextFold.Invariant, "\n", " | "))
		}
		if len(briefing.ContextFold.NonGoals) > 0 {
			renderContextRow(sb, "non-goals", strings.Join(briefing.ContextFold.NonGoals, "; "))
		}
		if len(briefing.ContextFold.SuccessObligations) > 0 {
			renderContextRow(sb, "success obligations", strings.Join(briefing.ContextFold.SuccessObligations, ","))
		}
	}
	if briefing.Boundary != nil {
		if summary := boundarySummary(briefing.Boundary); summary != "" {
			renderContextRow(sb, "boundary", summary)
		}
	}
	if len(briefing.ObligationClaims) > 0 {
		renderContextRow(sb, "obligation claims", strings.Join(briefing.ObligationClaims, ","))
	}
	if briefing.ObligationCoverage != nil {
		renderContextRow(sb, "success coverage", fmt.Sprintf("required=%s claimed=%s missing=%s unmatched=%s",
			joinStringsOrNone(briefing.ObligationCoverage.Required),
			joinStringsOrNone(briefing.ObligationCoverage.Claimed),
			joinStringsOrNone(briefing.ObligationCoverage.Missing),
			joinStringsOrNone(briefing.ObligationCoverage.UnmatchedClaims)))
	}
	if len(briefing.Warnings) > 0 {
		renderContextRow(sb, "warnings", strings.Join(briefing.Warnings, "; "))
	}
}

func renderContextRow(sb *strings.Builder, label string, value string) {
	fmt.Fprintf(sb, `<div class="row"><span>%s</span><span>%s</span></div>`, html.EscapeString(label), html.EscapeString(value))
}
