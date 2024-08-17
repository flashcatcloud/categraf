//go:build !no_ibex && windows

package ibex

import (
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

var kernel32 = windows.NewLazySystemDLL("kernel32")
var multiByteToWideChar = kernel32.NewProc("MultiByteToWideChar")
var wideCharToMultiByte = kernel32.NewProc("WideCharToMultiByte")

const CP_ACP = 0        // default to ANSI code page
const CP_OEMCP = 1      // default to OEM  code page
const CP_THREAD_ACP = 3 // current thread's ANSI code page

func CmdStart(cmd *exec.Cmd) error {
	return cmd.Start()
}

func CmdKill(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}

func ansiToUtf8(mbcs []byte) (string, error) {
	if mbcs == nil || len(mbcs) <= 0 {
		return "", nil
	}
	// https://learn.microsoft.com/en-us/windows/win32/api/stringapiset/nf-stringapiset-multibytetowidechar#syntax
	// ansi -> utf16
	size, _, _ := multiByteToWideChar.Call(CP_ACP, 0, uintptr(unsafe.Pointer(&mbcs[0])), uintptr(len(mbcs)), uintptr(0), 0)
	if size <= 0 {
		return "", windows.GetLastError()
	}
	utf16 := make([]uint16, size)
	rc, _, _ := multiByteToWideChar.Call(CP_ACP, 0, uintptr(unsafe.Pointer(&mbcs[0])), uintptr(len(mbcs)), uintptr(unsafe.Pointer(&utf16[0])), size)
	if rc == 0 {
		return "", windows.GetLastError()
	}
	return windows.UTF16ToString(utf16), nil
}

func utf8ToAnsi(utf8 string) (string, error) {
	utf16, err := windows.UTF16FromString(utf8)
	if err != nil {
		return "", err
	}
	// https://learn.microsoft.com/en-us/windows/win32/api/stringapiset/nf-stringapiset-widechartomultibyte
	size, _, _ := wideCharToMultiByte.Call(CP_ACP, 0, uintptr(unsafe.Pointer(&utf16[0])), uintptr(len(utf16)), uintptr(0), 0, uintptr(0), uintptr(0))
	if size <= 0 {
		return "", windows.GetLastError()
	}
	mbcs := make([]byte, size)
	rc, _, _ := wideCharToMultiByte.Call(CP_ACP, 0, uintptr(unsafe.Pointer(&utf16[0])), uintptr(len(utf16)), uintptr(unsafe.Pointer(&mbcs[0])), size, uintptr(0), uintptr(0))
	if rc == 0 {
		return "", windows.GetLastError()
	}
	if mbcs[size-1] == 0 {
		mbcs = mbcs[:size-1]
	}
	return string(mbcs), nil
}
