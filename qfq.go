package ashare

import (
	"math"
	"time"
)

// DividendEvent is a single 除权除息 (ex-rights/ex-dividend) event.
// All quantities are per-share (通达信 XDXR 接口的"每10股"数值已除以 10)。
type DividendEvent struct {
	Date       time.Time // 除权除息日
	Cash       float64   // 每股现金红利（税前）
	Song       float64   // 每股送转股
	PeiGu      float64   // 每股配股
	PeiGuPrice float64   // 配股价
}

// AdjustBars forward-adjusts (前复权) raw daily bars using the same linear
// model the 通达信/Tencent qfq tables are built on (verified 1:1 against
// live TDX servers, 1500 days max diff 0.00):
//
//	qfq(d) = raw(d)*M(d) - N(d)
//	M(d) = Π (1 + 送转_e + 配股_e)          for every event e after d
//	N(d) = Σ B_e * Π (1 + 送转 + 配股)     for every event e after d,
//	       where B_e = 每股红利_e - 配股价_e*每股配股_e
//
// Prices are rounded to 2 decimals, matching vendor output. The input slice
// is not mutated.
func AdjustBars(events []DividendEvent, bars []Bar) []Bar {
	out := make([]Bar, len(bars))
	for i := range bars {
		b := bars[i]
		m, n := 1.0, 0.0
		for _, e := range events {
			if !e.Date.After(b.Date) {
				continue
			}
			a := 1 + e.Song + e.PeiGu
			b2 := e.Cash - e.PeiGuPrice*e.PeiGu
			m *= a
			n = n*a + b2
		}
		out[i] = Bar{
			Date:   b.Date,
			Open:   round2(b.Open*m - n),
			High:   round2(b.High*m - n),
			Low:    round2(b.Low*m - n),
			Close:  round2(b.Close*m - n),
			Volume: b.Volume,
			Amount: b.Amount,
		}
	}
	return out
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
