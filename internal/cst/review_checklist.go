package cst

import (
	"encoding/json"
	"fmt"
)

type reviewChecklistEvidence struct {
	Items      []reviewChecklistItem       `json:"items"`
	BlindSpots []verifierContractBlindSpot `json:"blind_spots,omitempty"`
}

type reviewChecklistItem struct {
	ID        string `json:"id"`
	Criterion string `json:"criterion"`
	Status    string `json:"status"`
	Evidence  string `json:"evidence"`
}

func validateReviewChecklistEvidence(raw json.RawMessage) error {
	if len(raw) == 0 {
		return fmt.Errorf("review_checklist evidence missing data")
	}
	var data reviewChecklistEvidence
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("review_checklist data must be JSON object: %w", err)
	}
	if len(data.Items) == 0 {
		return fmt.Errorf("review_checklist requires items")
	}
	seen := map[string]bool{}
	for _, item := range data.Items {
		if item.ID == "" || item.Criterion == "" || item.Status == "" || item.Evidence == "" {
			return fmt.Errorf("review_checklist item missing id, criterion, status, or evidence")
		}
		if seen[item.ID] {
			return fmt.Errorf("review_checklist repeats item id %q", item.ID)
		}
		seen[item.ID] = true
		switch item.Status {
		case "pass", "fail", "na":
		default:
			return fmt.Errorf("review_checklist item %q has invalid status %q", item.ID, item.Status)
		}
	}
	for _, spot := range data.BlindSpots {
		if spot.Axis == "" || spot.Reason == "" || spot.Review == "" {
			return fmt.Errorf("review_checklist blind_spots entries require axis, reason, and review")
		}
	}
	return nil
}
