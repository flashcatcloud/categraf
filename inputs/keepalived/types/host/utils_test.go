package host

import (
	"bytes"
	"testing"
)

func TestParseSigNum(t *testing.T) {
	t.Parallel()

	signum := bytes.NewBufferString("10\n")
	sigNumInt := parseSigNum(*signum, "DATA")

	if sigNumInt != 10 {
		t.Fail()
	}
}
