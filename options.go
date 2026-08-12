package ashare

import "time"

// httpOptions configure an HTTP-backed source. Options are applied at
// construction time and must not be mutated afterwards.
type httpOptions struct {
	baseURL     string
	timeout     time.Duration
	minInterval time.Duration
	backoff     []time.Duration
}

// Option configures an HTTP-backed source.
type Option func(*httpOptions)

// WithHTTPBaseURL overrides the provider endpoint (used by tests and proxies).
func WithHTTPBaseURL(url string) Option {
	return func(o *httpOptions) { o.baseURL = url }
}

// WithHTTPTimeout overrides the per-request socket timeout (default 15s).
func WithHTTPTimeout(d time.Duration) Option {
	return func(o *httpOptions) { o.timeout = d }
}

// WithMinInterval overrides the per-host minimum request spacing (default 1s
// for Eastmoney-backed sources; 0 disables throttling).
func WithMinInterval(d time.Duration) Option {
	return func(o *httpOptions) { o.minInterval = d }
}

// WithRateLimitBackoff overrides the Tushare rate-limit retry schedule. Each
// wait is the given duration; the default schedule is 5s, 20s, 40s.
func WithRateLimitBackoff(d time.Duration) Option {
	return func(o *httpOptions) { o.backoff = []time.Duration{d, d, d} }
}

func defaultHTTPOptions(base string) httpOptions {
	return httpOptions{
		baseURL: base,
		timeout: 15 * time.Second,
		backoff: []time.Duration{5 * time.Second, 20 * time.Second, 40 * time.Second},
	}
}
