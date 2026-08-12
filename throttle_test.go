package ashare

import (
	"context"
	"testing"
	"time"
)

func TestThrottleZeroIntervalDoesNotBlock(t *testing.T) {
	h := NewHostThrottle()
	start := time.Now()
	if err := h.Wait(context.Background(), "b", 0); err != nil {
		t.Fatalf("Wait error: %v", err)
	}
	if err := h.Wait(context.Background(), "b", 0); err != nil {
		t.Fatalf("Wait error: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("zero interval should not block, took %v", elapsed)
	}
}

func TestThrottleSpacesSameBucket(t *testing.T) {
	h := NewHostThrottle()
	minInterval := 50 * time.Millisecond
	_ = h.Wait(context.Background(), "bucket", minInterval)
	start := time.Now()
	_ = h.Wait(context.Background(), "bucket", minInterval)
	if elapsed := time.Since(start); elapsed < minInterval {
		t.Fatalf("second call waited %v, want >= %v", elapsed, minInterval)
	}
}

func TestThrottleDifferentBucketsDoNotBlockEachOther(t *testing.T) {
	h := NewHostThrottle()
	minInterval := 200 * time.Millisecond
	_ = h.Wait(context.Background(), "a", minInterval)
	start := time.Now()
	_ = h.Wait(context.Background(), "b", minInterval)
	if elapsed := time.Since(start); elapsed >= minInterval/2 {
		t.Fatalf("different buckets must not block each other, waited %v", elapsed)
	}
}

func TestThrottleCanceledContextReturnsError(t *testing.T) {
	h := NewHostThrottle()
	minInterval := time.Hour
	_ = h.Wait(context.Background(), "c", minInterval)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := h.Wait(ctx, "c", minInterval); err == nil {
		t.Fatal("expected context-canceled error, got nil")
	}
}
