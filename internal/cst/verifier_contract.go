package cst

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var (
	contractGitRefRE    = regexp.MustCompile(`^git:[0-9a-fA-F]{7,64}:.+$`)
	contractPathAtShaRE = regexp.MustCompile(`^.+@[0-9a-fA-F]{7,64}$`)
	contractURLAtVerRE  = regexp.MustCompile(`^https?://[^@\s]+@\S+$`)
	contractSHA256RE    = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)
)

type verifierContractEvidence struct {
	CanonicalSource      verifierContractSource      `json:"canonical_source"`
	ContractArtifacts    []verifierContractArtifact  `json:"contract_artifacts"`
	VerifierScripts      []verifierContractArtifact  `json:"verifier_scripts"`
	Manifest             verifierContractManifest    `json:"manifest"`
	CheapestPlausibleLie string                      `json:"cheapest_plausible_lie"`
	RedCaseRuns          []verifierContractRedRun    `json:"red_case_runs"`
	BlindSpots           []verifierContractBlindSpot `json:"blind_spots"`
}

type verifierContractSource struct {
	Ref         string `json:"ref"`
	Description string `json:"description,omitempty"`
}

type verifierContractArtifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type verifierContractManifest struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Count  int    `json:"count"`
}

type verifierContractRedRun struct {
	Name         string `json:"name"`
	DiffPath     string `json:"diff_path"`
	DiffSHA256   string `json:"diff_sha256"`
	Command      string `json:"command"`
	ExpectedExit int    `json:"expected_exit"`
	ObservedExit int    `json:"observed_exit"`
	StderrPath   string `json:"stderr_path"`
	StdoutSHA256 string `json:"stdout_sha256,omitempty"`
	StderrSHA256 string `json:"stderr_sha256"`
}

type verifierContractBlindSpot struct {
	Axis   string `json:"axis"`
	Reason string `json:"reason"`
	Review string `json:"review"`
}

func validateVerifierContractEvidence(raw json.RawMessage) error {
	if len(raw) == 0 {
		return fmt.Errorf("verifier_contract evidence requires --data")
	}
	var v verifierContractEvidence
	if err := json.Unmarshal(raw, &v); err != nil {
		return fmt.Errorf("verifier_contract evidence data must be an object: %w", err)
	}
	if !validCanonicalRef(v.CanonicalSource.Ref) {
		return fmt.Errorf("verifier_contract canonical_source.ref must be git:<sha>:<path>, path@<sha>, or url@<version>")
	}
	if err := validateArtifactList("contract_artifacts", v.ContractArtifacts); err != nil {
		return err
	}
	if err := validateArtifactList("verifier_scripts", v.VerifierScripts); err != nil {
		return err
	}
	if strings.TrimSpace(v.Manifest.Path) == "" {
		return fmt.Errorf("verifier_contract manifest.path is required")
	}
	if !validSHA256(v.Manifest.SHA256) {
		return fmt.Errorf("verifier_contract manifest.sha256 must be sha256 hex")
	}
	if v.Manifest.Count < 0 {
		return fmt.Errorf("verifier_contract manifest.count cannot be negative")
	}
	if strings.TrimSpace(v.CheapestPlausibleLie) == "" {
		return fmt.Errorf("verifier_contract cheapest_plausible_lie is required")
	}
	if len(v.RedCaseRuns) == 0 {
		return fmt.Errorf("verifier_contract red_case_runs must be non-empty")
	}
	for i, run := range v.RedCaseRuns {
		if strings.TrimSpace(run.Name) == "" {
			return fmt.Errorf("verifier_contract red_case_runs[%d].name is required", i)
		}
		if strings.TrimSpace(run.DiffPath) == "" {
			return fmt.Errorf("verifier_contract red_case_runs[%d].diff_path is required", i)
		}
		if !validSHA256(run.DiffSHA256) {
			return fmt.Errorf("verifier_contract red_case_runs[%d].diff_sha256 must be sha256 hex", i)
		}
		if strings.TrimSpace(run.Command) == "" {
			return fmt.Errorf("verifier_contract red_case_runs[%d].command is required", i)
		}
		if run.ExpectedExit == 0 || run.ObservedExit == 0 {
			return fmt.Errorf("verifier_contract red_case_runs[%d] must record a failing expected_exit and observed_exit", i)
		}
		if strings.TrimSpace(run.StderrPath) == "" {
			return fmt.Errorf("verifier_contract red_case_runs[%d].stderr_path is required", i)
		}
		if !validSHA256(run.StderrSHA256) {
			return fmt.Errorf("verifier_contract red_case_runs[%d].stderr_sha256 must be sha256 hex", i)
		}
		if run.StdoutSHA256 != "" && !validSHA256(run.StdoutSHA256) {
			return fmt.Errorf("verifier_contract red_case_runs[%d].stdout_sha256 must be sha256 hex", i)
		}
	}
	for i, spot := range v.BlindSpots {
		if strings.TrimSpace(spot.Axis) == "" || strings.TrimSpace(spot.Reason) == "" || strings.TrimSpace(spot.Review) == "" {
			return fmt.Errorf("verifier_contract blind_spots[%d] requires axis, reason, and review", i)
		}
	}
	return nil
}

func validCanonicalRef(ref string) bool {
	ref = strings.TrimSpace(ref)
	return contractGitRefRE.MatchString(ref) || contractPathAtShaRE.MatchString(ref) || contractURLAtVerRE.MatchString(ref)
}

func validateArtifactList(field string, artifacts []verifierContractArtifact) error {
	if len(artifacts) == 0 {
		return fmt.Errorf("verifier_contract %s must be non-empty", field)
	}
	for i, artifact := range artifacts {
		if strings.TrimSpace(artifact.Path) == "" {
			return fmt.Errorf("verifier_contract %s[%d].path is required", field, i)
		}
		if !validSHA256(artifact.SHA256) {
			return fmt.Errorf("verifier_contract %s[%d].sha256 must be sha256 hex", field, i)
		}
	}
	return nil
}

func validSHA256(value string) bool {
	return contractSHA256RE.MatchString(strings.TrimSpace(value))
}
