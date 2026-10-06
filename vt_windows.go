package main

import (
	"syscall"
	"unsafe"
)

func enableVirtualTerminal() bool {
	handle, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil {
		return false
	}
	kernel := syscall.NewLazyDLL("kernel32.dll")
	getMode := kernel.NewProc("GetConsoleMode")
	setMode := kernel.NewProc("SetConsoleMode")
	var mode uint32
	ret, _, _ := getMode.Call(uintptr(handle), uintptr(unsafe.Pointer(&mode)))
	if ret == 0 {
		return false
	}
	ret, _, _ = setMode.Call(uintptr(handle), uintptr(mode|0x0004))
	return ret != 0
}
