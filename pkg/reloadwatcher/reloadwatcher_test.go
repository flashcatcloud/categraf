package reloadwatcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcherDebouncesTargetChangesAndIgnoresOtherFiles(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(target, []byte("one"), 0644); err != nil {
		t.Fatalf("write target: %v", err)
	}

	changes := make(chan struct{}, 10)
	w, err := Start(target, 50*time.Millisecond, func() {
		changes <- struct{}{}
	})
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}
	defer w.Close()

	time.Sleep(50 * time.Millisecond)

	if err := os.WriteFile(filepath.Join(dir, "other.toml"), []byte("ignored"), 0644); err != nil {
		t.Fatalf("write other file: %v", err)
	}
	assertNoChange(t, changes, 120*time.Millisecond)

	if err := os.WriteFile(target, []byte("two"), 0644); err != nil {
		t.Fatalf("write target second version: %v", err)
	}
	if err := os.WriteFile(target, []byte("three"), 0644); err != nil {
		t.Fatalf("write target third version: %v", err)
	}

	assertChange(t, changes, time.Second)
	assertNoChange(t, changes, 150*time.Millisecond)
}

func assertChange(t *testing.T, changes <-chan struct{}, timeout time.Duration) {
	t.Helper()
	select {
	case <-changes:
	case <-time.After(timeout):
		t.Fatal("timed out waiting for change")
	}
}

func assertNoChange(t *testing.T, changes <-chan struct{}, timeout time.Duration) {
	t.Helper()
	select {
	case <-changes:
		t.Fatal("unexpected change")
	case <-time.After(timeout):
	}
}
