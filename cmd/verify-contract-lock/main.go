package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	gitRefRE    = regexp.MustCompile(`^git:[0-9a-fA-F]{7,64}:.+$`)
	pathAtShaRE = regexp.MustCompile(`^.+@[0-9a-fA-F]{7,64}$`)
	urlAtVerRE  = regexp.MustCompile(`^https?://[^@\s]+@\S+$`)
	sha256RE    = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)
)

type contract struct {
	CanonicalSource      source      `json:"canonical_source"`
	ContractArtifacts    []artifact  `json:"contract_artifacts"`
	VerifierScripts      []artifact  `json:"verifier_scripts"`
	Manifest             manifest    `json:"manifest"`
	CheapestPlausibleLie string      `json:"cheapest_plausible_lie"`
	RedCaseRuns          []redRun    `json:"red_case_runs"`
	BlindSpots           []blindSpot `json:"blind_spots"`
}

type source struct {
	Ref string `json:"ref"`
}

type artifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type manifest struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Count  int    `json:"count"`
}

type redRun struct {
	Name         string `json:"name"`
	DiffPath     string `json:"diff_path"`
	DiffSHA256   string `json:"diff_sha256"`
	Command      string `json:"command"`
	ExpectedExit int    `json:"expected_exit"`
	ObservedExit int    `json:"observed_exit"`
	StderrPath   string `json:"stderr_path"`
	StderrSHA256 string `json:"stderr_sha256"`
}

type blindSpot struct {
	Axis   string `json:"axis"`
	Reason string `json:"reason"`
	Review string `json:"review"`
}

func main() {
	contractPath := flag.String("contract", "", "verifier contract JSON path")
	fixturePath := flag.String("fixture", "", "alias for --contract")
	flag.Parse()
	path := *contractPath
	if path == "" {
		path = *fixturePath
	}
	if path == "" {
		fail("pass --contract <path>")
	}
	if err := verify(path); err != nil {
		fail("%v", err)
	}
}

func verify(path string) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var c contract
	if err := json.Unmarshal(body, &c); err != nil {
		return err
	}
	if !validRef(c.CanonicalSource.Ref) {
		return fmt.Errorf("canonical_source.ref is not stable: %q", c.CanonicalSource.Ref)
	}
	if err := verifyArtifacts(root, "contract_artifacts", c.ContractArtifacts); err != nil {
		return err
	}
	if err := verifyArtifacts(root, "verifier_scripts", c.VerifierScripts); err != nil {
		return err
	}
	if err := verifyHash(root, "manifest", c.Manifest.Path, c.Manifest.SHA256); err != nil {
		return err
	}
	if c.Manifest.Count < 0 {
		return fmt.Errorf("manifest.count cannot be negative")
	}
	if strings.TrimSpace(c.CheapestPlausibleLie) == "" {
		return fmt.Errorf("cheapest_plausible_lie is required")
	}
	if len(c.RedCaseRuns) == 0 {
		return fmt.Errorf("red_case_runs must be non-empty")
	}
	for i, run := range c.RedCaseRuns {
		if strings.TrimSpace(run.Name) == "" {
			return fmt.Errorf("red_case_runs[%d].name is required", i)
		}
		if run.ExpectedExit == 0 || run.ObservedExit == 0 {
			return fmt.Errorf("red_case_runs[%d] must record a failing expected_exit and observed_exit", i)
		}
		if err := verifyHash(root, fmt.Sprintf("red_case_runs[%d].diff", i), run.DiffPath, run.DiffSHA256); err != nil {
			return err
		}
		if err := verifyHash(root, fmt.Sprintf("red_case_runs[%d].stderr", i), run.StderrPath, run.StderrSHA256); err != nil {
			return err
		}
	}
	for i, spot := range c.BlindSpots {
		if strings.TrimSpace(spot.Axis) == "" || strings.TrimSpace(spot.Reason) == "" || strings.TrimSpace(spot.Review) == "" {
			return fmt.Errorf("blind_spots[%d] requires axis, reason, and review", i)
		}
	}
	return nil
}

func verifyArtifacts(root string, field string, artifacts []artifact) error {
	if len(artifacts) == 0 {
		return fmt.Errorf("%s must be non-empty", field)
	}
	for i, a := range artifacts {
		if err := verifyHash(root, fmt.Sprintf("%s[%d]", field, i), a.Path, a.SHA256); err != nil {
			return err
		}
	}
	return nil
}

func verifyHash(root string, label string, path string, want string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("%s path is required", label)
	}
	if !sha256RE.MatchString(want) {
		return fmt.Errorf("%s sha256 is invalid", label)
	}
	fullPath, err := rootedArtifactPath(root, path)
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	got, err := fileSHA256(fullPath)
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("%s hash mismatch for %s: got %s want %s", label, filepath.Clean(path), got, want)
	}
	return nil
}

func rootedArtifactPath(root string, path string) (string, error) {
	path = strings.TrimSpace(path)
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("path %q must be relative to verifier root", path)
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	clean = strings.TrimPrefix(clean, "./")
	if clean == "" || clean == "." {
		return "", fmt.Errorf("path %q must name a file", path)
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("path %q escapes verifier root", path)
	}
	return filepath.Join(root, filepath.FromSlash(clean)), nil
}

func fileSHA256(path string) (string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

func validRef(ref string) bool {
	ref = strings.TrimSpace(ref)
	return gitRefRE.MatchString(ref) || pathAtShaRE.MatchString(ref) || urlAtVerRE.MatchString(ref)
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "verify-contract-lock: "+format+"\n", args...)
	os.Exit(1)
}
