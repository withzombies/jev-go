package jev

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RetryPolicy controls retries after a failed HTTP attempt. Its zero value makes
// one attempt. Policies are copied by [NewClient] and [WithRetry], including statuses.
// AdditionalRetry must be safe for concurrent calls if the client is shared.
type RetryPolicy struct {
	// MaxRetries is the maximum number of attempts after the initial attempt.
	MaxRetries int
	// BackoffInitial is the first exponential delay; zero disables backoff.
	BackoffInitial time.Duration
	// BackoffMax caps exponential delays; zero disables backoff.
	BackoffMax time.Duration
	// BackoffJitter is the randomly subtracted fraction of a delay, in [0,1].
	BackoffJitter float64
	// HTTPStatuses lists retryable HTTP statuses. Nil retries no HTTP statuses.
	HTTPStatuses []int
	// RespectRetryAfter enables retry-after-ms and Retry-After response hints.
	RespectRetryAfter bool
	// MaxRetryAfter bounds accepted server delays; zero allows any representable
	// delay. Longer hints fall back to exponential backoff.
	MaxRetryAfter time.Duration
	// RetryConnectionErrors includes connection failures and interrupted bodies.
	RetryConnectionErrors bool
	// RetryTimeouts controls attempt timeouts independently of connection errors.
	RetryTimeouts bool
	// AdditionalRetry may opt HTTP, transport or response-validation failures into retrying.
	// It cannot override caller cancellation or the retry limit. Local validation
	// and encoding errors never reach it.
	AdditionalRetry func(error) bool
}

// DefaultRetryPolicy returns a fresh SDK-style policy: two retries, 500ms initial
// backoff capped at 5s, 25% jitter, HTTP 408/429/5xx, connection errors and attempt
// timeouts. Server delay hints up to 60s are honored. It is never enabled implicitly.
// Use a context deadline to bound the total call, including waits and all attempts.
func DefaultRetryPolicy() RetryPolicy {
	statuses := []int{408, 429}
	for status := 500; status < 600; status++ {
		statuses = append(statuses, status)
	}
	return RetryPolicy{MaxRetries: 2, BackoffInitial: 500 * time.Millisecond, BackoffMax: 5 * time.Second, BackoffJitter: .25, HTTPStatuses: statuses, RespectRetryAfter: true, MaxRetryAfter: time.Minute, RetryConnectionErrors: true, RetryTimeouts: true}
}

// WithRetry replaces the client's policy for one call with a snapshot of policy.
// Pass RetryPolicy{} to disable retries, or edit a [DefaultRetryPolicy] copy to
// override individual SDK defaults without ambiguous zero values.
func WithRetry(policy RetryPolicy) RequestOption {
	snapshot := policy.clone()
	return func(o *requestOptions) error {
		if err := snapshot.validate(); err != nil {
			return err
		}
		o.retry = snapshot
		return nil
	}
}

func (p RetryPolicy) clone() RetryPolicy {
	p.HTTPStatuses = append([]int(nil), p.HTTPStatuses...)
	return p
}
func (p RetryPolicy) validate() error {
	if p.MaxRetries < 0 || p.BackoffInitial < 0 || p.BackoffMax < 0 || p.MaxRetryAfter < 0 {
		return fmt.Errorf("jev: retry counts and delays must not be negative")
	}
	if math.IsNaN(p.BackoffJitter) || p.BackoffJitter < 0 || p.BackoffJitter > 1 {
		return fmt.Errorf("jev: retry jitter must be in [0,1]")
	}
	for _, s := range p.HTTPStatuses {
		if s < 100 || s > 599 {
			return fmt.Errorf("jev: invalid retry status %d", s)
		}
	}
	return nil
}
func (p RetryPolicy) retryable(err error) bool {
	var transport *TransportError
	var api *APIError
	builtin := false
	switch {
	case errors.As(err, &transport):
		if transport.Timeout() {
			builtin = p.RetryTimeouts
		} else {
			builtin = p.RetryConnectionErrors
		}
	case errors.As(err, &api):
		for _, status := range p.HTTPStatuses {
			if api.StatusCode == status {
				builtin = true
				break
			}
		}
	}
	return builtin || (p.AdditionalRetry != nil && p.AdditionalRetry(err))
}

func retryDelay(p RetryPolicy, attempt int, headers http.Header, now time.Time, random float64) time.Duration {
	if p.RespectRetryAfter {
		if delay, ok := parseRetryAfter(headers, now); ok && (p.MaxRetryAfter == 0 || delay <= p.MaxRetryAfter) {
			return delay
		}
	}
	delay := min(p.BackoffInitial, p.BackoffMax)
	for i := 0; i < attempt && delay > 0 && delay < p.BackoffMax; i++ {
		if delay > p.BackoffMax/2 {
			delay = p.BackoffMax
		} else {
			delay *= 2
		}
	}
	adjusted := float64(delay) * (1 - random*p.BackoffJitter)
	// Float conversion can round large durations up beyond int64. Jitter only
	// subtracts, so preserve the integer cap before converting back.
	if adjusted >= float64(delay) {
		return delay
	}
	return time.Duration(adjusted)
}

func parseRetryAfter(headers http.Header, now time.Time) (time.Duration, bool) {
	for _, item := range []struct {
		name string
		unit time.Duration
	}{{"Retry-After-Ms", time.Millisecond}, {"Retry-After", time.Second}} {
		raw := strings.TrimSpace(headers.Get(item.name))
		if raw == "" {
			continue
		}
		value, err := strconv.ParseFloat(raw, 64)
		if err == nil && value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0) && value < float64(math.MaxInt64)/float64(item.unit) {
			return time.Duration(value * float64(item.unit)), true
		}
		if item.name == "Retry-After" {
			if date, err := http.ParseTime(raw); err == nil {
				return max(0, date.Sub(now)), true
			}
		}
	}
	return 0, false
}
