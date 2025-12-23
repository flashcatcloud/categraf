package snmp_zabbix

import (
	"math/rand"
	"time"
)

func jitterMagnitude(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}

	// 1. Calculate magnitude (e.g. 1% of interval)
	jm := d / 100
	if jm <= 0 {
		return 0
	}
	if jm > 30*time.Second {
		jm = 30 * time.Second
	}
	return jm
}

// jitter returns a random duration in [-d/100, +d/100]
func jitter(d time.Duration) time.Duration {
	jm := jitterMagnitude(d)
	if jm == 0 {
		return 0
	}

	// 2. Generate random in [0, 2 * magnitude)
	n := rand.Int63n(2 * int64(jm))

	// 3. Offset to [-magnitude, +magnitude)
	offset := n - int64(jm)

	return time.Duration(offset)
}
