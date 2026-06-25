package cst

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

type ExecutionEnvelopePatch struct {
	ExecCWDSet     bool
	ExecCWD        string
	ExecSurfaceSet bool
	ExecSurface    string
	OwnedPathsSet  bool
	OwnedPaths     []string
}

func normalizeExecutionEnvelope(env *ExecutionEnvelope) (*ExecutionEnvelope, error) {
	if env == nil {
		return nil, nil
	}
	ownedPaths, err := normalizeScopePaths(env.OwnedPaths)
	if err != nil {
		return nil, err
	}
	out := &ExecutionEnvelope{
		ExecSurface: env.ExecSurface,
		OwnedPaths:  ownedPaths,
	}
	if out.ExecSurface == "" {
		out.ExecSurface = ExecSurfaceShared
	}
	if out.ExecSurface != ExecSurfaceShared && out.ExecSurface != ExecSurfacePrivate {
		return nil, fmt.Errorf("exec_surface must be %s or %s", ExecSurfaceShared, ExecSurfacePrivate)
	}
	if env.ExecCWD != "" {
		abs, err := filepath.Abs(env.ExecCWD)
		if err != nil {
			return nil, err
		}
		out.ExecCWD = abs
	}
	if out.ExecSurface == ExecSurfacePrivate && out.ExecCWD == "" {
		return nil, fmt.Errorf("private exec surface requires exec_cwd")
	}
	if out.ExecCWD == "" && len(out.OwnedPaths) == 0 && out.ExecSurface == ExecSurfaceShared {
		return nil, nil
	}
	return out, nil
}

func normalizeScopePaths(paths []string) ([]string, error) {
	out := normalizeOwnedPaths(paths)
	for _, p := range out {
		if filepath.IsAbs(p) {
			return nil, fmt.Errorf("scope path %q must be relative to exec checkout", p)
		}
		if p == ".." || strings.HasPrefix(p, "../") {
			return nil, fmt.Errorf("scope path %q escapes the exec checkout", p)
		}
	}
	return out, nil
}

func effectiveExecutionEnvelope(n *Node) ExecutionEnvelope {
	if n == nil || n.Envelope == nil {
		return ExecutionEnvelope{ExecSurface: ExecSurfaceShared}
	}
	env := cloneExecutionEnvelope(n.Envelope)
	if env.ExecSurface == "" {
		env.ExecSurface = ExecSurfaceShared
	}
	env.OwnedPaths = normalizeOwnedPaths(env.OwnedPaths)
	return *env
}

func mergeExecutionEnvelopePatch(base *ExecutionEnvelope, patch ExecutionEnvelopePatch) (*ExecutionEnvelope, error) {
	env := &ExecutionEnvelope{}
	if base != nil {
		env = cloneExecutionEnvelope(base)
	}
	if env.ExecSurface == "" {
		env.ExecSurface = ExecSurfaceShared
	}
	if patch.ExecCWDSet {
		env.ExecCWD = patch.ExecCWD
		if !patch.ExecSurfaceSet {
			env.ExecSurface = ExecSurfaceShared
		}
	}
	if patch.ExecSurfaceSet {
		env.ExecSurface = patch.ExecSurface
	}
	if patch.OwnedPathsSet {
		env.OwnedPaths = patch.OwnedPaths
	}
	normalized, err := normalizeExecutionEnvelope(env)
	if err != nil {
		return nil, err
	}
	if normalized == nil {
		return &ExecutionEnvelope{ExecSurface: ExecSurfaceShared}, nil
	}
	return normalized, nil
}

func cloneExecutionEnvelope(env *ExecutionEnvelope) *ExecutionEnvelope {
	if env == nil {
		return nil
	}
	return &ExecutionEnvelope{
		ExecCWD:     env.ExecCWD,
		ExecSurface: env.ExecSurface,
		OwnedPaths:  append([]string(nil), env.OwnedPaths...),
	}
}

func normalizeOwnedPaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(paths))
	for _, raw := range paths {
		p := strings.TrimSpace(raw)
		if p == "" {
			continue
		}
		p = filepath.ToSlash(filepath.Clean(p))
		p = strings.TrimPrefix(p, "./")
		if p == "" {
			p = "."
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

func resolveExecCWD(override string, env ExecutionEnvelope) string {
	if override != "" {
		return override
	}
	return env.ExecCWD
}

func ownedPathContains(scope string, path string) bool {
	scope = strings.Trim(strings.TrimPrefix(filepath.ToSlash(filepath.Clean(scope)), "./"), "/")
	path = strings.Trim(strings.TrimPrefix(filepath.ToSlash(filepath.Clean(path)), "./"), "/")
	if scope == "" || scope == "." {
		return true
	}
	return path == scope || strings.HasPrefix(path, scope+"/")
}

func pathsOverlap(a []string, b []string) bool {
	a = normalizeOwnedPaths(a)
	b = normalizeOwnedPaths(b)
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	for _, ap := range a {
		for _, bp := range b {
			if ownedPathContains(ap, bp) || ownedPathContains(bp, ap) {
				return true
			}
		}
	}
	return false
}
