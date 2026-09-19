//go:build windows

package updater

import "syscall"

// detachedSysProcAttr launches the relaunched agent detached from the
// current console, with no visible window.
func detachedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: 0x08000000 | 0x00000008, // DETACHED_PROCESS | CREATE_NO_WINDOW
	}
}
