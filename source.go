package ashare

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Source is a single A-share daily-bar data provider. Implementations must be
// safe for concurrent use.
type Source interface {
	// Name is the stable source identifier, e.g. "tencent".
	Name() string
	// Available reports whether the source is usable right now (e.g. a
	// Tushare token is configured, or the TCP endpoint is reachable).
	Available(ctx context.Context) bool
	// Daily fetches daily bars for sym within [start, end] (inclusive).
	Daily(ctx context.Context, sym Symbol, start, end time.Time) ([]Bar, error)
}

// TriedSource records one fallback-chain attempt.
type TriedSource struct {
	Name  string
	Error error // nil when the source was merely unavailable
}

// NoAvailableSourceError is returned when every source in the fallback chain
// failed. Tried lists each attempted source and its failure reason.
type NoAvailableSourceError struct {
	Tried []TriedSource
}

func (e *NoAvailableSourceError) Error() string {
	parts := make([]string, 0, len(e.Tried))
	for _, ts := range e.Tried {
		if ts.Error != nil {
			parts = append(parts, fmt.Sprintf("%s (%v)", ts.Name, ts.Error))
		} else {
			parts = append(parts, ts.Name+" (unavailable)")
		}
	}
	return "no available data source for a_share; tried: " + strings.Join(parts, "; ")
}
