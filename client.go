package ashare

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// eventsCacheTTL is how long a symbol's 除权除息 event list is cached.
// Dividend events change a few times a year at most, so a day is plenty.
const eventsCacheTTL = 24 * time.Hour

// dividendEventsFn fetches the dividend-event history for a symbol; it is
// injectable for tests via UseEvents.
type dividendEventsFn func(ctx context.Context, sym Symbol) ([]DividendEvent, error)

// Client is the smart fallback entry point. It walks an ordered chain of
// sources (by IP-ban risk) and returns the first usable result, then applies
// a single forward-adjustment (前复权) on top of the raw bars using TDX
// XDXR dividend events, so every source yields identical adjusted prices.
type Client struct {
	sources []Source
	events  dividendEventsFn
	mu      sync.Mutex
	cache   map[string]eventsCacheEntry
}

type eventsCacheEntry struct {
	events []DividendEvent
	at     time.Time
}

// New returns a Client with the default A-share chain:
// tencent -> mootdx -> eastmoney -> baostock -> akshare -> tushare.
func New() *Client {
	return &Client{
		sources: DefaultChain(),
		events:  defaultDividendEvents,
		cache:   make(map[string]eventsCacheEntry),
	}
}

// defaultDividendEvents fetches dividend events from a live TDX server.
func defaultDividendEvents(ctx context.Context, sym Symbol) ([]DividendEvent, error) {
	return NewTdx().DividendEvents(ctx, sym)
}

// UseEvents replaces the dividend-event provider. The default connects to a
// live TDX server; tests substitute a fake.
func (c *Client) UseEvents(fn dividendEventsFn) { c.events = fn }

// Use replaces the fallback chain with the given sources, in order. All
// previously configured sources are removed.
func (c *Client) Use(sources ...Source) error {
	seen := make(map[string]bool, len(sources))
	for _, s := range sources {
		if s == nil {
			return errors.New("ashare: nil source")
		}
		if seen[s.Name()] {
			return fmt.Errorf("ashare: duplicate source name %q", s.Name())
		}
		seen[s.Name()] = true
	}
	c.sources = append([]Source(nil), sources...)
	return nil
}

// Daily fetches forward-adjusted (前复权) daily bars for an A-share symbol
// between start and end (both YYYY-MM-DD, inclusive), walking the fallback
// chain until one source succeeds. Every source is served raw (不复权) and
// adjusted identically from TDX dividend events, so results are 1:1 across
// sources. It returns the bars, the name of the serving source, or a
// *NoAvailableSourceError when every source failed.
func (c *Client) Daily(ctx context.Context, symbol, start, end string) ([]Bar, string, error) {
	sym, err := ParseSymbol(symbol)
	if err != nil {
		return nil, "", err
	}
	startT, err := time.Parse("2006-01-02", start)
	if err != nil {
		return nil, "", fmt.Errorf("ashare: invalid start date %q", start)
	}
	endT, err := time.Parse("2006-01-02", end)
	if err != nil {
		return nil, "", fmt.Errorf("ashare: invalid end date %q", end)
	}
	if startT.After(endT) {
		return nil, "", fmt.Errorf("ashare: start date %s > end date %s", start, end)
	}

	var tried []TriedSource
	for _, s := range c.sources {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		if !s.Available(ctx) {
			tried = append(tried, TriedSource{Name: s.Name()})
			continue
		}
		bars, err := s.Daily(ctx, sym, startT, endT)
		if err != nil || len(bars) == 0 {
			tried = append(tried, TriedSource{Name: s.Name(), Error: err})
			continue
		}
		normalized := normalizeBars(bars, startT, endT)
		events, err := c.eventsCached(ctx, sym)
		if err != nil {
			return nil, "", fmt.Errorf("ashare: dividend events for %s: %w", sym, err)
		}
		return AdjustBars(events, normalized), s.Name(), nil
	}
	return nil, "", &NoAvailableSourceError{Tried: tried}
}

// eventsCached returns the cached dividend events for a symbol, fetching
// them through the configured provider on a miss.
func (c *Client) eventsCached(ctx context.Context, sym Symbol) ([]DividendEvent, error) {
	key := sym.String()
	c.mu.Lock()
	if e, ok := c.cache[key]; ok && time.Since(e.at) < eventsCacheTTL {
		c.mu.Unlock()
		return e.events, nil
	}
	c.mu.Unlock()

	events, err := c.events(ctx, sym)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.cache[key] = eventsCacheEntry{events: events, at: time.Now()}
	c.mu.Unlock()
	return events, nil
}
