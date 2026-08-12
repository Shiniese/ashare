package ashare

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// tencentPayload builds the exact response shape the real endpoint returns
// for an unadjusted (no "qfq" param) request:
// {"code":0,"data":{"sh600519":{"day":[["2026-06-01","21.32","21.35","21.40","21.10","12345",...], ...]}}}
func tencentPayload(sym string, days [][]string) string {
	payload := map[string]any{
		"code": 0,
		"data": map[string]any{
			sym: map[string]any{"day": days},
		},
	}
	raw, _ := json.Marshal(payload)
	return string(raw)
}

func tencentServer(t *testing.T) (*httptest.Server, func() *http.Request) {
	t.Helper()
	var lastReq *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastReq = r
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, tencentPayload("sh600519", [][]string{
			{"2026-06-01", "21.32", "21.35", "21.40", "21.10", "12345", "0"},
			{"2026-06-02", "21.35", "21.50", "21.60", "21.20", "23456", "0"},
		}))
	}))
	return srv, func() *http.Request { return lastReq }
}

func TestTencentDailyParsesBars(t *testing.T) {
	srv, _ := tencentServer(t)
	defer srv.Close()

	src := NewTencent(WithHTTPBaseURL(srv.URL))
	sym, _ := ParseSymbol("600519.SH")
	bars, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-30"))
	if err != nil {
		t.Fatalf("Daily error: %v", err)
	}
	if len(bars) != 2 {
		t.Fatalf("expected 2 bars, got %d", len(bars))
	}
	b0 := bars[0]
	if !b0.Date.Equal(d("2026-06-01")) || b0.Open != 21.32 || b0.Close != 21.35 || b0.High != 21.40 || b0.Low != 21.10 || b0.Volume != 12345 {
		t.Fatalf("unexpected first bar: %+v", b0)
	}
	if !bars[1].Date.Equal(d("2026-06-02")) || bars[1].Open != 21.35 {
		t.Fatalf("unexpected second bar: %+v", bars[1])
	}
}

func TestTencentRequestsUnadjustedDayAndSymbolPrefix(t *testing.T) {
	srv, lastReq := tencentServer(t)
	defer srv.Close()

	src := NewTencent(WithHTTPBaseURL(srv.URL))
	sym, _ := ParseSymbol("000001.SZ")
	_, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-30"))
	if err != nil {
		t.Fatalf("Daily error: %v", err)
	}
	query := lastReq().URL.Query()
	param := query.Get("param")
	if !strings.HasPrefix(param, "sz000001,day,") {
		t.Fatalf("param = %q, want sz000001,day,... prefix", param)
	}
	if strings.HasSuffix(param, ",qfq") {
		t.Fatalf("param = %q, want unadjusted (no qfq suffix)", param)
	}
	if !strings.HasSuffix(param, ",") || strings.Count(param, ",") != 5 {
		t.Fatalf("param = %q, want 4 commas (symbol,day,start,end,500,)", param)
	}
}

func TestTencentRequiresBrowserHeaders(t *testing.T) {
	srv, lastReq := tencentServer(t)
	defer srv.Close()

	src := NewTencent(WithHTTPBaseURL(srv.URL))
	sym, _ := ParseSymbol("600519.SH")
	_, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-30"))
	if err != nil {
		t.Fatalf("Daily error: %v", err)
	}
	ua := lastReq().Header.Get("User-Agent")
	if !strings.Contains(ua, "Mozilla") {
		t.Fatalf("User-Agent = %q, want a browser UA", ua)
	}
}

func TestTencentRequiresDayKey(t *testing.T) {
	payload := map[string]any{
		"code": 0,
		"data": map[string]any{
			"sh600519": map[string]any{
				"qfqday": [][]string{{"2026-06-01", "10.0", "10.1", "10.2", "9.9", "1000", "0"}},
			},
		},
	}
	raw, _ := json.Marshal(payload)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, string(raw))
	}))
	defer srv.Close()

	src := NewTencent(WithHTTPBaseURL(srv.URL))
	sym, _ := ParseSymbol("600519.SH")
	if _, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-30")); err == nil {
		t.Fatal("expected error: qfqday-only payload has no unadjusted day rows")
	}
}

func TestTencentRejectsNonAShareSymbol(t *testing.T) {
	srv, _ := tencentServer(t)
	defer srv.Close()

	src := NewTencent(WithHTTPBaseURL(srv.URL))
	sym, _ := ParseSymbol("AAPL.US")
	if _, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-30")); err == nil {
		t.Fatal("expected error for non-A-share symbol")
	}
}

func TestTencentEmptyDataIsError(t *testing.T) {
	payload := `{"code":0,"data":{}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, payload)
	}))
	defer srv.Close()

	src := NewTencent(WithHTTPBaseURL(srv.URL))
	sym, _ := ParseSymbol("600519.SH")
	if _, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-30")); err == nil {
		t.Fatal("expected error for empty payload")
	}
}

func TestTencentHTTPFailureIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	src := NewTencent(WithHTTPBaseURL(srv.URL))
	sym, _ := ParseSymbol("600519.SH")
	if _, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-30")); err == nil {
		t.Fatal("expected error for HTTP failure")
	}
}

func TestTencentAvailableAlwaysTrue(t *testing.T) {
	src := NewTencent()
	if !src.Available(context.Background()) {
		t.Fatal("tencent should always be available")
	}
	if src.Name() != "tencent" {
		t.Fatalf("name = %s, want tencent", src.Name())
	}
}

var _ = time.Now

// 真实响应里，除权除息日的行尾部带有分红信息 object，如
// ["2024-12-20","1451.549",...,"28158.000",{"nd":"2024","FHcontent":"10派238.82元"}]。
// 解析必须容忍这种行。
func TestTencentToleratesDividendMetaRow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		payload := map[string]any{
			"code": 0,
			"data": map[string]any{
				"sh600519": map[string]any{"day": [][]any{
					{"2026-06-01", "21.32", "21.35", "21.40", "21.10", "12345", "0"},
					{"2026-06-02", "21.35", "21.50", "21.60", "21.20", "23456",
						map[string]any{"nd": "2026", "fh_sh": "238.82", "FHcontent": "10派238.82元"}},
				}},
			},
		}
		raw, _ := json.Marshal(payload)
		fmt.Fprint(w, string(raw))
	}))
	defer srv.Close()

	src := NewTencent(WithHTTPBaseURL(srv.URL))
	sym, _ := ParseSymbol("600519.SH")
	bars, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-30"))
	if err != nil {
		t.Fatalf("Daily error: %v", err)
	}
	if len(bars) != 2 {
		t.Fatalf("expected 2 bars, got %d", len(bars))
	}
	if !bars[1].Date.Equal(d("2026-06-02")) || bars[1].Close != 21.50 {
		t.Fatalf("unexpected second bar (dividend row): %+v", bars[1])
	}
}

func TestTencentSplitsLongRangesIntoPages(t *testing.T) {
	var reqCount int
	var params []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		param := r.URL.Query().Get("param")
		reqCount++
		params = append(params, param)
		w.Header().Set("Content-Type", "application/json")
		// Reply with one bar at the segment's own start date.
		parts := strings.Split(param, ",")
		fmt.Fprint(w, tencentPayload("sh600519", [][]string{
			{parts[2], "10.0", "10.1", "10.2", "9.9", "1000", "0"},
		}))
	}))
	defer srv.Close()

	src := NewTencent(WithHTTPBaseURL(srv.URL))
	sym, _ := ParseSymbol("600519.SH")
	bars, err := src.Daily(context.Background(), sym, d("2024-01-02"), d("2026-06-30"))
	if err != nil {
		t.Fatalf("Daily error: %v", err)
	}
	if reqCount < 5 {
		t.Fatalf("expected multiple paged requests for a 2.5y window, got %d", reqCount)
	}
	if len(bars) != reqCount {
		t.Fatalf("expected %d merged bars, got %d", reqCount, len(bars))
	}
	// Segments must cover the whole window in ascending order.
	if bars[0].Date.Before(d("2024-01-02")) || bars[len(bars)-1].Date.After(d("2026-06-30")) {
		t.Fatalf("bars outside window: %v", bars)
	}
	if !bars[0].Date.Equal(d("2024-01-02")) {
		t.Fatalf("first merged bar = %v, want 2024-01-02", bars[0].Date)
	}
}
