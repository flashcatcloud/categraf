// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

package memory

import (
	"fmt"
	"log"
	"strconv"
	"strings"
)

type Memory struct{}

const name = "memory"

func (self *Memory) Name() string {
	return name
}

func (self *Memory) Collect() (result interface{}, err error) {
	mem, err := getMemoryInfo()
	if total, ok := mem["total"]; ok {
		var (
			newTotal = total
			times    = 1
		)
		t := strings.ToLower(total)
		switch {
		case strings.HasSuffix(t, "k"):
			newTotal = strings.TrimSuffix(t, "k")
			times = 1024

		case strings.HasSuffix(t, "kb"):
			newTotal = strings.TrimSuffix(t, "kb")
			times = 1024

		case strings.HasSuffix(t, "m"):
			newTotal = strings.TrimSuffix(t, "m")
			times = 1024 * 1024
		case strings.HasSuffix(t, "mb"):
			newTotal = strings.TrimSuffix(t, "mb")
			times = 1024 * 1024

		case strings.HasSuffix(t, "g"):
			newTotal = strings.TrimSuffix(t, "g")
			times = 1024 * 1024 * 1024
		case strings.HasSuffix(t, "gb"):
			newTotal = strings.TrimSuffix(t, "gb")
			times = 1024 * 1024 * 1024

		}
		tv, e := convert(newTotal, times)
		if e != nil {
			log.Printf("W! parse memory total [%s||%s||%s] error: %s", total, t, newTotal, e)
			err = e
		} else {
			mem["total"] = fmt.Sprintf("%d", int64(tv))
		}
	}

	if swap, ok := mem["swap_total"]; ok {
		var (
			newSwap = swap
			times   = 1
		)
		s := strings.ToLower(swap)
		switch {
		case strings.HasSuffix(s, "k"):
			newSwap = strings.TrimSuffix(s, "k")
			times = 1024

		case strings.HasSuffix(s, "kb"):
			newSwap = strings.TrimSuffix(s, "kb")
			times = 1024

		case strings.HasSuffix(s, "m"):
			newSwap = strings.TrimSuffix(s, "m")
			times = 1024 * 1024
		case strings.HasSuffix(s, "mb"):
			newSwap = strings.TrimSuffix(s, "mb")
			times = 1024 * 1024

		case strings.HasSuffix(s, "g"):
			newSwap = strings.TrimSuffix(s, "g")
			times = 1024 * 1024 * 1024
		case strings.HasSuffix(s, "gb"):
			newSwap = strings.TrimSuffix(s, "gb")
			times = 1024 * 1024 * 1024

		}
		tv, e := convert(newSwap, times)
		if e != nil {
			log.Printf("W! parse memory swap [%s||%s||%s] error: %s", swap, s, newSwap, err)
			err = e
		} else {
			mem["swap_total"] = fmt.Sprintf("%d", int64(tv))
		}
	}

	return mem, err
}

func convert(total string, times int) (float64, error) {
	t, err := strconv.ParseFloat(total, 64)
	if err != nil {
		return 0, err
	}
	return t * float64(times), nil
}
