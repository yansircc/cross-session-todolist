package cst

import "fmt"

func NewVerifyAcceptance(cmd string) *Acceptance {
	return &Acceptance{
		Kind:   AcceptanceVerify,
		Checks: []VerifyCheck{{Name: DefaultVerifyCheckName, Cmd: cmd}},
	}
}

func NewVerifyChecksAcceptance(checks []VerifyCheck) *Acceptance {
	return &Acceptance{Kind: AcceptanceVerify, Checks: cloneVerifyChecks(checks)}
}

func (a *Acceptance) VerifyChecks() []VerifyCheck {
	if a == nil || a.Kind != AcceptanceVerify {
		return nil
	}
	if len(a.Checks) > 0 {
		return cloneVerifyChecks(a.Checks)
	}
	if a.Cmd != "" {
		return []VerifyCheck{{Name: DefaultVerifyCheckName, Cmd: a.Cmd}}
	}
	return nil
}

func validateVerifyChecks(nodeID int64, checks []VerifyCheck) error {
	if len(checks) == 0 {
		return fmt.Errorf("task #%d verify acceptance missing checks", nodeID)
	}
	seen := map[string]bool{}
	for _, check := range checks {
		if check.Name == "" {
			return fmt.Errorf("task #%d verify check missing name", nodeID)
		}
		if check.Cmd == "" {
			return fmt.Errorf("task #%d verify check %q missing command", nodeID, check.Name)
		}
		if seen[check.Name] {
			return fmt.Errorf("task #%d repeats verify check %q", nodeID, check.Name)
		}
		seen[check.Name] = true
	}
	return nil
}

func sameVerifyChecks(a, b []VerifyCheck) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func cloneVerifyChecks(checks []VerifyCheck) []VerifyCheck {
	return append([]VerifyCheck(nil), checks...)
}
