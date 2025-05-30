package features

import "sync"

func DepthImb(bid, ask float64) float64 {
	if bid+ask == 0 {
		return 0
	}
	return (bid - ask) / (bid + ask)
}

type TickImb struct {
	buf []int8
	max int
	mu  sync.RWMutex
}

func NewTickImb(n int) *TickImb { return &TickImb{max: n} }

func (t *TickImb) Add(sign int8) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.buf) == t.max {
		t.buf = t.buf[1:]
	}
	t.buf = append(t.buf, sign)
}

func (t *TickImb) Ratio() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if len(t.buf) == 0 {
		return 0
	}
	var s int
	for _, v := range t.buf {
		s += int(v)
	}
	return float64(s) / float64(len(t.buf))
}
