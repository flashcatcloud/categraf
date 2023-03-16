package procstat

import (
	"syscall"
	"time"
	"unsafe"
)

var (
	user32                       = syscall.NewLazyDLL("user32.dll")
	procEnumWindows              = user32.NewProc("EnumWindows")
	procGetWindowTextW           = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW     = user32.NewProc("GetWindowTextLengthW")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
)

type CallbackInfo struct {
	Pid         uint32
	WindowTitle string
}

func enumWindowsProc(hwnd syscall.Handle, lParam uintptr) uintptr {
	cbInfo := (*CallbackInfo)(unsafe.Pointer(lParam))
	pid := getWindowProcessId(hwnd)
	if pid == cbInfo.Pid {
		cbInfo.WindowTitle = getWindowText(hwnd)
		return 0
	}
	return 1
}

func getWindowText(hwnd syscall.Handle) string {
	textLen := getWindowTextLength(hwnd) + 1
	buf := make([]uint16, textLen)
	procGetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), uintptr(textLen))
	return syscall.UTF16ToString(buf)
}

func getWindowTextLength(hwnd syscall.Handle) int {
	ret, _, _ := procGetWindowTextLengthW.Call(uintptr(hwnd))
	return int(ret)
}

func getWindowProcessId(hwnd syscall.Handle) uint32 {
	var processId uint32
	procGetWindowThreadProcessId.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&processId)))
	return processId
}

func getWindowTitleByPid(pid uint32) string {
	cbInfo := CallbackInfo{Pid: pid}
	done := make(chan struct{})
	go func() {
		procEnumWindows.Call(syscall.NewCallback(enumWindowsProc), uintptr(unsafe.Pointer(&cbInfo)))
		done <- struct{}{}
	}()
	select {
	case <-time.After(2 * time.Second):
		return ""
	case <-done:
		return cbInfo.WindowTitle
	}
	return ""
}
