package features

import (
	"container/ring"
	"math"
	"sync"
	"time"
)

type sample struct {
	p, v float64
	t    time.Time
}

type VWAP struct {
	win  time.Duration
	ring *ring.Ring
	mu   sync.RWMutex
}

func NewVWAP(win time.Duration, size int) *VWAP {
	if size <= 0 {
		size = 1
	}
	return &VWAP{win, ring.New(size), sync.RWMutex{}}
}

func (v *VWAP) Add(price, volume float64) {
	v.mu.Lock()
	v.ring.Value = sample{price, volume, time.Now()}
	v.ring = v.ring.Next()
	v.mu.Unlock()
}

func (v *VWAP) Calc() (value, std float64) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	var pv, vv float64
	var count int
	var sum, sumSquared float64
	cutoff := time.Now().Add(-v.win)

	v.ring.Do(func(x any) {
		if s, ok := x.(sample); ok && s.t.After(cutoff) {
			pv += s.p * s.v
			vv += s.v
			sum += s.p
			sumSquared += s.p * s.p
			count++
		}
	})

	if vv == 0 || count == 0 {
		return 0, 0
	}

	value = pv / vv
	mean := sum / float64(count)
	variance := (sumSquared / float64(count)) - (mean * mean)
	if variance > 0 {
		std = math.Sqrt(variance)
	}
	return
}
