package main

import (
	"syscall"
	"unsafe"
)

func reportError(message string) {
	text, _ := syscall.UTF16PtrFromString(message)
	title, _ := syscall.UTF16PtrFromString("AppRivet")
	box := syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW")
	_, _, _ = box.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10)
}
