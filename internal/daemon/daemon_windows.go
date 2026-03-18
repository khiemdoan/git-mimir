//go:build windows

package daemon

import "syscall"

// sysProcDetach returns platform-specific process attributes to detach
// from the parent process group on Windows.
func sysProcDetach() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
		HideWindow:    true,
	}
}
