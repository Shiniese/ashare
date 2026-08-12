package ashare

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// newTestClient returns a Client with no dividend events and the given
// chain, so Daily applies AdjustBars with an empty event list (no-op).
func newTestClient(sources ...Source) *Client {
	c := New()
	if err := c.Use(sources...); err != nil {
		panic(err)
	}
	c.UseEvents(func(ctx context.Context, sym Symbol) ([]DividendEvent, error) { return nil, nil })
	return c
}

type fakeSource struct {
	name      string
	available bool
	bars      []Bar
	err       error
	calls     int
}

func (f *fakeSource) Name() string { return f.name }

func (f *fakeSource) Available(ctx context.Context) bool { return f.available }

func (f *fakeSource) Daily(ctx context.Context, sym Symbol, start, end time.Time) ([]Bar, error) {
	f.calls++
	return f.bars, f.err
}

func TestDailyReturnsFirstAvailableSource(t *testing.T) {
	want := []Bar{{Date: d("2024-01-02"), Open: 1, Close: 2}}
	c := newTestClient(
		&fakeSource{name: "first", available: true, bars: want},
		&fakeSource{name: "second", available: true},
	)
	bars, source, err := c.Daily(context.Background(), "600519.SH", "2024-01-01", "2024-01-31")
	if err != nil {
		t.Fatalf("Daily error: %v", err)
	}
	if source != "first" {
		t.Fatalf("source = %s, want first", source)
	}
	if len(bars) != 1 {
		t.Fatalf("expected 1 bar, got %d", len(bars))
	}
}

func TestDailySkipsUnavailableSource(t *testing.T) {
	c := newTestClient(
		&fakeSource{name: "off", available: false},
		&fakeSource{name: "on", available: true, bars: []Bar{{Date: d("2024-01-02"), Open: 1, Close: 2}}},
	)
	_, source, err := c.Daily(context.Background(), "600519.SH", "2024-01-01", "2024-01-31")
	if err != nil {
		t.Fatalf("Daily error: %v", err)
	}
	if source != "on" {
		t.Fatalf("source = %s, want on", source)
	}
}

func TestDailyWalksChainWhenSourceReturnsEmpty(t *testing.T) {
	c := newTestClient(
		&fakeSource{name: "empty", available: true},
		&fakeSource{name: "filled", available: true, bars: []Bar{{Date: d("2024-01-02"), Open: 1, Close: 2}}},
	)
	_, source, err := c.Daily(context.Background(), "600519.SH", "2024-01-01", "2024-01-31")
	if err != nil {
		t.Fatalf("Daily error: %v", err)
	}
	if source != "filled" {
		t.Fatalf("source = %s, want filled", source)
	}
}

func TestDailyWalksChainWhenSourceErrors(t *testing.T) {
	c := newTestClient(
		&fakeSource{name: "broken", available: true, err: errors.New("boom")},
		&fakeSource{name: "healthy", available: true, bars: []Bar{{Date: d("2024-01-02"), Open: 1, Close: 2}}},
	)
	_, source, err := c.Daily(context.Background(), "600519.SH", "2024-01-01", "2024-01-31")
	if err != nil {
		t.Fatalf("Daily error: %v", err)
	}
	if source != "healthy" {
		t.Fatalf("source = %s, want healthy", source)
	}
}

func TestDailyAllFailReturnsNoAvailableSourceError(t *testing.T) {
	c := newTestClient(
		&fakeSource{name: "a", available: true, err: errors.New("boom")},
		&fakeSource{name: "b", available: false},
	)
	_, _, err := c.Daily(context.Background(), "600519.SH", "2024-01-01", "2024-01-31")
	var nse *NoAvailableSourceError
	if !errors.As(err, &nse) {
		t.Fatalf("expected NoAvailableSourceError, got %v", err)
	}
	if len(nse.Tried) != 2 {
		t.Fatalf("expected 2 tried sources, got %d", len(nse.Tried))
	}
	if !strings.Contains(nse.Error(), "a") || !strings.Contains(nse.Error(), "b") {
		t.Fatalf("error should mention tried sources: %v", nse.Error())
	}
}

func TestDailyUnavailableSourcesAreRecordedAsTried(t *testing.T) {
	c := newTestClient(&fakeSource{name: "off", available: false})
	_, _, err := c.Daily(context.Background(), "600519.SH", "2024-01-01", "2024-01-31")
	var nse *NoAvailableSourceError
	if !errors.As(err, &nse) {
		t.Fatalf("expected NoAvailableSourceError, got %v", err)
	}
	if nse.Tried[0].Name != "off" {
		t.Fatalf("unavailable source should be recorded, got %+v", nse.Tried)
	}
}

func TestDailyRejectsInvalidSymbol(t *testing.T) {
	c := newTestClient(&fakeSource{name: "s", available: true})
	if _, _, err := c.Daily(context.Background(), "AAPL.US", "2024-01-01", "2024-01-31"); err == nil {
		t.Fatal("expected error for non-A-share symbol")
	}
}

func TestDailyRejectsInvertedDateRange(t *testing.T) {
	c := newTestClient(&fakeSource{name: "s", available: true})
	if _, _, err := c.Daily(context.Background(), "600519.SH", "2024-02-01", "2024-01-01"); err == nil {
		t.Fatal("expected error for inverted date range")
	}
}

func TestDailyRejectsMalformedDate(t *testing.T) {
	c := newTestClient(&fakeSource{name: "s", available: true})
	if _, _, err := c.Daily(context.Background(), "600519.SH", "yesterday", "2024-01-01"); err == nil {
		t.Fatal("expected error for malformed date")
	}
}

func TestDailyNormalizesBars(t *testing.T) {
	outOfOrder := []Bar{
		{Date: d("2024-01-03"), Open: 1, Close: 3},
		{Date: d("2024-01-01"), Open: 1, Close: 1},
		{Date: d("2024-01-02"), Open: 1, Close: 2},
		{Date: d("2023-12-31"), Open: 1, Close: 0},
	}
	c := newTestClient(&fakeSource{name: "s", available: true, bars: outOfOrder})
	bars, _, err := c.Daily(context.Background(), "600519.SH", "2024-01-01", "2024-01-31")
	if err != nil {
		t.Fatalf("Daily error: %v", err)
	}
	if len(bars) != 3 {
		t.Fatalf("expected 3 in-range sorted bars, got %d", len(bars))
	}
	if !bars[0].Date.Equal(d("2024-01-01")) || !bars[2].Date.Equal(d("2024-01-03")) {
		t.Fatalf("bars not sorted/clipped: %v", bars)
	}
}

func TestUseRejectsNilSource(t *testing.T) {
	c := New()
	if err := c.Use(nil); err == nil {
		t.Fatal("expected error for nil source")
	}
}

func TestUseRejectsDuplicateNames(t *testing.T) {
	c := New()
	if err := c.Use(&fakeSource{name: "dup"}, &fakeSource{name: "dup"}); err == nil {
		t.Fatal("expected error for duplicate source name")
	}
}

func TestDailyUsesProvidedContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := newTestClient(&fakeSource{name: "s", available: true})
	if _, _, err := c.Daily(ctx, "600519.SH", "2024-01-01", "2024-01-31"); err == nil {
		t.Fatal("expected error on canceled context")
	}
}

func TestDailySkipsFetchWhenSymbolDateInvalid(t *testing.T) {
	f := &fakeSource{name: "s", available: true}
	c := newTestClient(f)
	_, _, _ = c.Daily(context.Background(), "bad", "2024-01-01", "2024-01-31")
	if f.calls != 0 {
		t.Fatalf("source should not be called for invalid symbol, called %d times", f.calls)
	}
}

func TestDailyAppliesCentralAdjustment(t *testing.T) {
	raw := []Bar{{Date: d("2026-05-20"), Open: 1315, High: 1315, Low: 1315, Close: 1315}}
	c := newTestClient(&fakeSource{name: "s", available: true, bars: raw})
	c.UseEvents(func(ctx context.Context, sym Symbol) ([]DividendEvent, error) {
		return dividendEvents600519, nil
	})
	bars, _, err := c.Daily(context.Background(), "600519.SH", "2026-05-20", "2026-05-20")
	if err != nil {
		t.Fatalf("Daily error: %v", err)
	}
	if bars[0].Close != 1286.98 { // 1315.00 - 28.0242 (2026-06-26 事件)
		t.Fatalf("close = %.2f, want 1286.98 (central qfq)", bars[0].Close)
	}
}

func TestDailyCachesDividendEvents(t *testing.T) {
	raw := []Bar{{Date: d("2026-08-04"), Close: 1328.36}}
	c := newTestClient(&fakeSource{name: "s", available: true, bars: raw})
	fetches := 0
	c.UseEvents(func(ctx context.Context, sym Symbol) ([]DividendEvent, error) {
		fetches++
		return dividendEvents600519, nil
	})
	for i := 0; i < 3; i++ {
		if _, _, err := c.Daily(context.Background(), "600519.SH", "2026-08-04", "2026-08-04"); err != nil {
			t.Fatalf("Daily error: %v", err)
		}
	}
	if fetches != 1 {
		t.Fatalf("events fetched %d times, want 1 (cached)", fetches)
	}
}

func TestDailyPropagatesDividendEventsError(t *testing.T) {
	raw := []Bar{{Date: d("2026-08-04"), Close: 1328.36}}
	c := newTestClient(&fakeSource{name: "s", available: true, bars: raw})
	c.UseEvents(func(ctx context.Context, sym Symbol) ([]DividendEvent, error) {
		return nil, errors.New("xdxr down")
	})
	if _, _, err := c.Daily(context.Background(), "600519.SH", "2026-08-04", "2026-08-04"); err == nil {
		t.Fatal("expected error when dividend events are unavailable")
	}
}
