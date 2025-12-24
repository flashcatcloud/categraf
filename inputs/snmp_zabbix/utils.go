package snmp_zabbix

import (
	"crypto/rand"
	"math/big"
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
	// Use crypto/rand for thread safety and to avoid global lock contention
	max := big.NewInt(2 * int64(jm))
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		// Fallback or return 0 on error
		return 0
	}

	// 3. Offset to [-magnitude, +magnitude)
	offset := n.Int64() - int64(jm)

	return time.Duration(offset)
}
