package ashare

import "os"

// DefaultChain returns the default A-share fallback chain, ordered by
// IP-ban risk (least risky first, mirroring the Python registry):
// tencent -> mootdx -> eastmoney -> baostock -> akshare -> tushare.
//
// The tushare source is only usable when TUSHARE_TOKEN is set; without a
// token it reports unavailable and is skipped.
func DefaultChain() []Source {
	return []Source{
		NewTencent(),
		NewTdx(),
		NewEastmoney(),
		NewBaostock(),
		NewAkshare(),
		NewTushare(os.Getenv("TUSHARE_TOKEN")),
	}
}
