package ashare

import (
	"fmt"
	"strings"
)

// Exchange is a China A-share exchange.
type Exchange string

const (
	ExchangeSH Exchange = "SH"
	ExchangeSZ Exchange = "SZ"
	ExchangeBJ Exchange = "BJ"
)

// Symbol is a parsed A-share symbol: a 6-digit code plus its exchange.
type Symbol struct {
	Code     string
	Exchange Exchange
}

// String renders the canonical "600519.SH" form.
func (s Symbol) String() string {
	return s.Code + "." + string(s.Exchange)
}

// inferExchange maps a bare 6-digit code to its exchange by prefix,
// mirroring the Tushare convention: 5/6/9 -> SH, 0/2/3 -> SZ, 4/8 -> BJ.
func inferExchange(code string) (Exchange, bool) {
	switch code[0] {
	case '5', '6', '9':
		return ExchangeSH, true
	case '0', '2', '3':
		return ExchangeSZ, true
	case '4', '8':
		return ExchangeBJ, true
	}
	return "", false
}

// ParseSymbol parses any accepted A-share symbol form:
// "600519.SH", "SH600519", "sh.600519", or a bare 6-digit "600519".
func ParseSymbol(s string) (Symbol, error) {
	upper := strings.ToUpper(strings.TrimSpace(s))
	if upper == "" {
		return Symbol{}, fmt.Errorf("empty symbol")
	}

	code, exch := "", ""
	switch {
	case strings.HasPrefix(upper, "SH.") || strings.HasPrefix(upper, "SZ.") || strings.HasPrefix(upper, "BJ."):
		code, exch = upper[3:], upper[:2]
	case strings.HasPrefix(upper, "SH") || strings.HasPrefix(upper, "SZ") || strings.HasPrefix(upper, "BJ"):
		code, exch = upper[2:], upper[:2]
	case strings.Contains(upper, "."):
		parts := strings.SplitN(upper, ".", 2)
		code, exch = parts[0], parts[1]
	default:
		code = upper
	}

	if len(code) != 6 || !isDigits(code) {
		return Symbol{}, fmt.Errorf("invalid A-share symbol: %q", s)
	}
	if strings.Contains(upper, ".") && exch == "" {
		return Symbol{}, fmt.Errorf("invalid A-share symbol: %q", s)
	}

	var exchange Exchange
	switch exch {
	case "":
		var ok bool
		exchange, ok = inferExchange(code)
		if !ok {
			return Symbol{}, fmt.Errorf("cannot infer exchange for %q", s)
		}
	case "SH", "SZ", "BJ":
		exchange = Exchange(exch)
	default:
		return Symbol{}, fmt.Errorf("unsupported exchange %q in symbol %q", exch, s)
	}
	return Symbol{Code: code, Exchange: exchange}, nil
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
