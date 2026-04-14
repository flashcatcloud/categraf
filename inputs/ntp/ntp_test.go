package ntp

import (
	stdlog "log"
	"testing"
	"time"

	"github.com/beevik/ntp"
)

func TestClockOffset(t *testing.T) {
	logger := stdlog.New(stdlog.Writer(), "", 0)
	logger.Println("Begin")
	resp, err := ntp.QueryWithOptions("ntp1.aliyun.com", ntp.QueryOptions{
		Timeout: 20 * time.Second,
		Version: 4,
	})
	if err != nil {
		logger.Println(err)
		return
	}

	// offset in ms
	delta := resp.ClockOffset.Seconds() * 1000
	logger.Println("Offset (ms):", delta)
}
