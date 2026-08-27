// Copyright 2020 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// Modified for Categraf's native input and sample model.

package json_exporter

import (
	"errors"
	"fmt"
	"math"
	"strconv"
)

func sanitizeValue(value string) (float64, error) {
	if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
		return floatValue, nil
	}
	if boolValue, err := strconv.ParseBool(value); err == nil {
		if boolValue {
			return 1, nil
		}
		return 0, nil
	}
	if value == "<nil>" {
		return math.NaN(), nil
	}
	return 0, errors.New("value is neither a number nor a boolean")
}

func sanitizeIntValue(value string) (int64, error) {
	intValue, err := strconv.ParseInt(value, 10, 64)
	if err == nil {
		return intValue, nil
	}

	floatValue, floatErr := strconv.ParseFloat(value, 64)
	if floatErr != nil || math.IsNaN(floatValue) || math.IsInf(floatValue, 0) || math.Trunc(floatValue) != floatValue || floatValue < math.MinInt64 || floatValue > math.MaxInt64 {
		return 0, fmt.Errorf("parse integer: %w", err)
	}
	return int64(floatValue), nil
}
