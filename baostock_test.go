package ashare

import (
	"context"
	"errors"
	"testing"

	"github.com/millken/baostock"
)

var errFakeLogin = errors.New("fake login failure")

type fakeBaostock struct {
	req       *baostock.HistoryKDataRequest
	rows      [][]string
	fields    []string
	loginErr  error
	queryErr  error
	loggedOut bool
}

func (f *fakeBaostock) Login(ctx context.Context) error { return f.loginErr }
func (f *fakeBaostock) Logout(ctx context.Context) error {
	f.loggedOut = true
	return nil
}
func (f *fakeBaostock) QueryHistoryKDataPlus(ctx context.Context, req *baostock.HistoryKDataRequest, cb func(fields []string, record []string) error) error {
	if f.queryErr != nil {
		return f.queryErr
	}
	f.req = req
	for _, r := range f.rows {
		if err := cb(f.fields, r); err != nil {
			return err
		}
	}
	return nil
}

func newFakeBaostock(f *fakeBaostock) *Baostock {
	return &Baostock{newClient: func() (baostockClient, error) { return f, nil }}
}

func TestBaostockDailyParsesRows(t *testing.T) {
	fake := &fakeBaostock{
		fields: baostockFields,
		rows: [][]string{
			{"2026-06-01", "21.25", "21.45", "21.15", "21.35", "1200", "25620"},
			{"2026-06-02", "21.4", "21.6", "21.3", "21.5", "1000", "21500"},
		},
	}
	src := newFakeBaostock(fake)
	sym, _ := ParseSymbol("600519.SH")
	bars, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-02"))
	if err != nil {
		t.Fatalf("Daily error: %v", err)
	}
	if len(bars) != 2 {
		t.Fatalf("expected 2 bars, got %d", len(bars))
	}
	b0 := bars[0]
	if !b0.Date.Equal(d("2026-06-01")) || b0.Close != 21.35 || b0.Open != 21.25 ||
		b0.High != 21.45 || b0.Low != 21.15 || b0.Volume != 1200 || b0.Amount != 25620 {
		t.Fatalf("unexpected first bar: %+v", b0)
	}
}

func TestBaostockBuildsRequest(t *testing.T) {
	fake := &fakeBaostock{
		fields: baostockFields,
		rows:   [][]string{{"2026-06-01", "21.25", "21.45", "21.15", "21.35", "1200", "25620"}},
	}
	src := newFakeBaostock(fake)
	sym, _ := ParseSymbol("600519.SH")
	if _, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-02")); err != nil {
		t.Fatalf("Daily error: %v", err)
	}
	req := fake.req
	if req.Code != "sh.600519" {
		t.Fatalf("Code = %s, want sh.600519", req.Code)
	}
	if req.StartDate != "2026-06-01" || req.EndDate != "2026-06-02" {
		t.Fatalf("dates = %s..%s, want 2026-06-01..2026-06-02", req.StartDate, req.EndDate)
	}
	if req.Frequency != baostock.FrequencyDaily {
		t.Fatalf("Frequency = %s, want %s", req.Frequency, baostock.FrequencyDaily)
	}
	if req.AdjustFlag != baostock.AdjustFlagNoAdjust {
		t.Fatalf("AdjustFlag = %s, want %s (不复权)", req.AdjustFlag, baostock.AdjustFlagNoAdjust)
	}
	if req.Fields == "" {
		t.Fatal("Fields must not be empty")
	}
	for _, f := range []string{"date", "open", "high", "low", "close", "volume", "amount"} {
		if !containsField(req.Fields, f) {
			t.Fatalf("Fields %q missing %s", req.Fields, f)
		}
	}
}

func containsField(fields, want string) bool {
	for i := 0; i+len(want) <= len(fields); i++ {
		if fields[i] == want[0] && (i+len(want) == len(fields) || fields[i+len(want)] == ',') &&
			(i == 0 || fields[i-1] == ',') && fields[i:i+len(want)] == want {
			return true
		}
	}
	return false
}

func TestBaostockMapsShenzhen(t *testing.T) {
	fake := &fakeBaostock{
		fields: baostockFields,
		rows:   [][]string{{"2026-06-01", "21.25", "21.45", "21.15", "21.35", "1200", "25620"}},
	}
	src := newFakeBaostock(fake)
	sym, _ := ParseSymbol("000001.SZ")
	if _, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-02")); err != nil {
		t.Fatalf("Daily error: %v", err)
	}
	if fake.req.Code != "sz.000001" {
		t.Fatalf("Code = %s, want sz.000001", fake.req.Code)
	}
}

func TestBaostockRejectsBeijing(t *testing.T) {
	src := newFakeBaostock(&fakeBaostock{})
	sym, _ := ParseSymbol("430139.BJ")
	if _, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-02")); err == nil {
		t.Fatal("expected error for BJ symbol")
	}
}

func TestBaostockLoginFailure(t *testing.T) {
	src := newFakeBaostock(&fakeBaostock{loginErr: errFakeLogin})
	sym, _ := ParseSymbol("600519.SH")
	if _, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-02")); err == nil {
		t.Fatal("expected error when login fails")
	}
}

func TestBaostockEmptyDataIsError(t *testing.T) {
	src := newFakeBaostock(&fakeBaostock{fields: baostockFields})
	sym, _ := ParseSymbol("600519.SH")
	if _, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-02")); err == nil {
		t.Fatal("expected error for empty data")
	}
}

func TestBaostockLogsOutAfterFetch(t *testing.T) {
	fake := &fakeBaostock{fields: baostockFields, rows: [][]string{{"2026-06-01", "21.25", "21.45", "21.15", "21.35", "1200", "25620"}}}
	src := newFakeBaostock(fake)
	sym, _ := ParseSymbol("600519.SH")
	if _, err := src.Daily(context.Background(), sym, d("2026-06-01"), d("2026-06-02")); err != nil {
		t.Fatalf("Daily error: %v", err)
	}
	if !fake.loggedOut {
		t.Fatal("expected Logout after fetch")
	}
}

func TestBaostockNameAndAvailable(t *testing.T) {
	src := newFakeBaostock(&fakeBaostock{})
	if !src.Available(context.Background()) {
		t.Fatal("baostock should always be available")
	}
	if src.Name() != "baostock" {
		t.Fatalf("name = %s, want baostock", src.Name())
	}
}
