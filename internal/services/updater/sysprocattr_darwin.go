//go:build darwin

package updater

import "syscall"

// detachedSysProcAttr starts the relaunched agent in its own session so it
// survives the parent process exiting (same intent as the Linux/Windows
// variants).
func detachedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setsid: true,
	}
}
