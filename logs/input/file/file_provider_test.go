package file

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	logsconfig "flashcat.cloud/categraf/config/logs"
)

func TestDoublestarWalk(t *testing.T) {
	// Create a temporary directory for tests
	tmpDir, err := os.MkdirTemp("", "categraf-doublestar-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test files
	filesToCreate := []string{
		"a.log",
		"b.txt",
		"sub/c.log",
		"sub/sub2/d.log",
		"sub/sub2/sub3/e.log",
		"sub/sub2/sub3/sub4/f.log",
	}

	for _, f := range filesToCreate {
		path := filepath.Join(tmpDir, filepath.FromSlash(f))
		err := os.MkdirAll(filepath.Dir(path), 0755)
		if err != nil {
			t.Fatalf("Failed to create dirs for %s: %v", path, err)
		}
		err = os.WriteFile(path, []byte("test"), 0644)
		if err != nil {
			t.Fatalf("Failed to write file %s: %v", path, err)
		}
	}

	provider := NewProvider(100, 10000, 2) // set maxDepthLimit to 2

	pattern := filepath.Join(tmpDir, "**", "*.log")
	paths, err := provider.doublestarWalk(pattern)
	if err != nil {
		t.Fatalf("doublestarWalk() unexpected error: %v", err)
	}
	// depth 0: a.log, depth 1: c.log (sub/), depth 2: d.log (sub/sub2/) — all within limit
	if len(paths) != 3 {
		t.Errorf("doublestarWalk() returned %d paths, want 3", len(paths))
	}
}

func TestDoublestarWalk_TraverseLimit(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "categraf-traverse-limit-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	for _, f := range []string{"a.log", "sub/b.log", "sub/sub2/c.log"} {
		path := filepath.Join(tmpDir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("Failed to create dirs: %v", err)
		}
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to write file: %v", err)
		}
	}

	provider := NewProvider(100, 2, 8) // traverse limit = 2 (very small)
	pattern := filepath.Join(tmpDir, "**", "*.log")

	paths, err := provider.doublestarWalk(pattern)
	if err != nil {
		t.Fatalf("doublestarWalk() should not return error on traverse limit, got: %v", err)
	}
	// With limit=2, WalkDir visits: base dir (count=1), then "a.log" lexically first (count=2, matched).
	// Next entry triggers abort (count=3 > 2). Exactly 1 file should be returned.
	if len(paths) != 1 {
		t.Errorf("expected exactly 1 partial result due to traverse limit, got %d", len(paths))
	}
}

func TestSearchFiles_ShortCircuitAndExclude(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "categraf-search-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	filesToCreate := []string{
		"a.log",
		"b.log",
		"c.txt",
		"sub/d.log",
	}

	for _, f := range filesToCreate {
		path := filepath.Join(tmpDir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("Failed to create dirs for %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to write file %s: %v", path, err)
		}
	}

	provider := NewProvider(100, 10000, 8)

	// Test 1: Non-recursive pattern (no **)
	sourceNonRec := logsconfig.NewLogSource("", &logsconfig.LogsConfig{
		Path: filepath.Join(tmpDir, "*.log"),
	})
	files, err := provider.searchFiles(sourceNonRec.Config.Path, sourceNonRec)
	if err != nil {
		t.Fatalf("searchFiles failed: %v", err)
	}
	if len(files) != 2 { // a.log, b.log
		t.Errorf("expected 2 files, got %d", len(files))
	}

	// Test 2: Recursive pattern with Exclude
	sourceRecExclude := logsconfig.NewLogSource("", &logsconfig.LogsConfig{
		Path: filepath.Join(tmpDir, "**", "*.log"),
		ExcludePaths: []string{
			filepath.ToSlash(filepath.Join(tmpDir, "b.log")),
			filepath.ToSlash(filepath.Join(tmpDir, "sub", "*.log")),
		},
	})
	files2, err := provider.searchFiles(sourceRecExclude.Config.Path, sourceRecExclude)
	if err != nil {
		t.Fatalf("searchFiles failed: %v", err)
	}
	if len(files2) != 1 { // only a.log should remain
		t.Errorf("expected 1 file, got %d", len(files2))
	}
}

func TestDoublestarWalk_PermissionDenied(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping permission test on Windows")
	}

	tmpDir, err := os.MkdirTemp("", "categraf-perm-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	noPermDir := filepath.Join(tmpDir, "noperm")
	err = os.MkdirAll(noPermDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create dirs for %s: %v", noPermDir, err)
	}
	if err := os.WriteFile(filepath.Join(noPermDir, "hidden.log"), []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to write hidden.log: %v", err)
	}

	allowedDir := filepath.Join(tmpDir, "allowed")
	if err := os.MkdirAll(allowedDir, 0755); err != nil {
		t.Fatalf("Failed to create dirs for %s: %v", allowedDir, err)
	}
	if err := os.WriteFile(filepath.Join(allowedDir, "visible.log"), []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to write visible.log: %v", err)
	}

	// Remove permissions from noPermDir
	if err := os.Chmod(noPermDir, 0000); err != nil {
		t.Fatalf("Failed to chmod %s: %v", noPermDir, err)
	}
	defer os.Chmod(noPermDir, 0755) // restore for cleanup

	// Probe if chmod worked (e.g. root user might still be able to read)
	if _, err := os.ReadDir(noPermDir); err == nil {
		t.Skip("Skipping permission test because os.Chmod(0000) did not restrict read access (running as root?)")
	}

	provider := NewProvider(100, 10000, 8)
	pattern := filepath.Join(tmpDir, "**", "*.log")

	paths, err := provider.doublestarWalk(pattern)
	if err != nil {
		t.Fatalf("doublestarWalk returned error instead of ignoring permission: %v", err)
	}

	if len(paths) != 1 {
		t.Errorf("expected 1 file, got %d", len(paths))
	}
}
