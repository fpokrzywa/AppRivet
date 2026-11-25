package generation

import (
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

var lockDraftFile = syscall.NewLazyDLL("kernel32.dll").NewProc("LockFileEx")

func lockDraft(file *os.File) error {
	var offset syscall.Overlapped
	result, _, err := lockDraftFile.Call(file.Fd(), 3, 0, 1, 0, uintptr(unsafe.Pointer(&offset)))
	runtime.KeepAlive(file)
	if result == 0 {
		return err
	}
	return nil
}
