//go:build linux

package updater

import "syscall"

// detachedSysProcAttr starts the relaunched agent in its own session so it
// survives the parent process exiting (equivalent intent to the Windows
// DETACHED_PROCESS flag).
func detachedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setsid: true,
	}
}
