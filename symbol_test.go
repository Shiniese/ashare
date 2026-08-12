package ashare

import "testing"

func TestParseSymbolSuffixForm(t *testing.T) {
	sym, err := ParseSymbol("600519.SH")
	if err != nil {
		t.Fatalf("ParseSymbol(600519.SH) error: %v", err)
	}
	if sym.Code != "600519" || sym.Exchange != ExchangeSH {
		t.Fatalf("got %+v, want {600519 SH}", sym)
	}
}

func TestParseSymbolShenzhenSuffix(t *testing.T) {
	sym, err := ParseSymbol("000001.SZ")
	if err != nil {
		t.Fatalf("ParseSymbol error: %v", err)
	}
	if sym.Exchange != ExchangeSZ {
		t.Fatalf("got exchange %s, want SZ", sym.Exchange)
	}
}

func TestParseSymbolBeijingSuffix(t *testing.T) {
	sym, err := ParseSymbol("430139.BJ")
	if err != nil {
		t.Fatalf("ParseSymbol error: %v", err)
	}
	if sym.Exchange != ExchangeBJ {
		t.Fatalf("got exchange %s, want BJ", sym.Exchange)
	}
}

func TestParseSymbolLowercaseSuffix(t *testing.T) {
	sym, err := ParseSymbol("600519.sh")
	if err != nil {
		t.Fatalf("ParseSymbol error: %v", err)
	}
	if sym.Exchange != ExchangeSH {
		t.Fatalf("got exchange %s, want SH", sym.Exchange)
	}
}

func TestParseSymbolTusharePrefixForm(t *testing.T) {
	sym, err := ParseSymbol("SH600519")
	if err != nil {
		t.Fatalf("ParseSymbol(SH600519) error: %v", err)
	}
	if sym.Code != "600519" || sym.Exchange != ExchangeSH {
		t.Fatalf("got %+v, want {600519 SH}", sym)
	}
}

func TestParseSymbolBaostockDotForm(t *testing.T) {
	sym, err := ParseSymbol("sh.600519")
	if err != nil {
		t.Fatalf("ParseSymbol(sh.600519) error: %v", err)
	}
	if sym.Code != "600519" || sym.Exchange != ExchangeSH {
		t.Fatalf("got %+v, want {600519 SH}", sym)
	}
}

func TestParseSymbolBareSixDigitsInfersExchange(t *testing.T) {
	cases := map[string]Exchange{
		"600519": ExchangeSH,
		"000001": ExchangeSZ,
		"300750": ExchangeSZ,
		"430139": ExchangeBJ,
		"830799": ExchangeBJ,
		"930001": ExchangeSH, // 9 prefix → SH
	}
	for input, want := range cases {
		sym, err := ParseSymbol(input)
		if err != nil {
			t.Fatalf("ParseSymbol(%s) error: %v", input, err)
		}
		if sym.Exchange != want {
			t.Fatalf("ParseSymbol(%s) exchange = %s, want %s", input, sym.Exchange, want)
		}
	}
}

func TestParseSymbolRejectsNonAShare(t *testing.T) {
	for _, input := range []string{
		"600519.HK", "AAPL.US", "AAPL", "12345", "1234567",
		"abcdef", "", "600519.",
	} {
		if _, err := ParseSymbol(input); err == nil {
			t.Fatalf("ParseSymbol(%q) expected error, got nil", input)
		}
	}
}
