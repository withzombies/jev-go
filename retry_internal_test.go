package jev

import (
	"math"
	"net/http"
	"testing"
	"time"
)

func TestRetryDelay(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	p := DefaultRetryPolicy()
	for _, tc := range []struct {
		name    string
		attempt int
		headers http.Header
		want    time.Duration
	}{
		{"initial", 0, nil, 375 * time.Millisecond},
		{"capped", 10000, nil, 3750 * time.Millisecond},
		{"milliseconds", 0, http.Header{"Retry-After-Ms": {"250"}, "Retry-After": {"8"}}, 250 * time.Millisecond},
		{"fallback seconds", 0, http.Header{"Retry-After-Ms": {"NaN"}, "Retry-After": {"1.5"}}, 1500 * time.Millisecond},
		{"date", 0, http.Header{"Retry-After": {now.Add(time.Second).Format(http.TimeFormat)}}, time.Second},
		{"past", 0, http.Header{"Retry-After": {now.Add(-time.Second).Format(http.TimeFormat)}}, 0},
		{"too long", 0, http.Header{"Retry-After": {"61"}}, 375 * time.Millisecond},
		{"overflow", 0, http.Header{"Retry-After": {"1e100"}}, 375 * time.Millisecond},
		{"negative", 0, http.Header{"Retry-After": {"-1"}}, 375 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := retryDelay(p, tc.attempt, tc.headers, now, 1); got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
	p.RespectRetryAfter = false
	if got := retryDelay(p, 0, http.Header{"Retry-After": {"1"}}, now, 0); got != 500*time.Millisecond {
		t.Fatalf("ignored header: %s", got)
	}
	p.RespectRetryAfter = true
	p.MaxRetryAfter = 0
	if got := retryDelay(p, 0, http.Header{"Retry-After": {"61"}}, now, 0); got != 61*time.Second {
		t.Fatalf("unlimited header: %s", got)
	}
	p.BackoffJitter = math.NaN()
	if err := p.validate(); err == nil {
		t.Fatal("accepted NaN jitter")
	}
}
