package ashare

import (
	"testing"
	"time"
)

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("bad date %q: %v", s, err)
	}
	return d
}

// dividendEvents600519 are the real cat=1 XDXR events for 600519 that fall
// after 2024-07-18 (per-10-shares values already converted to per-share),
// as observed from live 通达信 servers via gotdx GetXDXRInfo.
var dividendEvents600519 = []DividendEvent{
	{Date: mustDateT("2026-06-26"), Cash: 28.0242},
	{Date: mustDateT("2025-12-19"), Cash: 23.957},
	{Date: mustDateT("2025-06-26"), Cash: 27.673},
	{Date: mustDateT("2024-12-20"), Cash: 23.882},
}

func mustDateT(s string) time.Time {
	d, _ := time.Parse("2006-01-02", s)
	return d
}

func TestAdjustBarsPureCashMatchesLiveTDX(t *testing.T) {
	// raw close prices and the matching mootdx/TDX qfq close prices,
	// verified 1:1 against live 通达信 servers (RI_K adjust=1).
	cases := []struct {
		date string
		raw  float64
		want float64
	}{
		{"2024-09-18", 1266.90, 1163.36},
		{"2024-12-20", 1522.00, 1442.35}, // 除权日当天: 不含当日事件
		{"2025-06-26", 1420.00, 1368.02}, // 除权日当天
		{"2025-07-30", 1449.44, 1397.46},
		{"2026-05-20", 1315.00, 1286.98},
		{"2026-08-04", 1328.36, 1328.36}, // 最新日: 无未来事件
		{"2026-08-10", 1348.86, 1348.86},
	}
	for _, tc := range cases {
		got := AdjustBars(dividendEvents600519, []Bar{{
			Date: mustDateT(tc.date), Open: tc.raw, High: tc.raw, Low: tc.raw, Close: tc.raw,
		}})
		if got[0].Close != tc.want {
			t.Errorf("%s: AdjustBars close = %.2f, want %.2f", tc.date, got[0].Close, tc.want)
		}
		if got[0].Open != tc.want || got[0].High != tc.want || got[0].Low != tc.want {
			t.Errorf("%s: OHLC not all adjusted to %.2f: got O=%.2f H=%.2f L=%.2f", tc.date, tc.want, got[0].Open, got[0].High, got[0].Low)
		}
	}
}

func TestAdjustBarsWithSongAndPeiGu(t *testing.T) {
	// 10送1派43.74 -> 每股送 0.1、红利 4.374; A=1.1, B=4.374
	song := []DividendEvent{{Date: mustDateT("2015-07-17"), Cash: 4.374, Song: 0.1}}
	got := AdjustBars(song, []Bar{{Date: mustDateT("2015-07-16"), Close: 100}})
	if got[0].Close != 105.63 { // 100*1.1 - 4.374 = 105.626
		t.Errorf("song: close = %.2f, want 105.63", got[0].Close)
	}
	after := AdjustBars(song, []Bar{{Date: mustDateT("2015-07-17"), Close: 100}})
	if after[0].Close != 100 {
		t.Errorf("song: bar on ex-date = %.2f, want 100 (event not applied)", after[0].Close)
	}

	// 每10股配3股、配股价5元、每股红利2 -> A=1.3, B=2-5*0.3=0.5
	pei := []DividendEvent{{Date: mustDateT("2016-01-01"), Cash: 2, PeiGu: 0.3, PeiGuPrice: 5}}
	got = AdjustBars(pei, []Bar{{Date: mustDateT("2015-12-31"), Close: 10}})
	if got[0].Close != 12.5 { // 10*1.3 - 0.5
		t.Errorf("peigu: close = %.2f, want 12.50", got[0].Close)
	}
}

func TestAdjustBarsEventOrderIndependent(t *testing.T) {
	shuffled := []DividendEvent{
		dividendEvents600519[2], dividendEvents600519[0],
		dividendEvents600519[3], dividendEvents600519[1],
	}
	a := AdjustBars(dividendEvents600519, []Bar{{Date: mustDateT("2024-09-18"), Close: 1266.90}})
	b := AdjustBars(shuffled, []Bar{{Date: mustDateT("2024-09-18"), Close: 1266.90}})
	if a[0].Close != b[0].Close {
		t.Errorf("order dependent: %v vs %v", a[0].Close, b[0].Close)
	}
}

func TestAdjustBarsDoesNotMutateInput(t *testing.T) {
	bar := Bar{Date: mustDateT("2024-09-18"), Open: 1, High: 2, Low: 3, Close: 1266.90}
	in := []Bar{bar}
	AdjustBars(dividendEvents600519, in)
	if in[0].Close != 1266.90 {
		t.Errorf("input mutated: %v", in[0])
	}
}
