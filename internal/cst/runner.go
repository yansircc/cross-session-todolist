package cst

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// RunResult captures the outcome of a single shell run, suitable for storing
// in a script_run event.
type RunResult struct {
	EventID                 string       `json:"event_id,omitempty"`
	Cmd                     string       `json:"cmd"`
	Trigger                 string       `json:"trigger"`
	CheckName               string       `json:"check_name,omitempty"`
	StartedAt               time.Time    `json:"started_at"`
	DurationMs              int64        `json:"duration_ms"`
	ExitCode                int          `json:"exit_code"`
	StdoutHead              string       `json:"stdout_head,omitempty"`
	StderrHead              string       `json:"stderr_head,omitempty"`
	Truncated               bool         `json:"truncated,omitempty"`
	StoreID                 string       `json:"store_id,omitempty"`
	ExecCWD                 string       `json:"exec_cwd,omitempty"`
	GitAvailable            bool         `json:"git_available"`
	GitRoot                 string       `json:"git_root,omitempty"`
	GitHead                 string       `json:"git_head,omitempty"`
	GitBranch               string       `json:"git_branch,omitempty"`
	GitStatusShort          string       `json:"git_status_short,omitempty"`
	StagedDiffSHA256        string       `json:"staged_diff_sha256,omitempty"`
	UnstagedDiffSHA256      string       `json:"unstaged_diff_sha256,omitempty"`
	UntrackedManifestSHA256 string       `json:"untracked_manifest_sha256,omitempty"`
	GitIdentityDigest       string       `json:"git_identity_digest,omitempty"`
	ParallelWorktree        string       `json:"parallel_worktree,omitempty"`
	ExecContextDigest       string       `json:"exec_context_digest,omitempty"`
	StdoutArtifact          *ArtifactRef `json:"stdout_artifact,omitempty"`
	StderrArtifact          *ArtifactRef `json:"stderr_artifact,omitempty"`
	TimedOut                bool         `json:"timed_out,omitempty"`
	StartError              error        `json:"start_error,omitempty"`
	ArtifactError           error        `json:"artifact_error,omitempty"`
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
	identity := CaptureExecIdentity(execCWD)
	identity.ExecContextDigest = execContextDigest(opts.StoreID, identity)

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
		EventID:                 eventID,
		Cmd:                     opts.Cmd,
		Trigger:                 opts.Trigger,
		CheckName:               opts.CheckName,
		StartedAt:               start,
		StoreID:                 opts.StoreID,
		ExecCWD:                 identity.ExecCWD,
		GitAvailable:            identity.GitAvailable,
		GitRoot:                 identity.GitRoot,
		GitHead:                 identity.GitHead,
		GitBranch:               identity.GitBranch,
		GitStatusShort:          identity.GitStatusShort,
		StagedDiffSHA256:        identity.StagedDiffSHA256,
		UnstagedDiffSHA256:      identity.UnstagedDiffSHA256,
		UntrackedManifestSHA256: identity.UntrackedManifestSHA256,
		GitIdentityDigest:       identity.GitIdentityDigest,
		ParallelWorktree:        identity.ParallelWorktree,
		ExecContextDigest:       identity.ExecContextDigest,
	}

	if err := cmd.Start(); err != nil {
		res.StartError = err
		res.ExitCode = -1
		res.DurationMs = time.Since(start).Milliseconds()
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
		res.StdoutArtifact, res.ArtifactError = writeRunArtifact(opts.ArtifactDir, eventID, "stdout", stdoutFull.Bytes())
		if res.ArtifactError == nil {
			res.StderrArtifact, res.ArtifactError = writeRunArtifact(opts.ArtifactDir, eventID, "stderr", stderrFull.Bytes())
		}
	}
	return res
}

func writeRunArtifact(dir string, eventID string, suffix string, data []byte) (*ArtifactRef, error) {
	if len(data) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	name := eventID + "." + suffix
	full := filepath.Join(dir, name)
	if err := os.WriteFile(full, data, 0o644); err != nil {
		return nil, err
	}
	return &ArtifactRef{
		Path:     filepath.ToSlash(filepath.Join("artifacts", "runs", name)),
		SHA256:   sha256Hex(data),
		ByteSize: int64(len(data)),
	}, nil
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
