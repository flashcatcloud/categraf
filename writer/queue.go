package writer

import (
	"time"

	"github.com/prometheus/prometheus/prompb"
)

type MetricQueue struct {
	queue         chan *prompb.TimeSeries
	batch         int
	flushCallBack func([]prompb.TimeSeries)
}

func newMetricQueue(batch int, flushCallback func([]prompb.TimeSeries)) MetricQueue {
	if batch <= 0 {
		batch = 2000
	}
	return MetricQueue{
		queue:         make(chan *prompb.TimeSeries, batch),
		batch:         batch,
		flushCallBack: flushCallback,
	}
}

func (mq *MetricQueue) Push(s *prompb.TimeSeries) {
	if s == nil {
		return
	}
	mq.queue <- s
}

func (mq *MetricQueue) LoopRead() {
	series := make([]prompb.TimeSeries, 0, mq.batch)
	var count int
	for {
		select {
		case item, open := <-mq.queue:
			if !open {
				// queue closed, post remaining series
				mq.flushCallBack(series)
				return
			}

			if item == nil {
				continue
			}

			series = append(series, *item)
			count++
			if count >= mq.batch {
				mq.flushCallBack(series)
				count = 0
				// reset series slice, do not release memory
				series = series[:0]
			}
		default:
			if len(series) > 0 {
				mq.flushCallBack(series)
				count = 0
				// reset series slice, do not release memory
				series = series[:0]
			}
			time.Sleep(time.Millisecond * 100)
		}
	}
}
