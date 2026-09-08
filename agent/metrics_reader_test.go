package agent

import (
	"testing"
	"testing/synctest"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
)

type timestampTestInput struct {
	config.PluginConfig
	durations []time.Duration
	calls     int
	samples   chan []*types.Sample
}

func (in *timestampTestInput) Name() string        { return "timestamp_test" }
func (in *timestampTestInput) Clone() inputs.Input { return in }

func (in *timestampTestInput) Gather(slist *types.SampleList) {
	duration := in.durations[in.calls%len(in.durations)]
	in.calls++
	time.Sleep(duration)
	slist.PushSample("test", "value", in.calls)
}

func (in *timestampTestInput) Process(slist *types.SampleList) *types.SampleList {
	in.samples <- in.PluginConfig.Process(slist).PopBackAll()
	return nil // Do not send test samples to a remote writer.
}

func TestPeriodicSampleTimestamps(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		previous := config.Config
		config.Config = &config.ConfigType{Global: config.Global{OmitHostname: true}}
		defer func() { config.Config = previous }()

		// Start near a deduplication boundary. Alternating collection durations
		// must not put consecutive samples into the same 30-second bucket.
		time.Sleep(29900 * time.Millisecond)
		in := &timestampTestInput{
			PluginConfig: config.PluginConfig{Interval: config.Duration(30 * time.Second)},
			durations:    []time.Duration{300 * time.Millisecond, 40 * time.Millisecond, 260 * time.Millisecond},
			samples:      make(chan []*types.Sample),
		}
		reader := newInputReader(in.Name(), in)
		done := make(chan struct{})
		go func() {
			reader.startInput()
			close(done)
		}()
		defer func() {
			reader.Stop()
			<-done
		}()

		var previousTimestamp time.Time
		buckets := make(map[int64]bool)
		for i := 0; i < 20; i++ {
			samples := <-in.samples
			if len(samples) != 1 {
				t.Fatalf("got %d samples, want 1", len(samples))
			}
			timestamp := samples[0].Timestamp
			if !previousTimestamp.IsZero() && timestamp.Sub(previousTimestamp) != 30*time.Second {
				t.Errorf("sample %d: timestamp interval = %s, want 30s", i, timestamp.Sub(previousTimestamp))
			}
			buckets[timestamp.UnixMilli()/30000] = true
			previousTimestamp = timestamp
		}
		if len(buckets) != 20 {
			t.Errorf("20 collections occupied %d deduplication buckets, want 20", len(buckets))
		}
	})
}

func TestPeriodicSampleTimestampsAfterSlowCollection(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		previous := config.Config
		config.Config = &config.ConfigType{Global: config.Global{OmitHostname: true}}
		defer func() { config.Config = previous }()

		in := &timestampTestInput{
			PluginConfig: config.PluginConfig{Interval: config.Duration(30 * time.Second)},
			durations:    []time.Duration{75 * time.Second, time.Millisecond, time.Millisecond},
			samples:      make(chan []*types.Sample),
		}
		reader := newInputReader(in.Name(), in)
		done := make(chan struct{})
		go func() {
			reader.startInput()
			close(done)
		}()
		defer func() {
			reader.Stop()
			<-done
		}()

		first := (<-in.samples)[0].Timestamp
		second := (<-in.samples)[0].Timestamp
		third := (<-in.samples)[0].Timestamp
		if second.Sub(first) != 75*time.Second {
			t.Errorf("after slow collection interval = %s, want 75s without backfilling missed ticks", second.Sub(first))
		}
		if third.Sub(second) != 30*time.Second {
			t.Errorf("after resynchronization interval = %s, want 30s", third.Sub(second))
		}
	})
}
