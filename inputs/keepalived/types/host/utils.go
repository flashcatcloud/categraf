package host

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
)

func parseSigNum(sigNum bytes.Buffer, sigString string) int64 {
	var signum int64
	if err := json.Unmarshal(sigNum.Bytes(), &signum); err != nil {
		slog.Error("Error parsing signum result",
			"signal", sigString,
			"signum", sigNum.String(),
			"error", err,
		)
		os.Exit(1)
	}

	return signum
}
