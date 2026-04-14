package logging

import (
	"bytes"
	"flag"
	"log"
	"strings"
	"testing"

	"k8s.io/klog/v2"
)

func TestConfigureMapsDebugToVerbosity(t *testing.T) {
	state := klog.CaptureState()
	defer state.Restore()
	oldOutput := log.Writer()
	oldFlags := log.Flags()
	oldPrefix := log.Prefix()
	defer log.SetOutput(oldOutput)
	defer log.SetFlags(oldFlags)
	defer log.SetPrefix(oldPrefix)

	fs := flag.NewFlagSet("logging", flag.ContinueOnError)
	RegisterFlags(fs)
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	var buf bytes.Buffer
	if err := configureWithWriter(&buf, fs, true, 0); err != nil {
		t.Fatalf("configureWithWriter: %v", err)
	}

	klog.V(1).InfoS("debug enabled")
	klog.Flush()

	if !strings.Contains(buf.String(), "debug enabled") {
		t.Fatalf("expected buffer to contain debug message, got %q", buf.String())
	}
}

func TestConfigureBridgesStandardLibraryLog(t *testing.T) {
	state := klog.CaptureState()
	defer state.Restore()
	oldOutput := log.Writer()
	oldFlags := log.Flags()
	oldPrefix := log.Prefix()
	defer log.SetOutput(oldOutput)
	defer log.SetFlags(oldFlags)
	defer log.SetPrefix(oldPrefix)

	fs := flag.NewFlagSet("logging", flag.ContinueOnError)
	RegisterFlags(fs)
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	var buf bytes.Buffer
	if err := configureWithWriter(&buf, fs, false, 0); err != nil {
		t.Fatalf("configureWithWriter: %v", err)
	}

	logger := log.New(log.Writer(), "", log.Flags())
	logger.Println("legacy bridge message")
	klog.Flush()

	if !strings.Contains(buf.String(), "legacy bridge message") {
		t.Fatalf("expected buffer to contain bridged message, got %q", buf.String())
	}
}

func TestConfigureUsesConfiguredVerbosity(t *testing.T) {
	state := klog.CaptureState()
	defer state.Restore()

	fs := flag.NewFlagSet("logging", flag.ContinueOnError)
	RegisterFlags(fs)
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	var buf bytes.Buffer
	if err := configureWithWriter(&buf, fs, false, 2); err != nil {
		t.Fatalf("configureWithWriter: %v", err)
	}

	klog.V(2).InfoS("verbosity two enabled")
	klog.Flush()

	if !strings.Contains(buf.String(), "verbosity two enabled") {
		t.Fatalf("expected buffer to contain v=2 message, got %q", buf.String())
	}
}

func TestConfigureDoesNotExceedConfiguredVerbosity(t *testing.T) {
	state := klog.CaptureState()
	defer state.Restore()

	fs := flag.NewFlagSet("logging", flag.ContinueOnError)
	RegisterFlags(fs)
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	var buf bytes.Buffer
	if err := configureWithWriter(&buf, fs, false, 1); err != nil {
		t.Fatalf("configureWithWriter: %v", err)
	}

	klog.V(2).InfoS("verbosity two should stay hidden")
	klog.Flush()

	if strings.Contains(buf.String(), "verbosity two should stay hidden") {
		t.Fatalf("expected buffer to exclude v=2 message, got %q", buf.String())
	}
}
