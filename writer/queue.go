package writer

import (
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/types"
)

var queue chan *types.Sample

func PushQueue(s *types.Sample) {
	queue <- s
}

func initQueue() {
	queue = make(chan *types.Sample, config.Config.WriterOpt.ChanSize)
	go readQueue(queue)
}

func readQueue(queue chan *types.Sample) {
	batch := config.Config.WriterOpt.Batch
	if batch <= 0 {
		batch = 2000
	}

	series := make([]*types.Sample, 0, batch)

	var count int

	for {
		select {
		case item, open := <-queue:
			if !open {
				// queue closed
				return
			}

			if item == nil {
				continue
			}

			series = append(series, item)
			count++
			if count >= batch {
				postSeries(series)
				count = 0
				// reset series slice, do not release memory
				series = series[:0]
			}
		default:
			if len(series) > 0 {
				postSeries(series)
				count = 0
				// reset series slice, do not release memory
				series = series[:0]
			}
			time.Sleep(time.Millisecond * 100)
		}
	}
}
