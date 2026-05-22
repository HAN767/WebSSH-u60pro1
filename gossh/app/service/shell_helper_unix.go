//go:build !windows
// +build !windows

package service

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

const shellHelperArg = "__gossh_exec_shell"
const sourceProfileArg = "__gossh_source_profile"

func MaybeExecShellHelper() {
	if len(os.Args) < 3 || os.Args[1] != shellHelperArg {
		return
	}

	resetShellSignals()

	shell := os.Args[2]
	args := append([]string{shell}, os.Args[3:]...)
	if len(os.Args) >= 4 && os.Args[3] == sourceProfileArg {
		cmd := fmt.Sprintf("source /etc/profile >/dev/null 2>&1 || . /etc/profile >/dev/null 2>&1; exec %s -i", quoteShellArg(shell))
		args = []string{shell, "-lc", cmd}
	}
	if err := syscall.Exec(shell, args, os.Environ()); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "exec shell failed: %v\n", err)
		os.Exit(127)
	}
}

func quoteShellArg(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func newShellCommand(shell string, args ...string) *exec.Cmd {
	exe, err := os.Executable()
	if err != nil {
		return exec.Command(shell, args...)
	}

	if len(args) == 0 {
		return exec.Command(exe, shellHelperArg, shell, sourceProfileArg)
	}
	helperArgs := append([]string{shellHelperArg, shell}, args...)
	return exec.Command(exe, helperArgs...)
}
