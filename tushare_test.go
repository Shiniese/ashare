package ashare

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type tushareRow struct {
	tsCode, tradeDate string
	open, high, low   float64
	close_            float64
	vol, amount       float64
}

func tushareDailyPayload(rows []tushareRow) string {
	fields := []string{"ts_code", "trade_date", "open", "high", "low", "close", "vol", "amount"}
	items := make([][]any, len(rows))
	for i, r := range rows {
		items[i] = []any{r.tsCode, r.tradeDate, r.open, r.high, r.low, r.close_, r.vol, r.amount}
	}
	return tushareOKEnvelope(0, "", fields, items)
}

func tushareAdjPayload(rows [][]any) string {
	return tushareOKEnvelope(0, "", []string{"ts_code", "trade_date", "adj_factor"}, rows)
}

func tushareOKEnvelope(code int, msg string, fields []string, items [][]any) string {
	payload := map[string]any{
		"code": code,
		"msg":  msg,
		"data": map[string]any{"fields": fields, "items": items},
	}
	raw, _ := json.Marshal(payload)
	return string(raw)
}

func tushareErrorEnvelope(msg string) string {
	raw, _ := json.Marshal(map[string]any{"code": 1, "msg": msg})
	return string(raw)
}

type tushareStub struct {
	mu        sync.Mutex
	requests  []map[string]any
	failFirst int // respond with a rate-limit error for the first N business calls
}

func (s *tushareStub) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		s.requests = append(s.requests, body)
		apiName, _ := body["api_name"].(string)
		fail := s.failFirst > 0
		if fail {
			s.failFirst--
		}
		s.mu.Unlock()

		if fail {
			fmt.Fprint(w, tushareErrorEnvelope("每分钟最多访问该接口5次，请稍候再试"))
			return
		}
		switch apiName {
		case "daily":
			fmt.Fprint(w, tushareDailyPayload([]tushareRow{
				{"600519.SH", "20260601", 10.0, 11.0, 9.5, 10.5, 10000, 105000},
				{"600519.SH", "20260602", 10.5, 11.5, 10.2, 11.0, 9000, 99000},
			}))
		case "adj_factor":
			fmt.Fprint(w, tushareAdjPayload([][]any{
				{"600519.SH", "20260601", 1.0},
				{"600519.SH", "20260602", 1.25},
			}))
		case "fund_daily":
			fmt.Fprint(w, tushareDailyPayload([]tushareRow{
				{"510050.SH", "20260601", 3.0, 3.1, 2.9, 3.05, 50000, 152500},
			}))
		case "fund_adj":
			fmt.Fprint(w, tushareAdjPayload([][]any{
				{"510050.SH", "20260601", 1.0},
			}))
		case "index_daily":
			fmt.Fprint(w, tushareDailyPayload([]tushareRow{
				{"000300.SH", "20260601", 3500.0, 3520.0, 3480.0, 3510.0, 100000, 3.5e9},
			}))
		default:
			fmt.Fprint(w, tushareErrorEnvelope("unknown api_name"))
		}
	}
}

func TestTushareDailyParsesBars(t *testing.T) {
	stub := &tushareStub{}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	src := NewTushare("tok", WithHTTPBaseURL(srv.URL))
	sym, _ := ParseSymbol("600519.SH")
	bars, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-02"))
	if err != nil {
		t.Fatalf("Daily error: %v", err)
	}
	if len(bars) != 2 {
		t.Fatalf("expected 2 bars, got %d", len(bars))
	}
	// Unadjusted (不复权): raw rows pass through untouched.
	b0 := bars[0]
	if !b0.Date.Equal(d("2026-06-01")) || !approx(b0.Open, 10.0) || !approx(b0.Close, 10.5) ||
		!approx(b0.High, 11.0) || !approx(b0.Low, 9.5) || !approx(b0.Volume, 10000) || !approx(b0.Amount, 105000) {
		t.Fatalf("unexpected first bar: %+v", b0)
	}
}

func approx(a, b float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < 1e-6
}

func TestTushareRequestEnvelope(t *testing.T) {
	stub := &tushareStub{}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	src := NewTushare("tok", WithHTTPBaseURL(srv.URL))
	sym, _ := ParseSymbol("600519.SH")
	_, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-02"))
	if err != nil {
		t.Fatalf("Daily error: %v", err)
	}

	stub.mu.Lock()
	defer stub.mu.Unlock()
	daily := stub.requests[0]
	if daily["api_name"] != "daily" {
		t.Fatalf("api_name = %v, want daily", daily["api_name"])
	}
	if daily["token"] != "tok" {
		t.Fatalf("token = %v, want tok", daily["token"])
	}
	params, _ := daily["params"].(map[string]any)
	if params["ts_code"] != "600519.SH" || params["start_date"] != "20260601" || params["end_date"] != "20260602" {
		t.Fatalf("params = %v", params)
	}
}

func TestTushareReturnsUnadjustedBars(t *testing.T) {
	stub := &tushareStub{}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	src := NewTushare("tok", WithHTTPBaseURL(srv.URL))
	sym, _ := ParseSymbol("600519.SH")
	bars, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-02"))
	if err != nil {
		t.Fatalf("Daily error: %v", err)
	}
	// Raw rows, no adj_factor call, no qfq scaling.
	if bars[0].Close != 10.5 || bars[1].Close != 11.0 {
		t.Fatalf("expected raw closes: %+v", bars)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	for _, r := range stub.requests {
		if r["api_name"] == "adj_factor" {
			t.Fatal("must not call adj_factor: adjustment is central now")
		}
	}
}

func TestTushareRoutesETFToFundDaily(t *testing.T) {
	stub := &tushareStub{}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	src := NewTushare("tok", WithHTTPBaseURL(srv.URL))
	sym, _ := ParseSymbol("510050.SH")
	bars, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-02"))
	if err != nil {
		t.Fatalf("Daily error: %v", err)
	}
	if len(bars) != 1 {
		t.Fatalf("expected 1 bar, got %d", len(bars))
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.requests[0]["api_name"] != "fund_daily" {
		t.Fatalf("api_name = %v, want fund_daily", stub.requests[0]["api_name"])
	}
}

func TestTushareRoutesIndexToIndexDaily(t *testing.T) {
	stub := &tushareStub{}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	src := NewTushare("tok", WithHTTPBaseURL(srv.URL))
	sym, _ := ParseSymbol("000300.SH")
	bars, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-02"))
	if err != nil {
		t.Fatalf("Daily error: %v", err)
	}
	if len(bars) != 1 {
		t.Fatalf("expected 1 bar, got %d", len(bars))
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.requests[0]["api_name"] != "index_daily" {
		t.Fatalf("api_name = %v, want index_daily", stub.requests[0]["api_name"])
	}
	if len(stub.requests) != 1 {
		t.Fatalf("index must not fetch adj_factor, got %d requests", len(stub.requests))
	}
}

func TestTushareRetriesRateLimit(t *testing.T) {
	stub := &tushareStub{failFirst: 1}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	src := NewTushare("tok", WithHTTPBaseURL(srv.URL), WithRateLimitBackoff(time.Millisecond))
	sym, _ := ParseSymbol("600519.SH")
	bars, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-02"))
	if err != nil {
		t.Fatalf("Daily error: %v", err)
	}
	if len(bars) != 2 {
		t.Fatalf("expected 2 bars after retry, got %d", len(bars))
	}
}

func TestTushareRateLimitExhaustedIsError(t *testing.T) {
	stub := &tushareStub{failFirst: 100}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	src := NewTushare("tok", WithHTTPBaseURL(srv.URL), WithRateLimitBackoff(time.Millisecond))
	sym, _ := ParseSymbol("600519.SH")
	if _, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-02")); err == nil {
		t.Fatal("expected error when rate limit persists")
	}
}

func TestTushareAvailableRequiresRealToken(t *testing.T) {
	ctx := context.Background()
	if NewTushare("").Available(ctx) {
		t.Fatal("empty token must be unavailable")
	}
	if NewTushare("your-tushare-token").Available(ctx) {
		t.Fatal("placeholder token must be unavailable")
	}
	if !NewTushare("real-token").Available(ctx) {
		t.Fatal("real token must be available")
	}
}

func TestTushareEmptyDataIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, tushareOKEnvelope(0, "", []string{"ts_code"}, [][]any{}))
	}))
	defer srv.Close()

	src := NewTushare("tok", WithHTTPBaseURL(srv.URL))
	sym, _ := ParseSymbol("600519.SH")
	if _, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-02")); err == nil {
		t.Fatal("expected error for empty data")
	}
}

func TestTushareRejectsNonAShare(t *testing.T) {
	src := NewTushare("tok")
	sym, _ := ParseSymbol("00700.HK")
	if _, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-02")); err == nil {
		t.Fatal("expected error for non-A-share symbol")
	}
}

func TestTushareName(t *testing.T) {
	if NewTushare("tok").Name() != "tushare" {
		t.Fatal("name should be tushare")
	}
}
