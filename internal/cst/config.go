package cst

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	BriefMaxTasks  int
	BriefMaxRules  int
	BriefMaxRecent int

	RunnerDefaultTimeout time.Duration
	RunnerStdoutMaxBytes int
	RunnerStderrMaxBytes int

	ClaimLeaseTTL   time.Duration
	ClaimRenewEvery time.Duration

	ActorDefault string
}

func DefaultConfig() Config {
	return Config{
		BriefMaxTasks:        10,
		BriefMaxRules:        20,
		BriefMaxRecent:       5,
		RunnerDefaultTimeout: 5 * time.Minute,
		RunnerStdoutMaxBytes: 4096,
		RunnerStderrMaxBytes: 4096,
		ClaimLeaseTTL:        10 * time.Minute,
		ClaimRenewEvery:      2 * time.Minute,
		ActorDefault:         "",
	}
}

// LoadConfig reads .cst/config.toml if present. Strict — unknown keys, bad
// values, and structural problems all fail loudly so an Agent never operates
// on phantom config.
func LoadConfig(storeDir string) (Config, error) {
	cfg := DefaultConfig()
	path := filepath.Join(storeDir, "config.toml")
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	var section string
	for lineNo, raw := range strings.Split(string(b), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			if !strings.HasSuffix(line, "]") {
				return cfg, fmt.Errorf("config.toml:%d: bad section header %q", lineNo+1, line)
			}
			section = strings.TrimSpace(line[1 : len(line)-1])
			if !knownSection(section) {
				return cfg, fmt.Errorf("config.toml:%d: unknown section [%s]", lineNo+1, section)
			}
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			return cfg, fmt.Errorf("config.toml:%d: missing '=' in %q", lineNo+1, line)
		}
		if section == "" {
			return cfg, fmt.Errorf("config.toml:%d: assignment outside any [section]", lineNo+1)
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		val = strings.Trim(val, "\"")
		full := section + "." + key
		if err := applyConfigKV(&cfg, full, val); err != nil {
			return cfg, fmt.Errorf("config.toml:%d: %w", lineNo+1, err)
		}
	}
	if err := validateConfig(cfg); err != nil {
		return cfg, fmt.Errorf("config.toml: %w", err)
	}
	return cfg, nil
}

func knownSection(s string) bool {
	switch s {
	case "brief", "runner", "claim", "actor":
		return true
	}
	return false
}

func applyConfigKV(cfg *Config, key, val string) error {
	parseInt := func() (int, error) {
		n, err := strconv.Atoi(val)
		if err != nil {
			return 0, fmt.Errorf("%s expects integer, got %q", key, val)
		}
		return n, nil
	}
	parseSeconds := func() (time.Duration, error) {
		n, err := strconv.Atoi(val)
		if err != nil {
			return 0, fmt.Errorf("%s expects seconds (integer), got %q", key, val)
		}
		if n <= 0 {
			return 0, fmt.Errorf("%s must be positive, got %d", key, n)
		}
		return time.Duration(n) * time.Second, nil
	}
	switch key {
	case "brief.max_tasks":
		n, err := parseInt()
		if err != nil {
			return err
		}
		if n <= 0 {
			return fmt.Errorf("%s must be positive", key)
		}
		cfg.BriefMaxTasks = n
	case "brief.max_rules":
		n, err := parseInt()
		if err != nil {
			return err
		}
		if n <= 0 {
			return fmt.Errorf("%s must be positive", key)
		}
		cfg.BriefMaxRules = n
	case "brief.max_recent":
		n, err := parseInt()
		if err != nil {
			return err
		}
		if n <= 0 {
			return fmt.Errorf("%s must be positive", key)
		}
		cfg.BriefMaxRecent = n
	case "runner.default_timeout_seconds":
		d, err := parseSeconds()
		if err != nil {
			return err
		}
		cfg.RunnerDefaultTimeout = d
	case "runner.stdout_max_bytes":
		n, err := parseInt()
		if err != nil {
			return err
		}
		if n <= 0 {
			return fmt.Errorf("%s must be positive", key)
		}
		cfg.RunnerStdoutMaxBytes = n
	case "runner.stderr_max_bytes":
		n, err := parseInt()
		if err != nil {
			return err
		}
		if n <= 0 {
			return fmt.Errorf("%s must be positive", key)
		}
		cfg.RunnerStderrMaxBytes = n
	case "claim.lease_ttl_seconds":
		d, err := parseSeconds()
		if err != nil {
			return err
		}
		cfg.ClaimLeaseTTL = d
	case "claim.renew_every_seconds":
		d, err := parseSeconds()
		if err != nil {
			return err
		}
		cfg.ClaimRenewEvery = d
	case "actor.default":
		cfg.ActorDefault = val
	default:
		return fmt.Errorf("unknown key %q", key)
	}
	return nil
}

func validateConfig(cfg Config) error {
	if cfg.ClaimRenewEvery >= cfg.ClaimLeaseTTL {
		return fmt.Errorf("claim.renew_every_seconds (%s) must be < claim.lease_ttl_seconds (%s)",
			cfg.ClaimRenewEvery, cfg.ClaimLeaseTTL)
	}
	return nil
}
