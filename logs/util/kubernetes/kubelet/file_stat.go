//go:build !no_logs

package kubelet

import "os"

// FileExists returns true if a file exists and is accessible, false otherwise
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
