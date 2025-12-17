package ntp

import (
	"log"
	"testing"
	"time"

	"github.com/beevik/ntp"
)

func TestClockOffset(t *testing.T) {
	log.Println("Begin")
	resp, err := ntp.QueryWithOptions("ntp1.aliyun.com", ntp.QueryOptions{
		Timeout: 20 * time.Second,
		Version: 4,
	})
	if err != nil {
		log.Println(err)
		return
	}

	// offset in ms
	delta := resp.ClockOffset.Seconds() * 1000
	log.Println("Offset (ms):", delta)
}
