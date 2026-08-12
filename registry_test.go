package ashare

import (
	"context"
	"testing"
)

func TestDefaultChainOrder(t *testing.T) {
	got := DefaultChain()
	want := []string{"tencent", "mootdx", "eastmoney", "baostock", "akshare", "tushare"}
	if len(got) != len(want) {
		t.Fatalf("chain length = %d, want %d", len(got), len(want))
	}
	for i, name := range want {
		if got[i].Name() != name {
			t.Fatalf("chain[%d] = %s, want %s", i, got[i].Name(), name)
		}
	}
}

func TestNewUsesDefaultChain(t *testing.T) {
	c := New()
	if len(c.sources) != 6 {
		t.Fatalf("New() sources = %d, want 6", len(c.sources))
	}
	want := []string{"tencent", "mootdx", "eastmoney", "baostock", "akshare", "tushare"}
	for i, name := range want {
		if c.sources[i].Name() != name {
			t.Fatalf("New() source[%d] = %s, want %s", i, c.sources[i].Name(), name)
		}
	}
}

func TestDefaultChainTushareTokenFromEnv(t *testing.T) {
	t.Setenv("TUSHARE_TOKEN", "secret-token")
	ts, ok := DefaultChain()[5].(*Tushare)
	if !ok {
		t.Fatal("chain[5] is not *Tushare")
	}
	if ts.token != "secret-token" {
		t.Fatalf("token = %q, want secret-token", ts.token)
	}
	if !ts.Available(context.Background()) {
		t.Fatal("tushare should be available when token is set")
	}
}

func TestDefaultChainTushareUnavailableWithoutToken(t *testing.T) {
	t.Setenv("TUSHARE_TOKEN", "")
	ts := DefaultChain()[5].(*Tushare)
	if ts.Available(context.Background()) {
		t.Fatal("tushare should be unavailable when no token is configured")
	}
}
