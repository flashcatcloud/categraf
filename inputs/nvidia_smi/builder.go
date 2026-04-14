package nvidia_smi

import (
	"fmt"
	"strings"

	"flashcat.cloud/categraf/pkg/stringx"
	"k8s.io/klog/v2"
)

func buildQFieldToMetricInfoMap(qFieldtoRFieldMap map[qField]rField) map[qField]MetricInfo {
	result := make(map[qField]MetricInfo)
	for qField, rField := range qFieldtoRFieldMap {
		result[qField] = buildMetricInfo(rField)
	}

	return result
}

func buildMetricInfo(rField rField) MetricInfo {
	rFieldStr := string(rField)
	suffixTransformed := rFieldStr
	multiplier := 1.0
	split := strings.Split(rFieldStr, " ")[0]

	//nolint:gocritic
	if strings.HasSuffix(rFieldStr, " [W]") {
		suffixTransformed = split + "_watts"
	} else if strings.HasSuffix(rFieldStr, " [MHz]") {
		suffixTransformed = split + "_clock_hz"
		multiplier = 1000000
	} else if strings.HasSuffix(rFieldStr, " [MiB]") {
		suffixTransformed = split + "_bytes"
		multiplier = 1048576
	} else if strings.HasSuffix(rFieldStr, " [%]") {
		suffixTransformed = split + "_ratio"
		multiplier = 0.01
	}

	metricName := stringx.SnakeCase(strings.ReplaceAll(suffixTransformed, ".", "_"))

	return MetricInfo{
		metricName:      metricName,
		valueMultiplier: multiplier,
	}
}

func (s *GPUStats) buildQFieldToRFieldMap() ([]qField, map[qField]rField, error) {
	qFieldsSeparated := strings.Split(s.QueryFieldNames, ",")

	qFields := toQFieldSlice(qFieldsSeparated)
	qFields = append(qFields, requiredFields...)
	qFields = removeDuplicateQFields(qFields)

	if len(qFieldsSeparated) == 1 && qFieldsSeparated[0] == qFieldsAuto {
		parsed, err := parseAutoQFields(s.NvidiaSmiCommand)
		if err != nil {
			klog.Warningf("failed to auto-determine nvidia-smi query field names, falling back to built-in list: err=%v", err)
			return getKeys(fallbackQFieldToRFieldMap), fallbackQFieldToRFieldMap, nil
		}

		qFields = parsed
	}

	resultTable, err := s.scrape(qFields)

	var rFields []rField

	if err != nil {
		klog.Warningf("failed to run initial nvidia-smi scrape, using built-in field mapping: err=%v", err)

		rFields, err = getFallbackValues(qFields)
		if err != nil {
			return nil, nil, err
		}
	} else {
		rFields = resultTable.rFields
	}

	r := make(map[qField]rField, len(qFields))
	for i, q := range qFields {
		r[q] = rFields[i]
	}

	return qFields, r, nil
}

func removeDuplicateQFields(qFields []qField) []qField {
	qFieldMap := make(map[qField]struct{})

	var uniqueQFields []qField

	for _, field := range qFields {
		_, exists := qFieldMap[field]
		if !exists {
			uniqueQFields = append(uniqueQFields, field)
			qFieldMap[field] = struct{}{}
		}
	}

	return uniqueQFields
}

func getKeys(m map[qField]rField) []qField {
	qFields := make([]qField, len(m))

	i := 0

	for key := range m {
		qFields[i] = key
		i++
	}

	return qFields
}

func getFallbackValues(qFields []qField) ([]rField, error) {
	rFields := make([]rField, len(qFields))

	counter := 0

	for _, q := range qFields {
		val, contains := fallbackQFieldToRFieldMap[q]
		if !contains {
			return nil, fmt.Errorf("%w: %s", ErrUnexpectedQueryField, q)
		}

		rFields[counter] = val
		counter++
	}

	return rFields, nil
}
