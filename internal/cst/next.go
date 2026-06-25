package cst

import (
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	NextPhaseInit          = "init"
	NextPhaseReconcile     = "reconcile"
	NextPhaseFixBoundary   = "fix-boundary"
	NextPhaseFixObligation = "fix-obligation"
	NextPhaseWork          = "work"
	NextPhaseComplete      = "complete"
	NextPhaseNoOp          = "no-op"
)

type NextInput struct {
	State     *State
	StoreRoot string
	StoreID   string
	Revision  Revision
	Actor     string
	Now       time.Time
}

type NextView struct {
	Revision          Revision                `json:"revision"`
	StoreRoot         string                  `json:"store_root"`
	StoreID           string                  `json:"store_id,omitempty"`
	Actor             string                  `json:"actor"`
	Phase             string                  `json:"phase"`
	Required          string                  `json:"required,omitempty"`
	Briefing          *DeveloperBriefing      `json:"briefing,omitempty"`
	Action            *BoundAction            `json:"action,omitempty"`
	Repair            *RepairContract         `json:"repair,omitempty"`
	WorkerStatus      *WorkerStatusView       `json:"worker_status,omitempty"`
	UnreconciledDiffs []UnreconciledDiffEntry `json:"unreconciled_diffs,omitempty"`
}

type RepairContract struct {
	Phase         string   `json:"phase"`
	RequiredInput string   `json:"required_input,omitempty"`
	Commands      []string `json:"commands"`
	Explanation   string   `json:"explanation,omitempty"`
}

type UnreconciledDiffEntry struct {
	Code string `json:"code"`
	Path string `json:"path"`
}

func NextInputFromTx(tx *Tx) NextInput {
	return NextInput{
		State:     tx.state,
		StoreRoot: tx.paths.Root,
		StoreID:   tx.StoreID(),
		Revision:  tx.state.Revision,
		Actor:     tx.actor,
		Now:       tx.now,
	}
}

func BuildNextView(input NextInput) (NextView, error) {
	if input.State == nil {
		return NextView{}, herr(ExitInvariantBroken, "next requires state")
	}
	view := NextView{
		Revision:  input.Revision,
		StoreRoot: input.StoreRoot,
		StoreID:   input.StoreID,
		Actor:     input.Actor,
	}
	root := input.State.Root()
	if root == nil {
		view.Phase = NextPhaseInit
		view.Required = "input"
		view.Repair = &RepairContract{
			Phase:         NextPhaseInit,
			RequiredInput: "project intent",
			Commands:      []string{`cst add --intent "<project goal>"`},
			Explanation:   "A CST store has one root goal for life; create it before adding work.",
		}
		return view, nil
	}

	unreconciled := UnreconciledDiffs(input.State, input.StoreRoot)
	if len(unreconciled) > 0 {
		view.Phase = NextPhaseReconcile
		view.Required = "reconcile-first"
		view.UnreconciledDiffs = unreconciled
		view.Repair = reconcileRepair(root.ID, unreconciled)
		return view, nil
	}

	if claim := currentActorClaim(input.State, input.Actor); claim != nil {
		return nextForClaim(input, view, claim)
	}

	ready := input.State.HeadOpenTasks(1)
	if len(ready) > 0 {
		return nextForReadyTask(input, view, ready[0])
	}

	coverage := input.State.ObligationCoverage(root.ID)
	if len(coverage.Missing) > 0 {
		view.Phase = NextPhaseFixObligation
		view.Required = "repair"
		view.Repair = obligationRepair(root.ID, coverage.Missing)
		return view, nil
	}

	if input.State.NodeStatus(root) == StatusCompleted {
		view.Phase = NextPhaseNoOp
		return view, nil
	}

	view.Phase = NextPhaseWork
	view.Required = "blocked"
	view.Repair = &RepairContract{
		Phase:       NextPhaseWork,
		Commands:    []string{"cst brief"},
		Explanation: "No legal action is currently projected; inspect the bounded frontier.",
	}
	return view, nil
}

func nextForClaim(input NextInput, view NextView, task *Node) (NextView, error) {
	if repair := taskObligationRepair(input.State, task); repair != nil {
		view.Phase = NextPhaseFixObligation
		view.Required = "repair"
		view.Briefing = BuildDeveloperBriefing(input.State, task.ID)
		view.Repair = repair
		return view, nil
	}
	status, err := BuildWorkerStatus(FrontierInput{
		State:     input.State,
		StoreRoot: input.StoreRoot,
		StoreID:   input.StoreID,
		Revision:  input.Revision,
		Actor:     input.Actor,
		Now:       input.Now,
		TaskID:    task.ID,
	})
	if err != nil {
		return NextView{}, err
	}
	action := recommendedAction(status.Actions)
	view.WorkerStatus = &status
	view.Briefing = status.Briefing
	if action != nil {
		view.Action = action
		if action.Kind == ActionCompleteFromAcceptance || action.Kind == ActionCompleteReviewWithEvidence {
			view.Phase = NextPhaseComplete
		} else {
			view.Phase = NextPhaseWork
		}
		return view, nil
	}
	view.Phase = NextPhaseWork
	view.Required = "input"
	view.Repair = &RepairContract{
		Phase:         NextPhaseWork,
		RequiredInput: "evidence or probe command",
		Commands: []string{
			fmt.Sprintf(`cst run %d --cmd "<probe command>"`, task.ID),
			fmt.Sprintf(`cst evidence %d --kind note --summary "<review evidence>"`, task.ID),
		},
		Explanation: "The active claim has no currently executable bound action.",
	}
	return view, nil
}

func nextForReadyTask(input NextInput, view NextView, task *Node) (NextView, error) {
	view.Briefing = BuildDeveloperBriefing(input.State, task.ID)
	if repair := taskObligationRepair(input.State, task); repair != nil {
		view.Phase = NextPhaseFixObligation
		view.Required = "repair"
		view.Repair = repair
		return view, nil
	}
	action := recommendedAction(LegalFrontier(FrontierInput{
		State:     input.State,
		StoreRoot: input.StoreRoot,
		StoreID:   input.StoreID,
		Revision:  input.Revision,
		Actor:     input.Actor,
		Now:       input.Now,
		TaskID:    task.ID,
	}))
	if action == nil {
		view.Phase = NextPhaseWork
		view.Required = "blocked"
		return view, nil
	}
	view.Phase = NextPhaseWork
	view.Action = action
	return view, nil
}

func UnreconciledDiffs(s *State, repoRoot string) []UnreconciledDiffEntry {
	if s == nil || repoRoot == "" {
		return nil
	}
	var out []UnreconciledDiffEntry
	for _, entry := range statusEntries(repoRoot) {
		path := strings.TrimSpace(entry.Path)
		if path == "" || isCSTInternalPath(path) {
			continue
		}
		if !activeTaskBoundaryCoversPath(s, path) {
			out = append(out, UnreconciledDiffEntry{Code: entry.Code, Path: path})
		}
	}
	return out
}

func activeTaskBoundaryCoversPath(s *State, path string) bool {
	for _, id := range s.Order {
		n := s.Nodes[id]
		if n == nil || n.Kind != KindTask || n.Terminal() || n.Boundary == nil || len(n.Boundary.Owned) == 0 {
			continue
		}
		if pathInOwnedPaths(n.Boundary.Owned, path) && !pathInOwnedPaths(n.Boundary.Excluded, path) {
			return true
		}
	}
	return false
}

func currentActorClaim(s *State, actor string) *Node {
	claims := s.CurrentClaims()
	for _, n := range claims {
		if n.Claim != nil && n.Claim.Actor == actor {
			return n
		}
	}
	return nil
}

func recommendedAction(actions []BoundAction) *BoundAction {
	for _, kind := range []string{
		ActionCompleteFromAcceptance,
		ActionCompleteReviewWithEvidence,
		ActionRunAcceptance,
		ActionTakeReadyTask,
	} {
		for i := range actions {
			if actions[i].Kind == kind {
				a := actions[i]
				return &a
			}
		}
	}
	return nil
}

func taskObligationRepair(s *State, task *Node) *RepairContract {
	if task == nil {
		return nil
	}
	coverage := s.ObligationCoverage(task.ID)
	if len(coverage.Missing) == 0 {
		return nil
	}
	return obligationRepairForTask(task, coverage.Missing)
}

func obligationRepairForTask(task *Node, missing []string) *RepairContract {
	commands := make([]string, 0, len(missing))
	for _, obligation := range missing {
		commands = append(commands, fmt.Sprintf(`cst revise %d --obligation-claim %s --reason "claim required success obligation"`, task.ID, obligation))
	}
	if task.Claim != nil {
		commands = append([]string{fmt.Sprintf("cst release %d", task.ID)}, commands...)
		commands = append(commands, fmt.Sprintf("cst take %d", task.ID))
	}
	return &RepairContract{
		Phase:         NextPhaseFixObligation,
		RequiredInput: "task obligation claim",
		Commands:      commands,
		Explanation:   "Named success obligations must be covered by descendant leaf task obligation claims.",
	}
}

func obligationRepair(parentID int64, missing []string) *RepairContract {
	commands := make([]string, 0, len(missing))
	for _, obligation := range missing {
		commands = append(commands, fmt.Sprintf(`cst add --parent %d --intent "<task intent>" --owned <repo-relative-path> --obligation-claim %s --check <name>="<verification command>"`, parentID, obligation))
	}
	return &RepairContract{
		Phase:         NextPhaseFixObligation,
		RequiredInput: "task obligation claim",
		Commands:      commands,
		Explanation:   "Named success obligations must be covered by descendant leaf task obligation claims.",
	}
}

func reconcileRepair(parentID int64, diffs []UnreconciledDiffEntry) *RepairContract {
	path := "<repo-relative-path>"
	if len(diffs) > 0 {
		path = diffs[0].Path
	}
	return &RepairContract{
		Phase:         NextPhaseReconcile,
		RequiredInput: "task intent and verification command",
		Commands: []string{
			fmt.Sprintf(`cst add --parent %d --intent "<describe current work>" --owned %s --check <name>="<verification command>"`, parentID, path),
		},
		Explanation: "Uncommitted work must be covered by an active task node.boundary.owned before next projects further procedure.",
	}
}

func isCSTInternalPath(path string) bool {
	path = strings.Trim(strings.TrimPrefix(path, "./"), "/")
	return path == StoreDirName || strings.HasPrefix(path, StoreDirName+"/")
}

func DoNext(out io.Writer, asJSON bool) error {
	return WithStore(TxOpts{Mutating: false, RepairLease: true}, func(tx *Tx) error {
		view, err := BuildNextView(NextInputFromTx(tx))
		if err != nil {
			return err
		}
		if asJSON {
			WriteJSON(out, view)
			return nil
		}
		RenderNextText(out, view)
		return nil
	})
}

func RenderNextText(w io.Writer, view NextView) {
	fmt.Fprintf(w, "next phase=%s\n", view.Phase)
	if view.Required != "" {
		fmt.Fprintf(w, "required: %s\n", view.Required)
	}
	if view.Action != nil {
		fmt.Fprintf(w, "action: %s\n", view.Action.Kind)
		if view.Action.Preview != "" {
			fmt.Fprintf(w, "run: %s\n", view.Action.Preview)
		}
	}
	if view.Repair != nil {
		fmt.Fprintf(w, "repair: %s\n", view.Repair.Phase)
		if view.Repair.RequiredInput != "" {
			fmt.Fprintf(w, "required_input: %s\n", view.Repair.RequiredInput)
		}
		for _, cmd := range view.Repair.Commands {
			fmt.Fprintf(w, "  %s\n", cmd)
		}
	}
	if len(view.UnreconciledDiffs) > 0 {
		fmt.Fprintln(w, "unreconciled_diffs:")
		for _, diff := range view.UnreconciledDiffs {
			fmt.Fprintf(w, "  %s %s\n", diff.Code, diff.Path)
		}
	}
	RenderDeveloperBriefingText(w, view.Briefing)
}
