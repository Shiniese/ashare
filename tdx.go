package ashare

import (
	"context"
	"fmt"
	"time"

	"github.com/bensema/gotdx"
	"github.com/bensema/gotdx/proto"
	"github.com/bensema/gotdx/types"
)

// tdxClient is the subset of the gotdx Client used by the mootdx source,
// isolated so tests can substitute a fake.
type tdxClient interface {
	Connect() (*proto.Hello1Reply, error)
	StockKLine(category uint16, market uint8, code string, start, count, times, adjust uint16) ([]proto.SecurityBar, error)
	GetXDXRInfo(market uint8, code string) (*proto.GetXDXRInfoReply, error)
}

// Tdx is the mootdx source: A-share daily bars via the 通达信 binary TCP
// protocol through gotdx, with automatic fastest-host selection. Not subject
// to HTTP rate limits or IP bans.
type Tdx struct {
	newClient func() (tdxClient, error)
}

// tdxBarsPage is the per-request page size. gotdx's response parser panics
// on larger payloads (verified: 800 bars corrupts the parse; 500 is safe).
const tdxBarsPage = 500

// tdxMaxPages mirrors mootdx's _MAX_PAGES: 25 x 500 = 12500 bars (~30y daily).
const tdxMaxPages = 25

// NewTdx returns a mootdx source backed by gotdx.
func NewTdx() *Tdx {
	return &Tdx{
		newClient: func() (tdxClient, error) {
			client := gotdx.New(gotdx.WithAutoSelectFastest(true))
			return client, nil
		},
	}
}

func (t *Tdx) Name() string { return "mootdx" }

func (t *Tdx) Available(ctx context.Context) bool { return true }

// Daily fetches unadjusted (不复权) daily bars via the TDX protocol, paging
// back through history until the requested start date is covered. Pages come
// back oldest-first (verified against live 通达信 servers). Forward
// adjustment is applied centrally by Client using DividendEvents.
func (t *Tdx) Daily(ctx context.Context, sym Symbol, start, end time.Time) ([]Bar, error) {
	if sym.Exchange == ExchangeBJ {
		return nil, fmt.Errorf("mootdx: 北交所 (%s) not supported upstream; use akshare/tushare", sym)
	}
	var market uint8
	switch sym.Exchange {
	case ExchangeSH:
		market = 1
	case ExchangeSZ:
		market = 0
	default:
		return nil, fmt.Errorf("mootdx: unsupported exchange %s", sym.Exchange)
	}

	client, err := t.newClient()
	if err != nil {
		return nil, fmt.Errorf("mootdx: %w", err)
	}

	var all []proto.SecurityBar
	covered := false
	for page := 0; page < tdxMaxPages; page++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		bars, err := client.StockKLine(
			types.KLINE_TYPE_DAILY, market, sym.Code,
			uint16(page*tdxBarsPage), tdxBarsPage, 1, 0, // adjust=0: 不复权
		)
		if err != nil {
			return nil, fmt.Errorf("mootdx: %w", err)
		}
		if len(bars) == 0 {
			break
		}
		// Pages come back oldest-first; each subsequent page holds older
		// bars, so it must be prepended to keep the merge ascending.
		all = append(bars, all...)
		if !bars[0].DateTime.After(start) {
			// The oldest row of this page predates start_date. The TDX
			// protocol returns bars oldest-first within each page.
			covered = true
			break
		}
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("mootdx: no bars for %s", sym)
	}
	if !covered {
		return nil, fmt.Errorf("mootdx: incomplete history for %s: hit %d pages without reaching %s", sym, tdxMaxPages, start.Format("2006-01-02"))
	}

	out := make([]Bar, 0, len(all))
	for _, b := range all {
		if b.DateTime.Before(start) || b.DateTime.After(end) {
			continue
		}
		out = append(out, Bar{
			Date:   b.DateTime,
			Open:   b.Open,
			High:   b.High,
			Low:    b.Low,
			Close:  b.Close,
			Volume: b.Vol,
			Amount: b.Amount,
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("mootdx: no bars in range for %s", sym)
	}
	return out, nil
}

// DividendEvents returns the 除权除息 history for a symbol from the TDX
// server (gotdx GetXDXRInfo), as per-share quantities. Only cash-dividend
// events (Category 1 with non-zero 分红) are returned, newest first. These
// feed the central forward-adjustment in Client.
func (t *Tdx) DividendEvents(ctx context.Context, sym Symbol) ([]DividendEvent, error) {
	var market uint8
	switch sym.Exchange {
	case ExchangeSH:
		market = 1
	case ExchangeSZ:
		market = 0
	default:
		return nil, fmt.Errorf("mootdx: unsupported exchange %s", sym.Exchange)
	}
	client, err := t.newClient()
	if err != nil {
		return nil, fmt.Errorf("mootdx: %w", err)
	}
	// GetXDXRInfo uses the primary connection directly (unlike StockKLine,
	// which connects lazily), so connect first.
	if _, err := client.Connect(); err != nil {
		return nil, fmt.Errorf("mootdx: connect: %w", err)
	}
	reply, err := client.GetXDXRInfo(market, sym.Code)
	if err != nil {
		return nil, fmt.Errorf("mootdx: xdxr: %w", err)
	}
	if reply == nil {
		return nil, fmt.Errorf("mootdx: xdxr: empty reply for %s", sym)
	}
	var out []DividendEvent
	for _, it := range reply.List {
		if it.Category != 1 || it.Fenhong == nil || *it.Fenhong == 0 {
			continue
		}
		out = append(out, DividendEvent{
			Date:       it.Date,
			Cash:       float64(*it.Fenhong) / 10, // XDXR 单位为"每10股"
			Song:       float64(f32orZero(it.Songzhuangu)) / 10,
			PeiGu:      float64(f32orZero(it.Peigu)) / 10,
			PeiGuPrice: float64(f32orZero(it.Peigujia)),
		})
	}
	return out, nil
}

func f32orZero(p *float32) float32 {
	if p == nil {
		return 0
	}
	return *p
}
