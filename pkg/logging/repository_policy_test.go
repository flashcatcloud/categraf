package logging

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

var forbiddenStdLog = regexp.MustCompile(`\blog\.(Print|Println|Printf|Panic|Panicf|Panicln|Fatal|Fatalf|Fatalln)\b`)
var forbiddenDebugBranch = regexp.MustCompile(`if\s+(config\.Config\.DebugMode|Config\.DebugMode)\s*\{`)

func TestCoreRuntimeDoesNotUseStandardLogOrDebugBranches(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	files := []string{
		"main.go",
		"main_posix.go",
		"main_windows.go",
		"agent/agent.go",
		"agent/metrics_agent.go",
		"agent/metrics_reader.go",
		"writer/writer.go",
		"writer/writers.go",
		"heartbeat/heartbeat.go",
	}

	for _, rel := range files {
		path := filepath.Join(repoRoot, rel)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if forbiddenStdLog.Match(content) {
			t.Fatalf("%s still uses forbidden standard log calls", path)
		}
		if forbiddenDebugBranch.Match(content) {
			t.Fatalf("%s still contains forbidden debug branch", path)
		}
	}
}
