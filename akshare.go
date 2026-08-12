package ashare

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Akshare replicates the exact HTTP call behind AKShare's stock_zh_a_hist for
// A-share daily bars (adjust="qfq"): the same Eastmoney push2his endpoint,
// the same ut token and 11-column fields2 set. Like AKShare it is free, no
// auth, and shares the Eastmoney throttle bucket.
type Akshare struct {
	baseURL     string
	timeout     time.Duration
	minInterval time.Duration
	client      *http.Client
	throttle    *HostThrottle
}

const akshareDefaultBaseURL = "https://push2his.eastmoney.com/api/qt/stock/kline/get"

// NewAkshare returns an AKShare-compatible Eastmoney source.
func NewAkshare(opts ...Option) *Akshare {
	o := defaultHTTPOptions(akshareDefaultBaseURL)
	o.minInterval = time.Second
	for _, opt := range opts {
		opt(&o)
	}
	return &Akshare{
		baseURL:     o.baseURL,
		timeout:     o.timeout,
		minInterval: o.minInterval,
		client:      &http.Client{Timeout: o.timeout},
		throttle:    eastmoneyThrottle,
	}
}

func (a *Akshare) Name() string { return "akshare" }

func (a *Akshare) Available(ctx context.Context) bool { return true }

// Daily fetches daily bars for an A-share symbol.
func (a *Akshare) Daily(ctx context.Context, sym Symbol, start, end time.Time) ([]Bar, error) {
	var secid string
	switch sym.Exchange {
	case ExchangeSH:
		secid = "1." + sym.Code
	case ExchangeSZ, ExchangeBJ:
		secid = "0." + sym.Code
	default:
		return nil, fmt.Errorf("akshare: unsupported exchange %s", sym.Exchange)
	}

	if err := a.throttle.Wait(ctx, "eastmoney", a.minInterval); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL, nil)
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	q.Set("secid", secid)
	q.Set("ut", "fa5fd1943c7b386f172d6893dbfba10b")
	q.Set("fields1", "f1,f2,f3,f4,f5,f6")
	q.Set("fields2", "f51,f52,f53,f54,f55,f56,f57,f58,f59,f60,f61")
	q.Set("klt", "101")
	q.Set("fqt", "0") // 不复权（前复权由 Client 统一处理）
	q.Set("beg", start.Format("20060102"))
	q.Set("end", end.Format("20060102"))
	q.Set("lmt", "1000000")
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,ko;q=0.8,ja;q=0.7,en;q=0.6")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Referer", "https://quote.eastmoney.com/center/gridlist.html")
	req.Header.Set("Sec-Fetch-Dest", "script")
	req.Header.Set("Sec-Fetch-Mode", "no-cors")
	req.Header.Set("Sec-Fetch-Site", "same-site")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36")
	req.Header.Set("sec-ch-ua", `"Chromium";v="146", "Not-A.Brand";v="24", "Google Chrome";v="146"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"Linux"`)
	req.Header.Set("Cookie", "qgqp_b_id=0de8b28afaaf4a01f21ca7a977a26af1; st_nvi=xWzU6lZzYN6coXyBQBllx2214; nid18=058585a5f22c0777e008d8d7462b09a8; nid18_create_time=1782024685563; gviem=ts_8p7jnDO-khYfcgBRLYfa43; gviem_create_time=1782024685563; wsc_checkuser_ok=1")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("akshare: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("akshare: HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("akshare: read body: %w", err)
	}

	var payload struct {
		Data *struct {
			Klines []string `json:"klines"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("akshare: decode body: %w", err)
	}
	if payload.Data == nil || len(payload.Data.Klines) == 0 {
		return nil, fmt.Errorf("akshare: empty payload for %s", sym)
	}

	bars := make([]Bar, 0, len(payload.Data.Klines))
	for _, rawRow := range payload.Data.Klines {
		// Column order: 日期,开盘,收盘,最高,最低,成交量,成交额,振幅,涨跌幅,涨跌额,换手率.
		parts := strings.Split(rawRow, ",")
		if len(parts) < 11 {
			continue
		}
		date, err := time.Parse("2006-01-02", parts[0])
		if err != nil {
			continue
		}
		open, err1 := strconv.ParseFloat(parts[1], 64)
		close_, err2 := strconv.ParseFloat(parts[2], 64)
		high, err3 := strconv.ParseFloat(parts[3], 64)
		low, err4 := strconv.ParseFloat(parts[4], 64)
		volume, err5 := strconv.ParseFloat(parts[5], 64)
		amount, err6 := strconv.ParseFloat(parts[6], 64)
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil || err6 != nil {
			continue
		}
		bars = append(bars, Bar{
			Date: date, Open: open, Close: close_, High: high, Low: low,
			Volume: volume, Amount: amount,
		})
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("akshare: no parseable bars for %s", sym)
	}
	return bars, nil
}
