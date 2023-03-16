//go:build !windows
// +build !windows

package procstat

func getWindowTitleByPid(pid uint32) string {
	return ""
}
