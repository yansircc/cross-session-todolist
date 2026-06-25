package cst

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// UIArgs is the parsed flag set for `cst ui`.
type UIArgs struct {
	Within int64
	Output string
	NoOpen bool
	Stdout bool
}

// uiMeta is the JSON metadata emitted to stdout after a successful ui render
// (preserving the cst invariant "every command emits JSON by default").
type uiMeta struct {
	Output       string `json:"output,omitempty"`
	Scope        int64  `json:"scope,omitempty"`
	ActiveScopes int    `json:"active_scopes"`
	OpenTasks    int    `json:"open_tasks"`
	Opened       bool   `json:"opened"`
	Stdout       bool   `json:"stdout,omitempty"`
}

// DoUI renders the current .cst state as a self-contained HTML document.
// Default: writes to .cst/ui.html and opens in the OS default browser,
// emits a JSON metadata object on stdout. --stdout writes HTML to out.
func DoUI(out io.Writer, args UIArgs, asJSON bool) error {
	outputPath := args.Output
	if outputPath == "" && !args.Stdout {
		if _, err := EnsureStoreDir(); err != nil {
			return err
		}
		outputPath = filepath.Join(StoreDir(), "ui.html")
	}
	if outputPath != "" {
		if dir := filepath.Dir(outputPath); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
	}

	meta, err := renderOnce(args.Within)
	if err != nil {
		return err
	}

	if args.Stdout {
		if _, err := io.WriteString(out, meta.html); err != nil {
			return err
		}
		return nil
	}

	if err := os.WriteFile(outputPath, []byte(meta.html), 0o644); err != nil {
		return err
	}
	absPath, err := filepath.Abs(outputPath)
	if err != nil {
		absPath = outputPath
	}
	m := uiMeta{
		Output:       absPath,
		Scope:        args.Within,
		ActiveScopes: meta.activeScopes,
		OpenTasks:    meta.openTasks,
	}
	if !args.NoOpen {
		if err := openInBrowser(absPath); err == nil {
			m.Opened = true
		}
	}
	if asJSON {
		WriteJSON(out, m)
	} else {
		switch {
		case m.Opened:
			fmt.Fprintf(out, "opened: %s\n", absPath)
		default:
			fmt.Fprintf(out, "wrote: %s\n", absPath)
		}
	}

	return nil
}

// renderResult bundles the projection statistics with the rendered HTML so
// callers can report the snapshot without reparsing the document.
type renderResult struct {
	html         string
	activeScopes int
	openTasks    int
}

func renderOnce(within int64) (renderResult, error) {
	events, err := Replay()
	if err != nil {
		return renderResult{}, err
	}
	state, err := Apply(events)
	if err != nil {
		return renderResult{}, err
	}
	if within != 0 {
		if _, ok := state.Nodes[within]; !ok {
			return renderResult{}, herr(ExitNotFound, "node #%d not found", within)
		}
	}
	paths, err := CurrentStorePaths()
	if err != nil {
		return renderResult{}, err
	}
	project := filepath.Base(paths.Root)
	var lastEvent time.Time
	if len(events) > 0 {
		lastEvent = events[len(events)-1].Timestamp
	}
	v := uiViewFrom(state, within, paths.EventsPath, project, len(events), lastEvent)
	return renderResult{
		html:         renderHTML(v),
		activeScopes: len(v.ActivePhases),
		openTasks:    v.Summary.OpenTasks,
	}, nil
}
