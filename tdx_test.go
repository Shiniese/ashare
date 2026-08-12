package ashare

import (
	"context"
	"testing"
	"time"

	"github.com/bensema/gotdx/proto"
	"github.com/bensema/gotdx/types"
)

type fakeTdxClient struct {
	// barsByStart: offset-from-latest -> bars returned for that page.
	barsByStart map[uint16][]proto.SecurityBar
	xdxr        *proto.GetXDXRInfoReply
	lastCalls   []tdxCall
}

type tdxCall struct {
	category uint16
	market   uint8
	code     string
	start    uint16
	count    uint16
	times    uint16
	adjust   uint16
}

func (f *fakeTdxClient) Connect() (*proto.Hello1Reply, error) {
	return nil, nil
}

func (f *fakeTdxClient) StockKLine(category uint16, market uint8, code string, start, count, times, adjust uint16) ([]proto.SecurityBar, error) {
	f.lastCalls = append(f.lastCalls, tdxCall{category, market, code, start, count, times, adjust})
	return f.barsByStart[start], nil
}

func (f *fakeTdxClient) GetXDXRInfo(market uint8, code string) (*proto.GetXDXRInfoReply, error) {
	return f.xdxr, nil
}

func tdxBar(date string, close_ float64) proto.SecurityBar {
	tm, _ := time.Parse("2006-01-02", date)
	return proto.SecurityBar{DateTime: tm, Open: close_ - 0.1, Close: close_, High: close_ + 0.1, Low: close_ - 0.2, Vol: 1000, Amount: 1e6}
}

func newFakeTdx(barsByStart map[uint16][]proto.SecurityBar) *Tdx {
	fake := &fakeTdxClient{barsByStart: barsByStart}
	return &Tdx{newClient: func() (tdxClient, error) { return fake, nil }}
}

func TestTdxDailyParsesBars(t *testing.T) {
	src := newFakeTdx(map[uint16][]proto.SecurityBar{
		0: {tdxBar("2026-06-01", 21.35), tdxBar("2026-06-02", 21.5)},
	})
	sym, _ := ParseSymbol("600519.SH")
	bars, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-02"))
	if err != nil {
		t.Fatalf("Daily error: %v", err)
	}
	if len(bars) != 2 {
		t.Fatalf("expected 2 bars, got %d", len(bars))
	}
	// TDX pages return oldest-first; the source clips to [start, end].
	b0 := bars[0]
	if !b0.Date.Equal(d("2026-06-01")) || b0.Close != 21.35 || b0.Open != 21.25 ||
		!approx(b0.High, 21.45) || !approx(b0.Low, 21.15) || b0.Volume != 1000 || b0.Amount != 1e6 {
		t.Fatalf("unexpected first bar: %+v", b0)
	}
	if !bars[1].Date.Equal(d("2026-06-02")) || bars[1].Close != 21.5 {
		t.Fatalf("unexpected second bar: %+v", bars[1])
	}
}

func TestTdxUsesUnadjustedDailyKLineCategory(t *testing.T) {
	src := newFakeTdx(map[uint16][]proto.SecurityBar{
		0: {tdxBar("2026-06-01", 21.35)},
	})
	sym, _ := ParseSymbol("600519.SH")
	_, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-02"))
	if err != nil {
		t.Fatalf("Daily error: %v", err)
	}
	fakeIface, _ := src.newClient()
	fakeClient, _ := fakeIface.(*fakeTdxClient)
	if len(fakeClient.lastCalls) == 0 {
		t.Fatal("no call recorded")
	}
	call := fakeClient.lastCalls[0]
	if call.category != types.KLINE_TYPE_DAILY {
		t.Fatalf("category = %d, want KLINE_TYPE_DAILY (4, 不复权日K)", call.category)
	}
	if call.adjust != 0 {
		t.Fatalf("adjust = %d, want 0 (不复权)", call.adjust)
	}
	if call.count != tdxBarsPage {
		t.Fatalf("count = %d, want %d (gotdx panics on larger pages)", call.count, tdxBarsPage)
	}
	if call.market != 1 {
		t.Fatalf("market = %d, want 1 (SH)", call.market)
	}
	if call.code != "600519" {
		t.Fatalf("code = %s, want 600519", call.code)
	}
}

func TestTdxMapsShenzhenMarket(t *testing.T) {
	src := newFakeTdx(map[uint16][]proto.SecurityBar{
		0: {tdxBar("2026-06-01", 10.0)},
	})
	sym, _ := ParseSymbol("000001.SZ")
	_, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-02"))
	if err != nil {
		t.Fatalf("Daily error: %v", err)
	}
	fakeIface, _ := src.newClient()
	fake, _ := fakeIface.(*fakeTdxClient)
	if fake.lastCalls[0].market != 0 {
		t.Fatalf("market = %d, want 0 (SZ)", fake.lastCalls[0].market)
	}
}

func TestTdxRejectsBeijing(t *testing.T) {
	src := newFakeTdx(nil)
	sym, _ := ParseSymbol("430139.BJ")
	if _, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-02")); err == nil {
		t.Fatal("expected error for BJ symbol (mootdx std does not serve BJ)")
	}
}

func TestTdxPagesBackUntilReachingStartDate(t *testing.T) {
	src := newFakeTdx(map[uint16][]proto.SecurityBar{
		0:   {tdxBar("2026-05-21", 20.1), tdxBar("2026-05-22", 20.2)},
		500: {tdxBar("2026-05-19", 19.9), tdxBar("2026-05-20", 20.0)},
	})
	sym, _ := ParseSymbol("600519.SH")
	bars, err := src.Daily(context.Background(), sym, d("2026-05-20"), d("2026-05-22"))
	if err != nil {
		t.Fatalf("Daily error: %v", err)
	}
	// Page 0 (newest 500, oldest-first) + page 1 (older), clipped to
	// [2026-05-20, 2026-05-22]. Pages must be merged back-to-front.
	if len(bars) != 3 {
		t.Fatalf("expected 3 bars (2 from page 0, 1 in-range from page 1), got %d", len(bars))
	}
	if !bars[0].Date.Equal(d("2026-05-20")) || !bars[1].Date.Equal(d("2026-05-21")) || !bars[2].Date.Equal(d("2026-05-22")) {
		t.Fatalf("unexpected bar dates (must be ascending): %v", bars)
	}
}

func TestTdxIncompleteHistoryIsError(t *testing.T) {
	src := newFakeTdx(map[uint16][]proto.SecurityBar{
		0: {tdxBar("2026-06-02", 21.5)},
	})
	sym, _ := ParseSymbol("600519.SH")
	// start=2020-01-01 is far older than the single 2026-06-02 bar; paging
	// 25 times must fail.
	if _, err := src.Daily(context.Background(), sym, d("2020-01-01"), d("2026-06-02")); err == nil {
		t.Fatal("expected error when history cannot reach start date")
	}
}

func TestTdxEmptyResultIsError(t *testing.T) {
	src := newFakeTdx(map[uint16][]proto.SecurityBar{})
	sym, _ := ParseSymbol("600519.SH")
	if _, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-02")); err == nil {
		t.Fatal("expected error for empty result")
	}
}

func TestTdxAvailableAndName(t *testing.T) {
	src := newFakeTdx(nil)
	if !src.Available(context.Background()) {
		t.Fatal("tdx should always be available")
	}
	if src.Name() != "mootdx" {
		t.Fatalf("name = %s, want mootdx", src.Name())
	}
}

func TestTdxDividendEvents(t *testing.T) {
	f32 := func(v float32) *float32 { return &v }
	xdxr := &proto.GetXDXRInfoReply{
		Count: 3,
		List: []proto.XDXRItem{
			{Date: d("2026-06-26"), Category: 1, Fenhong: f32(280.2423)},                   // 10派280.2423
			{Date: d("2025-09-01"), Category: 5, Fenhong: nil},                             // 股本变化，忽略
			{Date: d("2015-07-17"), Category: 1, Fenhong: f32(43.74), Songzhuangu: f32(1)}, // 10送1派43.74
			{Date: d("2002-07-25"), Category: 2, Fenhong: f32(6), Songzhuangu: f32(1)},     // 送配股上市，忽略
		},
	}
	src := newFakeTdx(map[uint16][]proto.SecurityBar{})
	fakeIface, _ := src.newClient()
	fakeClient, _ := fakeIface.(*fakeTdxClient)
	fakeClient.xdxr = xdxr

	sym, _ := ParseSymbol("600519.SH")
	evs, err := src.DividendEvents(context.Background(), sym)
	if err != nil {
		t.Fatalf("DividendEvents error: %v", err)
	}
	if len(evs) != 2 {
		t.Fatalf("expected 2 dividend events, got %d", len(evs))
	}
	if !evs[0].Date.Equal(d("2026-06-26")) || !approx(evs[0].Cash, 28.02423) { // 280.2423/10 (float32)
		t.Fatalf("unexpected newest event: %+v", evs[0])
	}
	if !evs[1].Date.Equal(d("2015-07-17")) || !approx(evs[1].Cash, 4.374) || !approx(evs[1].Song, 0.1) {
		t.Fatalf("unexpected oldest event: %+v", evs[1])
	}
	if evs[1].PeiGu != 0 || evs[1].PeiGuPrice != 0 {
		t.Fatalf("peigu fields should default to 0: %+v", evs[1])
	}
}

func TestTdxDividendEventsEmptyList(t *testing.T) {
	src := newFakeTdx(nil)
	fakeIface, _ := src.newClient()
	fakeClient, _ := fakeIface.(*fakeTdxClient)
	fakeClient.xdxr = &proto.GetXDXRInfoReply{Count: 0, List: nil}
	sym, _ := ParseSymbol("600519.SH")
	evs, err := src.DividendEvents(context.Background(), sym)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(evs) != 0 {
		t.Fatalf("expected no events, got %d", len(evs))
	}
}
