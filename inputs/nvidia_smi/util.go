package nvidia_smi

import (
	"regexp"
	"strconv"
	"strings"
)

const (
	hexToDecimalBase        = 16
	hexToDecimalUIntBitSize = 64
)

var (
	matchAllCap = regexp.MustCompile("([a-z0-9])([A-Z])")
)

func hexToDecimal(hex string) (float64, error) {
	s := hex
	s = strings.ReplaceAll(s, "0x", "")
	s = strings.ReplaceAll(s, "0X", "")
	parsed, err := strconv.ParseUint(s, hexToDecimalBase, hexToDecimalUIntBitSize)

	return float64(parsed), err
}
