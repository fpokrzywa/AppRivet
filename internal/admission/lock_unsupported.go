//go:build !linux && !darwin && !windows

package admission

import "os"

func tryLockFile(*os.File) (bool, error) {
	return false, failure("unsupported_platform", "reservation locking requires Linux, macOS or Windows")
}
func unlockFile(*os.File) {}
