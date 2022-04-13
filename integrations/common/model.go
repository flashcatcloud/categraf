package common

import "time"

type IntervalDuration struct {
	IntervalStr string        // e.g. 1s 2s
	IntervalDur time.Duration // e.g. 1s 2s
}

type TimeoutDuration struct {
	TimeoutStr string        // e.g. 1s 2s
	TimeoutDur time.Duration // e.g. 1s 2s
}

func (d *IntervalDuration) Tidy() error {
	if d.IntervalStr != "" {
		val, err := time.ParseDuration(d.IntervalStr)
		if err != nil {
			return err
		}
		d.IntervalDur = val
	}

	return nil
}

func (d *TimeoutDuration) Tidy() error {
	if d.TimeoutStr != "" {
		val, err := time.ParseDuration(d.TimeoutStr)
		if err != nil {
			return err
		}
		d.TimeoutDur = val
	}

	return nil
}
