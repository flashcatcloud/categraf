package influx

import (
	"log"
	"strings"
	"time"

	"flashcat.cloud/categraf/types"
	"flashcat.cloud/categraf/types/metric"
	"github.com/influxdata/line-protocol/v2/lineprotocol"
)

// Parser is an InfluxDB Line Protocol parser that implements the
// parsers.Parser interface.
type Parser struct {
	defaultTime TimeFunc
	precision   lineprotocol.Precision
}

type TimeFunc func() time.Time

// NewParser returns a Parser that accepts a measurement and tagset
func NewParser() *Parser {
	return &Parser{
		defaultTime: time.Now,
		precision:   lineprotocol.Nanosecond,
	}
}

func (p *Parser) Parse(input []byte, slist *types.SampleList) error {
	metrics := make([]types.Metric, 0)
	decoder := lineprotocol.NewDecoderWithBytes(input)

	for decoder.Next() {
		m, err := nextMetric(decoder, p.precision, p.defaultTime)
		if err != nil {
			log.Println("E! failed to parse influx line:", string(input), err)
			continue
		}
		metrics = append(metrics, m)
	}

	for _, m := range metrics {
		name := m.Name()
		tags := m.Tags()
		fields := m.Fields()
		for k, v := range fields {
			slist.PushSample(name, k, v, tags)
		}
	}

	return nil
}

func nextMetric(decoder *lineprotocol.Decoder, precision lineprotocol.Precision, defaultTime TimeFunc) (types.Metric, error) {
	measurement, err := decoder.Measurement()
	if err != nil {
		return nil, err
	}
	m := metric.New(string(measurement), nil, nil, time.Time{})

	for {
		key, value, err := decoder.NextTag()
		if err != nil {
			// Allow empty tags for series parser
			if strings.Contains(err.Error(), "empty tag name") {
				break
			}

			return nil, err
		} else if key == nil {
			break
		}

		m.AddTag(string(key), string(value))
	}

	for {
		key, value, err := decoder.NextField()
		if err != nil {
			// Allow empty fields for series parser
			if strings.Contains(err.Error(), "expected field key") {
				break
			}

			return nil, err
		} else if key == nil {
			break
		}

		m.AddField(string(key), value.Interface())
	}

	t, err := decoder.Time(precision, defaultTime())
	if err != nil {
		return nil, err
	}
	m.SetTime(t)

	return m, nil
}
