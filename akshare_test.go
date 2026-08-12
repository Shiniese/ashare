package ashare

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// akshareRows mirrors stock_zh_a_hist's 11-column klines layout:
// 日期,开盘,收盘,最高,最低,成交量,成交额,振幅,涨跌幅,涨跌额,换手率
func akshareRows() []string {
	return []string{
		"2026-06-01,21.32,21.35,21.40,21.10,12345,4567890.0,1.41,0.14,0.03,0.32",
		"2026-06-02,21.35,21.50,21.60,21.20,23456,5123456.0,1.87,0.70,0.15,0.61",
	}
}

func TestAkshareDailyParsesBars(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, eastmoneyPayload(akshareRows()))
	}))
	defer srv.Close()

	src := NewAkshare(WithHTTPBaseURL(srv.URL), WithMinInterval(0))
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

func TestAkshareUsesEastmoneyParams(t *testing.T) {
	var last url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		last = r.URL.Query()
		fmt.Fprint(w, eastmoneyPayload(akshareRows()))
	}))
	defer srv.Close()

	src := NewAkshare(WithHTTPBaseURL(srv.URL), WithMinInterval(0))
	sym, _ := ParseSymbol("600519.SH")
	_, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-30"))
	if err != nil {
		t.Fatalf("Daily error: %v", err)
	}
	if got := last.Get("ut"); got != "fa5fd1943c7b386f172d6893dbfba10b" {
		t.Fatalf("ut = %q, want the akshare ut token", got)
	}
	if got := last.Get("secid"); got != "1.600519" {
		t.Fatalf("secid = %q, want 1.600519", got)
	}
	if got := last.Get("fields2"); got != "f51,f52,f53,f54,f55,f56,f57,f58,f59,f60,f61" {
		t.Fatalf("fields2 = %q, want the 11-column akshare set", got)
	}
	if got := last.Get("fqt"); got != "0" {
		t.Fatalf("fqt = %q, want 0 (不复权)", got)
	}
	if got := last.Get("klt"); got != "101" {
		t.Fatalf("klt = %q, want 101", got)
	}
}

func TestAkshareEmptyPayloadIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"rc":0,"data":null}`)
	}))
	defer srv.Close()

	src := NewAkshare(WithHTTPBaseURL(srv.URL), WithMinInterval(0))
	sym, _ := ParseSymbol("600519.SH")
	if _, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-30")); err == nil {
		t.Fatal("expected error for empty payload")
	}
}

func TestAkshareRejectsNonAShare(t *testing.T) {
	src := NewAkshare()
	sym, _ := ParseSymbol("00700.HK")
	if _, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-30")); err == nil {
		t.Fatal("expected error for non-A-share symbol")
	}
}

func TestAkshareAvailableAlwaysTrue(t *testing.T) {
	src := NewAkshare()
	if !src.Available(context.Background()) {
		t.Fatal("akshare should always be available")
	}
	if src.Name() != "akshare" {
		t.Fatalf("name = %s, want akshare", src.Name())
	}
}
