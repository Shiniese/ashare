package ashare

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func eastmoneyPayload(klines []string) string {
	return fmt.Sprintf(`{"rc":0,"rt":1,"data":{"code":"600519","market":1,
		"klines":[%s]}}`, quoteJoin(klines))
}

func quoteJoin(rows []string) string {
	quoted := make([]string, len(rows))
	for i, r := range rows {
		quoted[i] = fmt.Sprintf("%q", r)
	}
	return strings.Join(quoted, ",")
}

func emKlineRows() []string {
	return []string{
		"2026-06-01,21.32,21.35,21.40,21.10,12345,4567890.0",
		"2026-06-02,21.35,21.50,21.60,21.20,23456,5123456.0",
	}
}

func TestEastmoneyDailyParsesBars(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, eastmoneyPayload(emKlineRows()))
	}))
	defer srv.Close()

	src := NewEastmoney(WithHTTPBaseURL(srv.URL), WithMinInterval(0))
	sym, _ := ParseSymbol("600519.SH")
	bars, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-30"))
	if err != nil {
		t.Fatalf("Daily error: %v", err)
	}
	if len(bars) != 2 {
		t.Fatalf("expected 2 bars, got %d", len(bars))
	}
	b0 := bars[0]
	if !b0.Date.Equal(d("2026-06-01")) || b0.Open != 21.32 || b0.Close != 21.35 ||
		b0.High != 21.40 || b0.Low != 21.10 || b0.Volume != 12345 || b0.Amount != 4567890.0 {
		t.Fatalf("unexpected first bar: %+v", b0)
	}
}

func TestEastmoneySecidMapping(t *testing.T) {
	cases := map[string]string{
		"600519.SH": "1.600519",
		"000001.SZ": "0.000001",
		"430139.BJ": "0.430139",
	}
	for symbol, wantSecid := range cases {
		var gotSecid string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotSecid = r.URL.Query().Get("secid")
			fmt.Fprint(w, eastmoneyPayload(emKlineRows()))
		}))
		src := NewEastmoney(WithHTTPBaseURL(srv.URL), WithMinInterval(0))
		sym, _ := ParseSymbol(symbol)
		_, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-30"))
		srv.Close()
		if err != nil {
			t.Fatalf("Daily(%s) error: %v", symbol, err)
		}
		if gotSecid != wantSecid {
			t.Fatalf("secid for %s = %q, want %q", symbol, gotSecid, wantSecid)
		}
	}
}

func TestEastmoneyRequestParams(t *testing.T) {
	var last url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		last = r.URL.Query()
		fmt.Fprint(w, eastmoneyPayload(emKlineRows()))
	}))
	defer srv.Close()

	src := NewEastmoney(WithHTTPBaseURL(srv.URL), WithMinInterval(0))
	sym, _ := ParseSymbol("600519.SH")
	_, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-30"))
	if err != nil {
		t.Fatalf("Daily error: %v", err)
	}
	if got := last.Get("klt"); got != "101" {
		t.Fatalf("klt = %q, want 101", got)
	}
	if got := last.Get("fqt"); got != "0" {
		t.Fatalf("fqt = %q, want 0 (不复权)", got)
	}
	if got := last.Get("beg"); got != "20260601" {
		t.Fatalf("beg = %q, want 20260601", got)
	}
	if got := last.Get("end"); got != "20260630" {
		t.Fatalf("end = %q, want 20260630", got)
	}
	if got := last.Get("fields2"); got != "f51,f52,f53,f54,f55,f56,f57" {
		t.Fatalf("fields2 = %q", got)
	}
}

func TestEastmoneyEmptyPayloadIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"rc":0,"data":null}`)
	}))
	defer srv.Close()

	src := NewEastmoney(WithHTTPBaseURL(srv.URL), WithMinInterval(0))
	sym, _ := ParseSymbol("600519.SH")
	if _, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-30")); err == nil {
		t.Fatal("expected error for empty payload")
	}
}

func TestEastmoneySkipsMalformedRows(t *testing.T) {
	rows := []string{
		"bad-row",
		"2026-06-01,21.32,21.35,21.40,21.10,12345,4567890.0",
		"2026-06-02,21.35,21.50,21.60,21.20,23456,5123456.0",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, eastmoneyPayload(rows))
	}))
	defer srv.Close()

	src := NewEastmoney(WithHTTPBaseURL(srv.URL), WithMinInterval(0))
	sym, _ := ParseSymbol("600519.SH")
	bars, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-30"))
	if err != nil {
		t.Fatalf("Daily error: %v", err)
	}
	if len(bars) != 2 {
		t.Fatalf("expected 2 valid bars (1 bad row skipped), got %d", len(bars))
	}
}

func TestEastmoneyAllRowsMalformedIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, eastmoneyPayload([]string{"bad", "also-bad"}))
	}))
	defer srv.Close()

	src := NewEastmoney(WithHTTPBaseURL(srv.URL), WithMinInterval(0))
	sym, _ := ParseSymbol("600519.SH")
	if _, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-30")); err == nil {
		t.Fatal("expected error when no rows parse")
	}
}

func TestEastmoneyHTTPFailureIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	src := NewEastmoney(WithHTTPBaseURL(srv.URL), WithMinInterval(0))
	sym, _ := ParseSymbol("600519.SH")
	if _, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-30")); err == nil {
		t.Fatal("expected error for HTTP failure")
	}
}

func TestEastmoneyThrottlesRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, eastmoneyPayload(emKlineRows()))
	}))
	defer srv.Close()

	src := NewEastmoney(WithHTTPBaseURL(srv.URL), WithMinInterval(60*time.Millisecond))
	sym, _ := ParseSymbol("600519.SH")
	_, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-30"))
	if err != nil {
		t.Fatalf("Daily error: %v", err)
	}
	start := time.Now()
	_, err = src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-30"))
	if err != nil {
		t.Fatalf("Daily error: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 60*time.Millisecond {
		t.Fatalf("second call not throttled, waited %v", elapsed)
	}
}

func TestEastmoneyAvailableAlwaysTrue(t *testing.T) {
	src := NewEastmoney()
	if !src.Available(context.Background()) {
		t.Fatal("eastmoney should always be available")
	}
	if src.Name() != "eastmoney" {
		t.Fatalf("name = %s, want eastmoney", src.Name())
	}
}
