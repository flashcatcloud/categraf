package internal

import (
	"errors"
	"math"
	"sort"

	"google.golang.org/genproto/googleapis/api/distribution"
)

func GenerateHistogramBuckets(dist *distribution.Distribution) (quantileContainer, error) {
	var bucketKeys []float64

	opts := dist.BucketOptions.Options
	switch o := opts.(type) {
	case *distribution.Distribution_BucketOptions_ExponentialBuckets:
		exponential := o.ExponentialBuckets
		// 指数桶
		// https://cloud.google.com/monitoring/api/ref_v3/rest/v3/TypedValue#exponential
		num := int(exponential.GetNumFiniteBuckets())
		bucketKeys = make([]float64, num+2)
		for i := 0; i <= num; i++ {
			bucketKeys[i] = exponential.GetScale() * math.Pow(exponential.GetGrowthFactor(), float64(i))
		}

	case *distribution.Distribution_BucketOptions_LinearBuckets:
		linear := o.LinearBuckets
		// 线性桶
		num := int(linear.GetNumFiniteBuckets())
		bucketKeys = make([]float64, num+2)
		for i := 0; i <= num; i++ {
			bucketKeys[i] = linear.GetOffset() + (float64(i) * linear.GetWidth())
		}

	case *distribution.Distribution_BucketOptions_ExplicitBuckets:
		explicit := o.ExplicitBuckets
		// 自定义桶
		bucketKeys = make([]float64, len(explicit.GetBounds())+1)
		copy(bucketKeys, explicit.GetBounds())

	default:
		return quantileContainer{}, errors.New("Unknown distribution buckets type")
	}

	// 最后一个桶为无穷大
	bucketKeys[len(bucketKeys)-1] = math.Inf(0)

	bs := make(buckets, 0, len(bucketKeys))
	var last float64
	for i, b := range bucketKeys {
		if len(dist.BucketCounts) > i {
			bs = append(bs, bucket{
				upperBound: b,
				count:      float64(dist.BucketCounts[i]) + last,
			})
			last = float64(dist.BucketCounts[i]) + last
		} else {
			bs = append(bs, bucket{
				upperBound: b,
				count:      last,
			})
		}
	}

	qc := bucketQuantile([]float64{.50, .75, .90, .95, .99, .999}, bs)
	if qc.isNan {
		return quantileContainer{}, errors.New("bucket quantile value is Nan")
	}

	return qc, nil
}

type bucket struct {
	upperBound float64
	count      float64
}

type buckets []bucket

func (b buckets) Len() int           { return len(b) }
func (b buckets) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b buckets) Less(i, j int) bool { return b[i].upperBound < b[j].upperBound }

type quantileContainer struct {
	qs    []float64
	isNan bool
}

func (q quantileContainer) GetQuantiles() []float64 {
	return q.qs
}

func bucketQuantile(qs []float64, buckets buckets) quantileContainer {
	quantile := quantileContainer{
		qs:    make([]float64, 0, len(qs)),
		isNan: true,
	}

	for _, q := range qs {
		if math.IsNaN(q) {
			return quantile
		}
		if q < 0 {
			return quantile
		}
		if q > 1 {
			return quantile
		}
	}

	sort.Sort(buckets)
	if !math.IsInf(buckets[len(buckets)-1].upperBound, +1) {
		return quantile
	}

	buckets = coalesceBuckets(buckets)
	ensureMonotonic(buckets)

	if len(buckets) < 2 {
		return quantile
	}
	observations := buckets[len(buckets)-1].count
	if observations == 0 {
		return quantile
	}

	for _, q := range qs {
		rank := q * observations
		b := sort.Search(len(buckets)-1, func(i int) bool { return buckets[i].count >= rank })

		if b == len(buckets)-1 {
			quantile.qs = append(quantile.qs, buckets[len(buckets)-2].upperBound)
			continue
		}
		if b == 0 && buckets[0].upperBound <= 0 {
			quantile.qs = append(quantile.qs, buckets[0].upperBound)
			continue
		}
		var (
			bucketStart float64
			bucketEnd   = buckets[b].upperBound
			count       = buckets[b].count
		)
		if b > 0 {
			bucketStart = buckets[b-1].upperBound
			count -= buckets[b-1].count
			rank -= buckets[b-1].count
		}
		quantile.qs = append(quantile.qs, bucketStart+(bucketEnd-bucketStart)*(rank/count))
	}
	quantile.isNan = false
	return quantile
}

func coalesceBuckets(buckets buckets) buckets {
	last := buckets[0]
	i := 0
	for _, b := range buckets[1:] {
		if b.upperBound == last.upperBound {
			last.count += b.count
		} else {
			buckets[i] = last
			last = b
			i++
		}
	}
	buckets[i] = last
	return buckets[:i+1]
}

func ensureMonotonic(buckets buckets) {
	maxN := buckets[0].count
	for i := 1; i < len(buckets); i++ {
		switch {
		case buckets[i].count > maxN:
			maxN = buckets[i].count
		case buckets[i].count < maxN:
			buckets[i].count = maxN
		}
	}
}
