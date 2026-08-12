package ashare

import (
	"context"
	"math/rand"
	"sync"
	"time"
)

// maxJitter is the upper bound (in seconds) of random jitter added on top of
// the minimum interval so parallel callers de-synchronize instead of all
// firing at the same instant.
const maxJitter = 400 * time.Millisecond

// HostThrottle is a process-wide minimum-spacing gate keyed by an arbitrary
// host bucket, mirroring the Python _http.HostThrottle. Wait blocks until at
// least minInterval (plus jitter) has elapsed since the last call tagged with
// the same bucket. Distinct buckets never block one another.
type HostThrottle struct {
	mu         sync.Mutex
	last       map[string]time.Time
	lastSweep  time.Time
	rng        *rand.Rand
}

// NewHostThrottle returns an empty throttle gate.
func NewHostThrottle() *HostThrottle {
	return &HostThrottle{
		last:      make(map[string]time.Time),
		lastSweep: time.Now(),
		rng:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// eastmoneyThrottle is the process-wide gate shared by every source that hits
// Eastmoney hosts (eastmoney and akshare), mirroring the Python _THROTTLE.
var eastmoneyThrottle = NewHostThrottle()

// Wait blocks until bucket may fire again (respecting ctx cancellation).
func (h *HostThrottle) Wait(ctx context.Context, bucket string, minInterval time.Duration) error {
	if minInterval <= 0 {
		return nil
	}
	h.mu.Lock()
	if time.Since(h.lastSweep) >= time.Minute {
		cutoff := time.Now().Add(-time.Hour)
		for k, last := range h.last {
			if last.Before(cutoff) {
				delete(h.last, k)
			}
		}
		h.lastSweep = time.Now()
	}
	now := time.Now()
	last, ok := h.last[bucket]
	if !ok || now.Sub(last) >= minInterval {
		h.last[bucket] = now
		h.mu.Unlock()
		return nil
	}
	fireAt := last.Add(minInterval + time.Duration(h.rng.Float64()*float64(maxJitter)))
	h.last[bucket] = fireAt
	h.mu.Unlock()

	timer := time.NewTimer(time.Until(fireAt))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
