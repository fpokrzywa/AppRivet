//go:build darwin || linux

package generation

import (
	"os"
	"syscall"
)

func lockDraft(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}
