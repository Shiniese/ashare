package ashare

import (
	"testing"
	"time"
)

func d(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestNormalizeBarsEmpty(t *testing.T) {
	if got := normalizeBars(nil, d("2024-01-01"), d("2024-01-31")); len(got) != 0 {
		t.Fatalf("expected empty result, got %d bars", len(got))
	}
}

func TestNormalizeBarsSortsAscending(t *testing.T) {
	in := []Bar{
		{Date: d("2024-01-03"), Open: 1, High: 2, Low: 1, Close: 2},
		{Date: d("2024-01-01"), Open: 1, High: 2, Low: 1, Close: 2},
		{Date: d("2024-01-02"), Open: 1, High: 2, Low: 1, Close: 2},
	}
	got := normalizeBars(in, d("2024-01-01"), d("2024-01-31"))
	if len(got) != 3 {
		t.Fatalf("expected 3 bars, got %d", len(got))
	}
	if !got[0].Date.Equal(d("2024-01-01")) || !got[2].Date.Equal(d("2024-01-03")) {
		t.Fatalf("bars not sorted ascending: %v", got)
	}
}

func TestNormalizeBarsDeduplicatesByDate(t *testing.T) {
	in := []Bar{
		{Date: d("2024-01-01"), Open: 1, Close: 2},
		{Date: d("2024-01-01"), Open: 3, Close: 4},
	}
	got := normalizeBars(in, d("2024-01-01"), d("2024-01-31"))
	if len(got) != 1 {
		t.Fatalf("expected 1 bar after dedupe, got %d", len(got))
	}
	if got[0].Open != 1 || got[0].Close != 2 {
		t.Fatalf("expected first occurrence kept, got %+v", got[0])
	}
}

func TestNormalizeBarsClipsToRange(t *testing.T) {
	in := []Bar{
		{Date: d("2023-12-31"), Open: 1, Close: 1},
		{Date: d("2024-01-15"), Open: 1, Close: 1},
		{Date: d("2024-02-01"), Open: 1, Close: 1},
	}
	got := normalizeBars(in, d("2024-01-01"), d("2024-01-31"))
	if len(got) != 1 {
		t.Fatalf("expected 1 bar in range, got %d", len(got))
	}
	if !got[0].Date.Equal(d("2024-01-15")) {
		t.Fatalf("got unexpected bar %v", got[0].Date)
	}
}

func TestNormalizeBarsDropsMalformedBars(t *testing.T) {
	in := []Bar{
		{Date: d("2024-01-02"), Open: 1, Close: 2},
		{Date: d("2024-01-01")}, // missing OHLC
		{Date: d("2024-01-03"), Open: 1, High: 1, Low: 1, Close: 1, Volume: 0},
	}
	got := normalizeBars(in, d("2024-01-01"), d("2024-01-31"))
	if len(got) != 2 {
		t.Fatalf("expected 2 valid bars, got %d", len(got))
	}
}
