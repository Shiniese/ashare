package ashare

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/millken/baostock"
)

// baostockClient is the subset of the millken/baostock Client used here,
// isolated so tests can substitute a fake. millken/baostock connects and
// logs in lazily on first query.
type baostockClient interface {
	Login(ctx context.Context) error
	Logout(ctx context.Context) error
	QueryHistoryKDataPlus(ctx context.Context, req *baostock.HistoryKDataRequest, callback func(fields []string, record []string) error) error
}

// Baostock is the baostock source: A-share daily bars via the baostock TCP
// protocol (millken/baostock), forward-adjusted (qfq). Not subject to HTTP
// rate limits or IP bans.
type Baostock struct {
	newClient func() (baostockClient, error)
}

// baostockFields mirrors the Python loader's field selection; baostock rows
// are returned in exactly this order.
var baostockFields = []string{"date", "open", "high", "low", "close", "volume", "amount"}

// NewBaostock returns a baostock source using default login (anonymous).
func NewBaostock() *Baostock {
	return &Baostock{
		newClient: func() (baostockClient, error) {
			return baostock.NewClient(), nil
		},
	}
}

func (b *Baostock) Name() string { return "baostock" }

func (b *Baostock) Available(ctx context.Context) bool { return true }

// Daily fetches unadjusted (不复权) daily bars. The baostock server filters by
// date range itself, so rows are already in [start, end].
func (b *Baostock) Daily(ctx context.Context, sym Symbol, start, end time.Time) ([]Bar, error) {
	if sym.Exchange != ExchangeSH && sym.Exchange != ExchangeSZ {
		return nil, fmt.Errorf("baostock: only SH/SZ supported upstream, got %s", sym)
	}
	bsCode := fmt.Sprintf("%s.%s", strings.ToLower(string(sym.Exchange)), sym.Code)

	client, err := b.newClient()
	if err != nil {
		return nil, fmt.Errorf("baostock: %w", err)
	}
	if err := client.Login(ctx); err != nil {
		return nil, fmt.Errorf("baostock: login: %w", err)
	}
	defer client.Logout(ctx)

	req := &baostock.HistoryKDataRequest{
		Code:       bsCode,
		Fields:     strings.Join(baostockFields, ","),
		StartDate:  start.Format("2006-01-02"),
		EndDate:    end.Format("2006-01-02"),
		Frequency:  baostock.FrequencyDaily,
		AdjustFlag: baostock.AdjustFlagNoAdjust, // 不复权（前复权由 Client 统一处理）
	}
	var out []Bar
	err = client.QueryHistoryKDataPlus(ctx, req, func(fields []string, record []string) error {
		if len(record) < 7 {
			return nil
		}
		date, err := time.Parse("2006-01-02", record[0])
		if err != nil {
			return fmt.Errorf("baostock: bad date %q: %w", record[0], err)
		}
		out = append(out, Bar{
			Date:   date,
			Open:   mustFloat(record[1], "open"),
			High:   mustFloat(record[2], "high"),
			Low:    mustFloat(record[3], "low"),
			Close:  mustFloat(record[4], "close"),
			Volume: mustFloat(record[5], "volume"),
			Amount: mustFloat(record[6], "amount"),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("baostock: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("baostock: no bars for %s in %s..%s", sym, req.StartDate, req.EndDate)
	}
	return out, nil
}

// mustFloat parses s as float64, panicking via panicFloat on malformed input
// so the query is aborted loudly rather than silently dropping data.
func mustFloat(s, field string) float64 {
	v, err := parseFloat(s)
	if err != nil {
		panicFloat(field, s)
	}
	return v
}

func parseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscan(s, &f)
	return f, err
}

func panicFloat(field, s string) {
	panic(fmt.Sprintf("baostock: invalid %s value %q", field, s))
}
