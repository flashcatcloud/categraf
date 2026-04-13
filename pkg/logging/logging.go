package logging

import (
	"flag"
	"io"
	stdlog "log"
	"os"
	"strconv"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
	"k8s.io/klog/v2"
)

// RegisterFlags registers klog flags on the provided flag set.
func RegisterFlags(fs *flag.FlagSet) {
	klog.InitFlags(fs)
}

// Configure initializes logging with the configured output target and klog flags.
func Configure(output string, maxSize, maxAge, maxBackups int, localTime, compress, debug bool, debugLevel int) error {
	return configureWithWriter(newWriter(output, maxSize, maxAge, maxBackups, localTime, compress), flag.CommandLine, debug, debugLevel)
}

func configureWithWriter(writer io.Writer, fs *flag.FlagSet, debug bool, debugLevel int) error {
	verbosity := debugLevel
	if debug && verbosity == 0 {
		verbosity = 1
	}

	sets := []struct {
		name  string
		value string
	}{
		{name: "logtostderr", value: "false"},
		{name: "alsologtostderr", value: "false"},
		{name: "stderrthreshold", value: "FATAL"},
		{name: "v", value: strconv.Itoa(verbosity)},
	}
	for _, set := range sets {
		if err := fs.Set(set.name, set.value); err != nil {
			return err
		}
	}

	stdlog.SetFlags(0)
	klog.SetOutput(writer)
	klog.CopyStandardLogTo("INFO")
	klog.StartFlushDaemon(5 * time.Second)
	return nil
}

func newWriter(output string, maxSize, maxAge, maxBackups int, localTime, compress bool) io.Writer {
	switch output {
	case "", "stdout":
		return os.Stdout
	case "stderr":
		return os.Stderr
	default:
		return &lumberjack.Logger{
			Filename:   output,
			MaxSize:    maxSize,
			MaxAge:     maxAge,
			MaxBackups: maxBackups,
			LocalTime:  localTime,
			Compress:   compress,
		}
	}
}

// Sync flushes pending log I/O.
func Sync() {
	klog.Flush()
}
