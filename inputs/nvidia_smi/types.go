package nvidia_smi

import (
	"errors"
	"regexp"
)

// qField stands for query field - the field name before the query.
type qField string

// rField stands for returned field - the field name as returned by the nvidia-smi.
type rField string

type MetricInfo struct {
	metricName      string
	valueMultiplier float64
}

var (
	ErrUnexpectedQueryField = errors.New("unexpected query field")
	ErrParseNumber          = errors.New("could not parse number from value")

	numericRegex = regexp.MustCompile("[+-]?([0-9]*[.])?[0-9]+")

	requiredFields = []qField{
		uuidQField,
		nameQField,
		driverModelCurrentQField,
		driverModelPendingQField,
		vBiosVersionQField,
		driverVersionQField,
	}
)
