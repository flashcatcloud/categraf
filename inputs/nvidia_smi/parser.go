package nvidia_smi

import (
	"fmt"
	"strconv"
	"strings"
)

//nolint:gomnd
func transformRawValue(rawValue string, valueMultiplier float64) (float64, error) {
	trimmed := strings.TrimSpace(rawValue)
	if strings.HasPrefix(trimmed, "0x") {
		return hexToDecimal(trimmed)
	}

	val := strings.ToLower(trimmed)

	switch val {
	case "enabled", "yes", "active":
		return 1, nil
	case "disabled", "no", "not active":
		return 0, nil
	case "default":
		return 0, nil
	case "exclusive_thread":
		return 1, nil
	case "prohibited":
		return 2, nil
	case "exclusive_process":
		return 3, nil
	default:
		return parseSanitizedValueWithBestEffort(val, valueMultiplier)
	}
}

func parseSanitizedValueWithBestEffort(sanitizedValue string, valueMultiplier float64) (float64, error) {
	allNums := numericRegex.FindAllString(sanitizedValue, 2) //nolint:gomnd
	if len(allNums) != 1 {
		return -1, fmt.Errorf("%w: %s", ErrParseNumber, sanitizedValue)
	}

	parsed, err := strconv.ParseFloat(allNums[0], 64)
	if err != nil {
		return -1, fmt.Errorf("failed to parse float: %w", err)
	}

	return parsed * valueMultiplier, nil
}
