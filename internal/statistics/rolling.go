package statistics

import "math"

type Rolling struct {
	values []float64
	next   int
	count  int
	sum    float64
	sumSq  float64
}

type Snapshot struct {
	Count  int
	Mean   float64
	StdDev float64
	ZScore float64
}

func NewRolling(window int) *Rolling {
	if window < 2 {
		window = 2
	}
	return &Rolling{values: make([]float64, window)}
}

func (r *Rolling) Add(value float64) Snapshot {
	if r.count == len(r.values) {
		old := r.values[r.next]
		r.sum -= old
		r.sumSq -= old * old
	} else {
		r.count++
	}
	r.values[r.next] = value
	r.next = (r.next + 1) % len(r.values)
	r.sum += value
	r.sumSq += value * value

	mean := r.sum / float64(r.count)
	variance := r.sumSq/float64(r.count) - mean*mean
	if variance < 0 && variance > -1e-12 {
		variance = 0
	}
	stdDev := math.Sqrt(math.Max(0, variance))
	zScore := 0.0
	if stdDev > 0 {
		zScore = (value - mean) / stdDev
	}
	return Snapshot{Count: r.count, Mean: mean, StdDev: stdDev, ZScore: zScore}
}

func (r *Rolling) Ready(minimum int) bool {
	return r.count >= minimum
}
