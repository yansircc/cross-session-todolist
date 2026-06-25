package cst

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// RunResult captures the outcome of a single shell run, suitable for storing
// in a script_run event.
type RunResult struct {
	EventID                       string       `json:"event_id,omitempty"`
	Cmd                           string       `json:"cmd"`
	Trigger                       string       `json:"trigger"`
	CheckName                     string       `json:"check_name,omitempty"`
	StartedAt                     time.Time    `json:"started_at"`
	DurationMs                    int64        `json:"duration_ms"`
	ExitCode                      int          `json:"exit_code"`
	StdoutHead                    string       `json:"stdout_head,omitempty"`
	StderrHead                    string       `json:"stderr_head,omitempty"`
	Truncated                     bool         `json:"truncated,omitempty"`
	StoreID                       string       `json:"store_id,omitempty"`
	ExecCWD                       string       `json:"exec_cwd,omitempty"`
	GitAvailable                  bool         `json:"git_available"`
	GitRoot                       string       `json:"git_root,omitempty"`
	GitHead                       string       `json:"git_head,omitempty"`
	GitBranch                     string       `json:"git_branch,omitempty"`
	GitStatusShort                string       `json:"git_status_short,omitempty"`
	StagedDiffSHA256              string       `json:"staged_diff_sha256,omitempty"`
	UnstagedDiffSHA256            string       `json:"unstaged_diff_sha256,omitempty"`
	UntrackedManifestSHA256       string       `json:"untracked_manifest_sha256,omitempty"`
	GitIdentityDigest             string       `json:"git_identity_digest,omitempty"`
	ExecSurface                   string       `json:"exec_surface,omitempty"`
	OwnedPaths                    []string     `json:"owned_paths,omitempty"`
	ScopedGitStatusShort          string       `json:"scoped_git_status_short,omitempty"`
	ScopedStagedDiffSHA256        string       `json:"scoped_staged_diff_sha256,omitempty"`
	ScopedUnstagedDiffSHA256      string       `json:"scoped_unstaged_diff_sha256,omitempty"`
	ScopedUntrackedManifestSHA256 string       `json:"scoped_untracked_manifest_sha256,omitempty"`
	ScopedDigest                  string       `json:"scoped_digest,omitempty"`
	OutOfScopeGitStatusShort      string       `json:"out_of_scope_git_status_short,omitempty"`
	OutOfScopeDeltaCount          int          `json:"out_of_scope_delta_count,omitempty"`
	OutOfScopeDigest              string       `json:"out_of_scope_digest,omitempty"`
	WholeRepoDigest               string       `json:"whole_repo_digest,omitempty"`
	ParallelWorktree              string       `json:"parallel_worktree,omitempty"`
	ExecContextDigest             string       `json:"exec_context_digest,omitempty"`
	StdoutArtifact                *ArtifactRef `json:"stdout_artifact,omitempty"`
	StderrArtifact                *ArtifactRef `json:"stderr_artifact,omitempty"`
	TimedOut                      bool         `json:"timed_out,omitempty"`
	StartError                    error        `json:"start_error,omitempty"`
	ArtifactError                 error        `json:"artifact_error,omitempty"`
}

// LeaseRenewer is invoked periodically while a long shell command is running.
// Implementations should append a claim_renewed event under the store lock.
// May return an error to signal renewal failure (the caller logs but does not
// abort the running command).
type LeaseRenewer func(now time.Time) error

// RunOpts configures a single shell execution.
type RunOpts struct {
	EventID     string
	Cmd         string
	Trigger     string
	CheckName   string
	Timeout     time.Duration
	ExecCWD     string
	StoreID     string
	ArtifactDir string
	Envelope    ExecutionEnvelope

	StdoutMaxBytes int
	StderrMaxBytes int

	// Optional: invoked every RenewEvery while the command runs. nil to skip.
	Renew      LeaseRenewer
	RenewEvery time.Duration
}

// Run executes a shell command via /bin/sh -c, captures bounded output,
// optionally renews the caller's claim lease, and reports a structured result.
func Run(opts RunOpts) RunResult {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	stdoutMax := opts.StdoutMaxBytes
	if stdoutMax <= 0 {
		stdoutMax = 4096
	}
	stderrMax := opts.StderrMaxBytes
	if stderrMax <= 0 {
		stderrMax = 4096
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	execCWD := opts.ExecCWD
	if execCWD == "" {
		if cwd, err := os.Getwd(); err == nil {
			execCWD = cwd
		}
	}
	if abs, err := filepath.Abs(execCWD); err == nil {
		execCWD = abs
	}
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", opts.Cmd)
	cmd.Dir = execCWD
	stdoutBuf := newCappedBuffer(stdoutMax)
	stderrBuf := newCappedBuffer(stderrMax)
	var stdoutFull bytes.Buffer
	var stderrFull bytes.Buffer
	cmd.Stdout = io.MultiWriter(stdoutBuf, &stdoutFull)
	cmd.Stderr = io.MultiWriter(stderrBuf, &stderrFull)

	start := time.Now()
	eventID := opts.EventID
	if eventID == "" {
		eventID = NewEventID()
	}
	res := RunResult{
		EventID:   eventID,
		Cmd:       opts.Cmd,
		Trigger:   opts.Trigger,
		CheckName: opts.CheckName,
		StartedAt: start,
		StoreID:   opts.StoreID,
		ExecCWD:   execCWD,
	}

	if err := cmd.Start(); err != nil {
		res.StartError = err
		res.ExitCode = -1
		res.DurationMs = time.Since(start).Milliseconds()
		identity := CaptureExecIdentity(execCWD, opts.Envelope)
		identity.ExecContextDigest = execContextDigest(opts.StoreID, identity)
		copyExecIdentityToRunResult(&res, identity)
		return res
	}

	var stopRenew chan struct{}
	var renewWG sync.WaitGroup
	if opts.Renew != nil && opts.RenewEvery > 0 {
		stopRenew = make(chan struct{})
		renewWG.Add(1)
		go func() {
			defer renewWG.Done()
			ticker := time.NewTicker(opts.RenewEvery)
			defer ticker.Stop()
			for {
				select {
				case <-stopRenew:
					return
				case t := <-ticker.C:
					_ = opts.Renew(t)
				}
			}
		}()
	}

	err := cmd.Wait()
	if stopRenew != nil {
		close(stopRenew)
		renewWG.Wait()
	}
	res.DurationMs = time.Since(start).Milliseconds()
	identity := CaptureExecIdentity(execCWD, opts.Envelope)
	identity.ExecContextDigest = execContextDigest(opts.StoreID, identity)
	copyExecIdentityToRunResult(&res, identity)

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		res.TimedOut = true
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			res.ExitCode = exitErr.ExitCode()
		} else if res.TimedOut {
			res.ExitCode = 124
		} else {
			res.ExitCode = -1
			res.StartError = err
		}
	} else {
		res.ExitCode = 0
	}

	res.StdoutHead, _ = stdoutBuf.Head()
	res.StderrHead, _ = stderrBuf.Head()
	res.Truncated = stdoutBuf.Truncated() || stderrBuf.Truncated()
	if opts.ArtifactDir != "" {
		stdoutArtifact, err := writeRunArtifact(opts.ArtifactDir, eventID, "stdout", stdoutFull.Bytes())
		if err != nil {
			res.ArtifactError = err
			return res
		}
		stderrArtifact, err := writeRunArtifact(opts.ArtifactDir, eventID, "stderr", stderrFull.Bytes())
		if err != nil {
			res.ArtifactError = err
			return res
		}
		res.StdoutArtifact = stdoutArtifact
		res.StderrArtifact = stderrArtifact
	}
	return res
}

func copyExecIdentityToRunResult(res *RunResult, identity ExecIdentity) {
	res.ExecCWD = identity.ExecCWD
	res.GitAvailable = identity.GitAvailable
	res.GitRoot = identity.GitRoot
	res.GitHead = identity.GitHead
	res.GitBranch = identity.GitBranch
	res.GitStatusShort = identity.GitStatusShort
	res.StagedDiffSHA256 = identity.StagedDiffSHA256
	res.UnstagedDiffSHA256 = identity.UnstagedDiffSHA256
	res.UntrackedManifestSHA256 = identity.UntrackedManifestSHA256
	res.GitIdentityDigest = identity.GitIdentityDigest
	res.ExecSurface = identity.ExecSurface
	res.OwnedPaths = append([]string(nil), identity.OwnedPaths...)
	res.ScopedGitStatusShort = identity.ScopedGitStatusShort
	res.ScopedStagedDiffSHA256 = identity.ScopedStagedDiffSHA256
	res.ScopedUnstagedDiffSHA256 = identity.ScopedUnstagedDiffSHA256
	res.ScopedUntrackedManifestSHA256 = identity.ScopedUntrackedManifestSHA256
	res.ScopedDigest = identity.ScopedDigest
	res.OutOfScopeGitStatusShort = identity.OutOfScopeGitStatusShort
	res.OutOfScopeDeltaCount = identity.OutOfScopeDeltaCount
	res.OutOfScopeDigest = identity.OutOfScopeDigest
	res.WholeRepoDigest = identity.WholeRepoDigest
	res.ParallelWorktree = identity.ParallelWorktree
	res.ExecContextDigest = identity.ExecContextDigest
}

func writeRunArtifact(dir string, eventID string, suffix string, data []byte) (*ArtifactRef, error) {
	if len(data) == 0 {
		return nil, nil
	}
	if !safeArtifactNamePart(eventID) {
		return nil, errors.New("artifact event_id must be a single path segment")
	}
	if suffix != "stdout" && suffix != "stderr" {
		return nil, errors.New("artifact suffix must be stdout or stderr")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	name := eventID + "." + suffix
	full := filepath.Join(dir, name)
	if err := writeFileAtomically(full, data, 0o644); err != nil {
		return nil, err
	}
	ref := &ArtifactRef{
		Path:     filepath.ToSlash(filepath.Join("artifacts", "runs", name)),
		SHA256:   sha256Hex(data),
		ByteSize: int64(len(data)),
	}
	if err := verifyRunArtifactRef(full, ref); err != nil {
		return nil, err
	}
	return ref, nil
}

func safeArtifactNamePart(part string) bool {
	part = strings.TrimSpace(part)
	return part != "" && part != "." && part != ".." && !strings.ContainsAny(part, `/\`)
}

func verifyRunArtifactRef(path string, ref *ArtifactRef) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if int64(len(data)) != ref.ByteSize {
		return errors.New("artifact size verification failed")
	}
	if sha256Hex(data) != ref.SHA256 {
		return errors.New("artifact sha256 verification failed")
	}
	return nil
}

// cappedBuffer collects up to N bytes; further writes are counted but
// dropped, recording that truncation happened.
type cappedBuffer struct {
	max int
	buf bytes.Buffer
	mu  sync.Mutex
	cut atomic.Bool
}

func newCappedBuffer(max int) *cappedBuffer { return &cappedBuffer{max: max} }

func (c *cappedBuffer) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	remain := c.max - c.buf.Len()
	if remain <= 0 {
		c.cut.Store(true)
		return len(p), nil
	}
	if len(p) > remain {
		c.buf.Write(p[:remain])
		c.cut.Store(true)
		return len(p), nil
	}
	return c.buf.Write(p)
}

func (c *cappedBuffer) Head() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String(), nil
}

func (c *cappedBuffer) Truncated() bool { return c.cut.Load() }
