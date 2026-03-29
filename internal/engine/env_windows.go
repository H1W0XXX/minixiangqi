//go:build windows

package engine

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	modkernel32 = syscall.NewLazyDLL("kernel32.dll")
	procSetEnv  = modkernel32.NewProc("SetEnvironmentVariableW")
)

func setNativeEnv(key, value string) {
	_ = os.Setenv(key, value)

	k, err := syscall.UTF16PtrFromString(key)
	if err != nil {
		return
	}
	v, err := syscall.UTF16PtrFromString(value)
	if err != nil {
		return
	}
	_, _, _ = procSetEnv.Call(uintptr(unsafe.Pointer(k)), uintptr(unsafe.Pointer(v)))
}
