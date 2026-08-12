package ashare

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"time"
)

// Tencent is the Tencent Finance daily-bar source (free, no auth). It uses
// the web.ifzq.gtimg.cn fqkline endpoint, which is not subject to the
// Eastmoney CDN blocks. A-share (SH/SZ) only.
type Tencent struct {
	baseURL string
	timeout time.Duration
	client  *http.Client
}

const tencentDefaultBaseURL = "https://web.ifzq.gtimg.cn/appstock/app/fqkline/get"

// NewTencent returns a Tencent source. Options override the endpoint and
// timeout for tests and proxies.
func NewTencent(opts ...Option) *Tencent {
	o := defaultHTTPOptions(tencentDefaultBaseURL)
	for _, opt := range opts {
		opt(&o)
	}
	return &Tencent{
		baseURL: o.baseURL,
		timeout: o.timeout,
		client:  &http.Client{Timeout: o.timeout},
	}
}

func (t *Tencent) Name() string { return "tencent" }

func (t *Tencent) Available(ctx context.Context) bool { return true }

// Daily fetches daily bars. Only .SH / .SZ symbols are supported.
func (t *Tencent) Daily(ctx context.Context, sym Symbol, start, end time.Time) ([]Bar, error) {
	// The endpoint silently truncates at 500 rows per request, so long
	// windows are split into ~6-month segments (≈125 trading days each).
	var out []Bar
	for cur := start; !cur.After(end); cur = cur.AddDate(0, 6, 0) {
		segEnd := cur.AddDate(0, 6, -1)
		if segEnd.After(end) {
			segEnd = end
		}
		bars, err := t.dailyOnce(ctx, sym, cur, segEnd)
		if err != nil {
			return nil, err
		}
		out = append(out, bars...)
	}
	return normalizeBars(out, start, end), nil
}

func (t *Tencent) dailyOnce(ctx context.Context, sym Symbol, start, end time.Time) ([]Bar, error) {
	var prefix string
	switch sym.Exchange {
	case ExchangeSH:
		prefix = "sh"
	case ExchangeSZ:
		prefix = "sz"
	default:
		return nil, fmt.Errorf("tencent: unsupported exchange %s for %s", sym.Exchange, sym)
	}
	tencentCode := prefix + sym.Code
	// No "qfq" suffix: request unadjusted bars (day). Forward adjustment is
	// applied centrally by Client using TDX XDXR dividend events.
	param := fmt.Sprintf("%s,day,%s,%s,500,", tencentCode, start.Format("2006-01-02"), end.Format("2006-01-02"))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.baseURL, nil)
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	q.Set("param", param)
	req.URL.RawQuery = q.Encode()
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://web.ifzq.gtimg.cn/")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tencent: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tencent: HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("tencent: read body: %w", err)
	}

	var payload struct {
		Code int `json:"code"`
		Data map[string]struct {
			Day [][]json.RawMessage `json:"day"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("tencent: decode body: %w", err)
	}
	if len(payload.Data) == 0 {
		return nil, fmt.Errorf("tencent: empty payload for %s", sym)
	}

	var rows [][]json.RawMessage
	for _, stock := range payload.Data {
		rows = stock.Day
		break
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("tencent: no klines for %s", sym)
	}

	bars := make([]Bar, 0, len(rows))
	for _, k := range rows {
		if len(k) < 6 {
			continue
		}
		var vals [6]string
		ok := true
		for i := 0; i < 6; i++ {
			if err := json.Unmarshal(k[i], &vals[i]); err != nil {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		date, err := time.Parse("2006-01-02", vals[0])
		if err != nil {
			continue
		}
		open, err1 := strconv.ParseFloat(vals[1], 64)
		close_, err2 := strconv.ParseFloat(vals[2], 64)
		high, err3 := strconv.ParseFloat(vals[3], 64)
		low, err4 := strconv.ParseFloat(vals[4], 64)
		volume, err5 := strconv.ParseFloat(vals[5], 64)
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil {
			continue
		}
		bars = append(bars, Bar{
			Date: date, Open: open, Close: close_, High: high, Low: low, Volume: volume,
		})
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("tencent: no parseable bars for %s", sym)
	}
	return bars, nil
}
