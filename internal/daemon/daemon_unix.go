//go:build unix

package daemon

import "syscall"

// sysProcDetach returns platform-specific process attributes to detach
// from the parent process group and terminal.
func sysProcDetach() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setsid: true, // Create new session, detached from terminal
	}
}
