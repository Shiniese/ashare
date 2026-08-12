package ashare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Tushare is the Tushare Pro daily-bar source (key-gated HTTP). It posts to
// api.tushare.pro exactly like the Python tushare package, waits out per-minute
// quota rejections with backoff. Prices are unadjusted (不复权); forward
// adjustment is applied centrally by Client using TDX XDXR events.
type Tushare struct {
	token   string
	baseURL string
	timeout time.Duration
	backoff []time.Duration
	client  *http.Client
}

const tushareDefaultBaseURL = "https://api.tushare.pro"

var tushareTokenPlaceholders = map[string]bool{"": true, "your-tushare-token": true}

var rateLimitMarkers = []string{
	"每分钟", "每天", "抽取", "访问该接口", "频率", "rate limit", "too many requests",
}

// NewTushare returns a Tushare source using the given token. Pass the empty
// string when the token should come from the TUSHARE_TOKEN environment
// variable (resolved lazily by Available/Daily).
func NewTushare(token string, opts ...Option) *Tushare {
	o := defaultHTTPOptions(tushareDefaultBaseURL)
	for _, opt := range opts {
		opt(&o)
	}
	return &Tushare{
		token:   token,
		baseURL: o.baseURL,
		timeout: o.timeout,
		backoff: o.backoff,
		client:  &http.Client{Timeout: o.timeout},
	}
}

func (t *Tushare) Name() string { return "tushare" }

func (t *Tushare) Available(ctx context.Context) bool { return !tushareTokenPlaceholders[t.token] }

// Daily fetches unadjusted (不复权) daily bars. ETF symbols route to
// fund_daily, index symbols to index_daily (no adjustment anyway),
// everything else to daily. Forward adjustment is applied centrally.
func (t *Tushare) Daily(ctx context.Context, sym Symbol, start, end time.Time) ([]Bar, error) {
	params := map[string]string{
		"ts_code":    sym.String(),
		"start_date": start.Format("20060102"),
		"end_date":   end.Format("20060102"),
	}

	switch {
	case isETF(sym):
		resp, err := t.call(ctx, "fund_daily", params)
		if err != nil {
			return nil, fmt.Errorf("tushare: %w", err)
		}
		return parseTushareBars(resp, sym)
	case isIndex(sym):
		resp, err := t.call(ctx, "index_daily", params)
		if err != nil {
			return nil, fmt.Errorf("tushare: %w", err)
		}
		return parseTushareBars(resp, sym)
	default:
		resp, err := t.call(ctx, "daily", params)
		if err != nil {
			return nil, fmt.Errorf("tushare: %w", err)
		}
		return parseTushareBars(resp, sym)
	}
}

// isETF mirrors _symbol_utils._is_etf_listed: SH 50/51/52/56/58, SZ 15/16.
func isETF(sym Symbol) bool {
	if sym.Exchange != ExchangeSH && sym.Exchange != ExchangeSZ {
		return false
	}
	prefix := sym.Code[:2]
	switch prefix {
	case "15", "16", "50", "51", "52", "56", "58":
		return true
	}
	return false
}

// isIndex mirrors tushare._is_index: 000xxx.SH and 399xxx.SZ.
func isIndex(sym Symbol) bool {
	switch sym.Exchange {
	case ExchangeSH:
		return strings.HasPrefix(sym.Code, "000")
	case ExchangeSZ:
		return strings.HasPrefix(sym.Code, "399")
	}
	return false
}

type tushareEnvelope struct {
	Code int             `json:"code"`
	Msg  json.RawMessage `json:"msg"`
	Data *struct {
		Fields []string `json:"fields"`
		Items  [][]any  `json:"items"`
	} `json:"data"`
}

func (t *Tushare) call(ctx context.Context, apiName string, params map[string]string) (*tushareEnvelope, error) {
	body, err := json.Marshal(map[string]any{
		"api_name": apiName,
		"token":    t.token,
		"params":   params,
		"fields":   "",
	})
	if err != nil {
		return nil, err
	}

	for i := 0; ; i++ {
		env, err := t.callOnce(ctx, body)
		if err == nil {
			return env, nil
		}
		if !isRateLimited(err) || i >= len(t.backoff) {
			return nil, err
		}
		delay := t.backoff[i]
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (t *Tushare) callOnce(ctx context.Context, body []byte) (*tushareEnvelope, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var env tushareEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("decode body: %w", err)
	}
	if env.Code != 0 {
		return nil, fmt.Errorf("%s", msgText(env.Msg))
	}
	return &env, nil
}

func msgText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}

func isRateLimited(err error) bool {
	lower := strings.ToLower(err.Error())
	for _, marker := range rateLimitMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func parseTushareBars(env *tushareEnvelope, sym Symbol) ([]Bar, error) {
	if env.Data == nil || len(env.Data.Items) == 0 {
		return nil, fmt.Errorf("tushare: empty data for %s", sym)
	}
	idx := make(map[string]int, len(env.Data.Fields))
	for i, f := range env.Data.Fields {
		idx[f] = i
	}
	bars := make([]Bar, 0, len(env.Data.Items))
	for _, item := range env.Data.Items {
		dateStr, ok := item[idx["trade_date"]].(string)
		if !ok {
			continue
		}
		date, err := time.Parse("20060102", dateStr)
		if err != nil {
			continue
		}
		bar := Bar{Date: date}
		if v, ok := asFloat(item, idx["open"]); ok {
			bar.Open = v
		}
		if v, ok := asFloat(item, idx["high"]); ok {
			bar.High = v
		}
		if v, ok := asFloat(item, idx["low"]); ok {
			bar.Low = v
		}
		if v, ok := asFloat(item, idx["close"]); ok {
			bar.Close = v
		}
		if v, ok := asFloat(item, idx["vol"]); ok {
			bar.Volume = v
		}
		if v, ok := asFloat(item, idx["amount"]); ok {
			bar.Amount = v
		}
		bars = append(bars, bar)
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("tushare: no parseable bars for %s", sym)
	}
	sort.SliceStable(bars, func(i, j int) bool { return bars[i].Date.Before(bars[j].Date) })
	return bars, nil
}

func asFloat(item []any, i int) (float64, bool) {
	if i < 0 || i >= len(item) {
		return 0, false
	}
	switch v := item[i].(type) {
	case float64:
		return v, true
	case string:
		f, err := parseFloatOrZero(v)
		return f, err == nil
	}
	return 0, false
}

func parseFloatOrZero(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	return strconv.ParseFloat(s, 64)
}

func applyQFQ(bars []Bar, factors map[time.Time]float64) ([]Bar, error) {
	if len(factors) == 0 || len(bars) == 0 {
		return nil, fmt.Errorf("tushare: no usable adjustment factors")
	}
	// Sort factor dates for binary search (ffill/bfill).
	factorDates := make([]time.Time, 0, len(factors))
	for date, f := range factors {
		if f <= 0 {
			return nil, fmt.Errorf("tushare: non-positive adjustment factor")
		}
		factorDates = append(factorDates, date)
	}
	sort.Slice(factorDates, func(i, j int) bool { return factorDates[i].Before(factorDates[j]) })

	for i := range bars {
		factor, ok := factorFor(factorDates, factors, bars[i].Date)
		if !ok {
			return nil, fmt.Errorf("tushare: no adjustment factor for %s", bars[i].Date.Format("2006-01-02"))
		}
		bars[i].factor = factor
	}
	last := bars[len(bars)-1].factor
	for i := range bars {
		ratio := bars[i].factor / last
		bars[i].Open *= ratio
		bars[i].High *= ratio
		bars[i].Low *= ratio
		bars[i].Close *= ratio
		bars[i].Volume /= ratio
	}
	return bars, nil
}

// factorFor returns the most recent factor on or before date (ffill), falling
// back to the earliest factor on or after date (bfill).
func factorFor(dates []time.Time, factors map[time.Time]float64, date time.Time) (float64, bool) {
	i := sort.Search(len(dates), func(i int) bool { return !dates[i].Before(date) })
	if i < len(dates) && dates[i].Equal(date) {
		return factors[dates[i]], true
	}
	if i > 0 {
		return factors[dates[i-1]], true // ffill
	}
	if i < len(dates) {
		return factors[dates[i]], true // bfill
	}
	return 0, false
}
