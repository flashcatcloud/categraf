// Copyright 2011 Google Inc. All Rights Reserved.
// This file is available under the Apache license.

package tailer

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"flashcat.cloud/categraf/inputs/mtail/internal/logline"
	"flashcat.cloud/categraf/inputs/mtail/internal/testutil"
	"flashcat.cloud/categraf/inputs/mtail/internal/waker"
)

func makeTestTail(t *testing.T, options ...Option) (*Tailer, chan *logline.LogLine, func(int), string, func()) {
	t.Helper()
	tmpDir := testutil.TestTempDir(t)

	ctx, cancel := context.WithCancel(context.Background())
	lines := make(chan *logline.LogLine, 5) // 5 loglines ought to be enough for any test
	var wg sync.WaitGroup
	waker, awaken := waker.NewTest(ctx, 1)
	options = append(options, LogPatterns([]string{tmpDir}), LogstreamPollWaker(waker))
	ta, err := New(ctx, &wg, lines, options...)
	testutil.FatalIfErr(t, err)
	return ta, lines, awaken, tmpDir, func() { cancel(); wg.Wait() }
}

func TestTail(t *testing.T) {
	ta, _, _, dir, stop := makeTestTail(t)

	logfile := filepath.Join(dir, "log")
	f := testutil.TestOpenFile(t, logfile)
	defer f.Close()

	err := ta.TailPath(logfile)
	testutil.FatalIfErr(t, err)

	if _, ok := ta.logstreams[logfile]; !ok {
		t.Errorf("path not found in files map: %+#v", ta.logstreams)
	}

	stop()
}

func TestHandleLogUpdate(t *testing.T) {
	ta, lines, awaken, dir, stop := makeTestTail(t)

	logfile := filepath.Join(dir, "log")
	f := testutil.TestOpenFile(t, logfile)
	defer f.Close()

	testutil.FatalIfErr(t, ta.TailPath(logfile))
	awaken(1)

	testutil.WriteString(t, f, "a\nb\nc\nd\n")
	awaken(1)

	stop()

	received := testutil.LinesReceived(lines)
	expected := []*logline.LogLine{
		{Context: context.Background(), Filename: logfile, Line: "a"},
		{Context: context.Background(), Filename: logfile, Line: "b"},
		{Context: context.Background(), Filename: logfile, Line: "c"},
		{Context: context.Background(), Filename: logfile, Line: "d"},
	}
	testutil.ExpectNoDiff(t, expected, received, testutil.IgnoreFields(logline.LogLine{}, "Context"))
}

// TestHandleLogTruncate writes to a file, waits for those
// writes to be seen, then truncates the file and writes some more.
// At the end all lines written must be reported by the tailer.
func TestHandleLogTruncate(t *testing.T) {
	ta, lines, awaken, dir, stop := makeTestTail(t)

	logfile := filepath.Join(dir, "log")
	f := testutil.OpenLogFile(t, logfile)
	defer f.Close()

	testutil.FatalIfErr(t, ta.TailPath(logfile))
	// Expect to wake 1 wakee, the logstream reading `logfile`.
	awaken(1)

	testutil.WriteString(t, f, "a\nb\nc\n")
	awaken(1)

	if err := f.Truncate(0); err != nil {
		t.Fatal(err)
	}

	// "File.Truncate" does not change the file offset, force a seek to start.
	_, err := f.Seek(0, 0)
	testutil.FatalIfErr(t, err)
	awaken(1)

	testutil.WriteString(t, f, "d\ne\n")
	awaken(1)

	stop()

	received := testutil.LinesReceived(lines)
	expected := []*logline.LogLine{
		{Context: context.Background(), Filename: logfile, Line: "a"},
		{Context: context.Background(), Filename: logfile, Line: "b"},
		{Context: context.Background(), Filename: logfile, Line: "c"},
		{Context: context.Background(), Filename: logfile, Line: "d"},
		{Context: context.Background(), Filename: logfile, Line: "e"},
	}
	testutil.ExpectNoDiff(t, expected, received, testutil.IgnoreFields(logline.LogLine{}, "Context"))
}

func TestHandleLogUpdatePartialLine(t *testing.T) {
	ta, lines, awaken, dir, stop := makeTestTail(t)

	logfile := filepath.Join(dir, "log")
	f := testutil.TestOpenFile(t, logfile)
	defer f.Close()

	testutil.FatalIfErr(t, ta.TailPath(logfile))
	awaken(1) // ensure we've hit an EOF before writing starts

	testutil.WriteString(t, f, "a")
	awaken(1)

	testutil.WriteString(t, f, "b")
	awaken(1)

	testutil.WriteString(t, f, "\n")
	awaken(1)

	stop()

	received := testutil.LinesReceived(lines)
	expected := []*logline.LogLine{
		{Context: context.Background(), Filename: logfile, Line: "ab"},
	}
	testutil.ExpectNoDiff(t, expected, received, testutil.IgnoreFields(logline.LogLine{}, "Context"))
}

func TestTailerUnreadableFile(t *testing.T) {
	// Test broken files are skipped.
	ta, lines, awaken, dir, stop := makeTestTail(t)

	brokenfile := filepath.Join(dir, "brokenlog")
	logfile := filepath.Join(dir, "log")
	testutil.FatalIfErr(t, ta.AddPattern(brokenfile))
	testutil.FatalIfErr(t, ta.AddPattern(logfile))

	log.Println("create logs")
	testutil.FatalIfErr(t, os.Symlink("/nonexistent", brokenfile))
	f := testutil.TestOpenFile(t, logfile)
	defer f.Close()

	testutil.FatalIfErr(t, ta.PollLogPatterns())
	testutil.FatalIfErr(t, ta.PollLogStreamsForCompletion())

	log.Println("write string")
	testutil.WriteString(t, f, "\n")
	awaken(1)

	stop()

	received := testutil.LinesReceived(lines)
	expected := []*logline.LogLine{
		{Context: context.Background(), Filename: logfile, Line: ""},
	}
	testutil.ExpectNoDiff(t, expected, received, testutil.IgnoreFields(logline.LogLine{}, "Context"))
}

func TestTailerInitErrors(t *testing.T) {
	var wg sync.WaitGroup
	_, err := New(context.TODO(), &wg, nil)
	if err == nil {
		t.Error("expected error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	_, err = New(ctx, &wg, nil, nil)
	if err == nil {
		t.Error("expected error")
	}
	lines := make(chan *logline.LogLine, 1)
	_, err = New(ctx, &wg, lines, nil)
	if err == nil {
		t.Error("expected error")
	}
	cancel()
	wg.Wait()
	lines = make(chan *logline.LogLine, 1)
	ctx, cancel = context.WithCancel(context.Background())
	_, err = New(ctx, &wg, lines)
	if err != nil {
		t.Errorf("unexpected error %s", err)
	}
	cancel()
	wg.Wait()
	lines = make(chan *logline.LogLine, 1)
	ctx, cancel = context.WithCancel(context.Background())
	_, err = New(ctx, &wg, lines, OneShot)
	if err != nil {
		t.Errorf("unexpected error %s", err)
	}
	cancel()
	wg.Wait()
}

func TestTailExpireStaleHandles(t *testing.T) {
	t.Skip("need to set lastRead on logstream to inject condition")
	ta, lines, awaken, dir, stop := makeTestTail(t)

	log1 := filepath.Join(dir, "log1")
	f1 := testutil.TestOpenFile(t, log1)
	log2 := filepath.Join(dir, "log2")
	f2 := testutil.TestOpenFile(t, log2)

	if err := ta.TailPath(log1); err != nil {
		t.Fatal(err)
	}
	if err := ta.TailPath(log2); err != nil {
		t.Fatal(err)
	}
	testutil.WriteString(t, f1, "1\n")
	testutil.WriteString(t, f2, "2\n")

	awaken(1)

	stop()

	received := testutil.LinesReceived(lines)
	expected := []*logline.LogLine{
		{Context: context.Background(), Filename: log1, Line: "1"},
		{Context: context.Background(), Filename: log2, Line: "2"},
	}
	testutil.ExpectNoDiff(t, expected, received, testutil.IgnoreFields(logline.LogLine{}, "Context"))

	if err := ta.ExpireStaleLogstreams(); err != nil {
		t.Fatal(err)
	}
	ta.logstreamsMu.RLock()
	if len(ta.logstreams) != 2 {
		t.Errorf("expecting 2 handles, got %v", ta.logstreams)
	}
	ta.logstreamsMu.RUnlock()
	// ta.logstreamsMu.Lock()
	// ta.logstreams[log1].(*File).lastRead = time.Now().Add(-time.Hour*24 + time.Minute)
	// ta.logstreamsMu.Unlock()
	if err := ta.ExpireStaleLogstreams(); err != nil {
		t.Fatal(err)
	}
	ta.logstreamsMu.RLock()
	if len(ta.logstreams) != 2 {
		t.Errorf("expecting 2 handles, got %v", ta.logstreams)
	}
	ta.logstreamsMu.RUnlock()
	// ta.logstreamsMu.Lock()
	// ta.logstreams[log1].(*File).lastRead = time.Now().Add(-time.Hour*24 - time.Minute)
	// ta.logstreamsMu.Unlock()
	if err := ta.ExpireStaleLogstreams(); err != nil {
		t.Fatal(err)
	}
	ta.logstreamsMu.RLock()
	if len(ta.logstreams) != 1 {
		t.Errorf("expecting 1 logstreams, got %v", ta.logstreams)
	}
	ta.logstreamsMu.RUnlock()
	log.Println("good")
}
