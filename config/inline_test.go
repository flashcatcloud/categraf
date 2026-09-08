package config

import (
	"testing"
	"testing/synctest"
	"time"

	"flashcat.cloud/categraf/types"
)

func TestProcessSampleTimestamps(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		previous := Config
		Config = &ConfigType{Global: Global{OmitHostname: true}}
		defer func() { Config = previous }()

		scheduled := time.Now()
		explicit := scheduled.Add(-time.Hour)
		slist := types.NewSampleList()
		slist.Timestamp = scheduled
		slist.PushSample("test", "default", 1)
		slist.PushFront(types.NewSample("test", "explicit", 2).SetTime(explicit))
		time.Sleep(250 * time.Millisecond)

		ic := &InternalConfig{}
		samples := ic.Process(slist).PopBackAll()
		if len(samples) != 2 {
			t.Fatalf("got %d samples, want 2", len(samples))
		}
		if !samples[0].Timestamp.Equal(scheduled) {
			t.Errorf("default timestamp = %s, want scheduled time %s", samples[0].Timestamp, scheduled)
		}
		if !samples[1].Timestamp.Equal(explicit) {
			t.Errorf("explicit timestamp = %s, want %s", samples[1].Timestamp, explicit)
		}

		// Inputs outside the periodic reader still get the current time.
		unscheduled := types.NewSampleList()
		unscheduled.PushSample("test", "unscheduled", 3)
		samples = ic.Process(unscheduled).PopBackAll()
		if !samples[0].Timestamp.Equal(time.Now()) {
			t.Errorf("unscheduled timestamp = %s, want %s", samples[0].Timestamp, time.Now())
		}
	})
}
