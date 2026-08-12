package ashare

import (
	"math"
	"sort"
	"time"
)

// Bar is one OHLCV bar. Amount is zero when the provider does not report it.
type Bar struct {
	Date   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
	Amount float64

	factor float64 // internal: qfq adjustment factor of this bar
}

func validBar(b Bar) bool {
	if b.Date.IsZero() {
		return false
	}
	for _, v := range []float64{b.Open, b.High, b.Low, b.Close} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return false
		}
	}
	// An all-zero OHLC row is a gap artifact, not a real bar.
	if b.Open == 0 && b.High == 0 && b.Low == 0 && b.Close == 0 {
		return false
	}
	return true
}

// normalizeBars sorts ascending by date, deduplicates by date (first
// occurrence wins), clips to [start, end] and drops malformed bars.
func normalizeBars(bars []Bar, start, end time.Time) []Bar {
	out := make([]Bar, 0, len(bars))
	for _, b := range bars {
		if !validBar(b) {
			continue
		}
		if b.Date.Before(start) || b.Date.After(end) {
			continue
		}
		out = append(out, b)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	seen := make(map[time.Time]bool, len(out))
	uniq := out[:0]
	for _, b := range out {
		if seen[b.Date] {
			continue
		}
		seen[b.Date] = true
		uniq = append(uniq, b)
	}
	return uniq
}
