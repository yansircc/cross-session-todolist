package cst

import (
	"fmt"
	"os/exec"
	"runtime"
)

// openInBrowser launches the OS default handler for path. It is fire-and-forget:
// the spawned process keeps running after cst exits.
func openInBrowser(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "linux":
		cmd = exec.Command("xdg-open", path)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	default:
		return fmt.Errorf("open: unsupported platform %s", runtime.GOOS)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// Release process so it survives our exit; don't Wait.
	_ = cmd.Process.Release()
	return nil
}
