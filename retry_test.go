package jev_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	jev "github.com/withzombies/jev-go"
)

func TestRetryReplayAndPolicyIsolation(t *testing.T) {
	policy := jev.DefaultRetryPolicy()
	policy.BackoffInitial = 0
	policy.BackoffMax = 0
	bodies := []*trackedBody{}
	var sent []string
	h := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		wire, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		sent = append(sent, string(wire))
		n := len(sent)
		if n > 1 && !bodies[n-2].closed {
			t.Error("previous attempt body not closed")
		}
		wantRetry := ""
		if n == 2 {
			wantRetry = "1"
		}
		if n == 3 {
			wantRetry = "2"
		}
		if r.Header.Get("X-TypeSafe-Retry-Count") != wantRetry {
			t.Error("wrong retry count")
		}
		status := 503
		if n == 3 {
			status = 200
		}
		body := &trackedBody{Reader: strings.NewReader(oneResponse)}
		bodies = append(bodies, body)
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: body}, nil
	})}
	c, err := jev.NewClient(jev.Config{APIKey: "key", HTTPClient: h, Retry: policy})
	if err != nil {
		t.Fatal(err)
	}
	for i := range policy.HTTPStatuses {
		policy.HTTPStatuses[i] = 400
	}
	if _, err := c.SystemOne(t.Context(), oneRequest()); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 3 || sent[0] != sent[1] || sent[1] != sent[2] {
		t.Fatalf("request replay: %v", sent)
	}
	for _, body := range bodies {
		if !body.closed {
			t.Error("body leaked")
		}
	}
}

func TestRetrySelectionAndCallOverride(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		policy jev.RetryPolicy
		want   int
	}{
		{"disabled", 503, jev.RetryPolicy{}, 1},
		{"server", 503, jev.RetryPolicy{MaxRetries: 2, HTTPStatuses: []int{503}}, 3},
		{"unauthorized", 401, jev.DefaultRetryPolicy(), 1},
		{"custom", 409, jev.RetryPolicy{MaxRetries: 1, AdditionalRetry: func(err error) bool { var a *jev.APIError; return errors.As(err, &a) && a.StatusCode == 409 }}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c, err := jev.NewClient(jev.Config{APIKey: "key", Retry: jev.DefaultRetryPolicy(), HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: tc.status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("failed"))}, nil
			})}})
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.ListModels(t.Context(), jev.WithRetry(tc.policy))
			var apiErr *jev.APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != tc.status || calls != tc.want {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestRetryTransportAndReadErrors(t *testing.T) {
	for _, reading := range []bool{false, true} {
		t.Run(map[bool]string{false: "connection", true: "body"}[reading], func(t *testing.T) {
			calls := 0
			cause := io.ErrUnexpectedEOF
			c, err := jev.NewClient(jev.Config{APIKey: "key", Retry: jev.RetryPolicy{MaxRetries: 1, RetryConnectionErrors: true}, HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				if !reading {
					return nil, cause
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(errorReader{cause})}, nil
			})}})
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.ListModels(t.Context())
			var transport *jev.TransportError
			if calls != 2 || !errors.Is(err, cause) || !errors.As(err, &transport) || transport.Timeout() {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestRetryClockAndCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		policy := jev.DefaultRetryPolicy()
		policy.BackoffJitter = 0
		c, err := jev.NewClient(jev.Config{APIKey: "key", Retry: policy, HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After-Ms": {"1500"}}, Body: io.NopCloser(strings.NewReader("limited"))}, nil
		})}})
		if err != nil {
			t.Fatal(err)
		}
		started := time.Now()
		_, err = c.ListModels(t.Context())
		if err == nil || calls != 3 || time.Since(started) != 3*time.Second {
			t.Fatalf("calls=%d elapsed=%s err=%v", calls, time.Since(started), err)
		}
		calls = 0
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		_, err = c.ListModels(ctx)
		if !errors.Is(err, context.DeadlineExceeded) || calls != 1 {
			t.Fatalf("cancellation: calls=%d err=%v", calls, err)
		}
	})
}

func TestAttemptTimeoutRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for _, retryTimeout := range []bool{false, true} {
			calls := 0
			c, err := jev.NewClient(jev.Config{APIKey: "key", Timeout: time.Second, Retry: jev.RetryPolicy{MaxRetries: 1, RetryTimeouts: retryTimeout, RetryConnectionErrors: true}, HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				<-r.Context().Done()
				return nil, r.Context().Err()
			})}})
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.ListModels(t.Context())
			var transport *jev.TransportError
			want := 1
			if retryTimeout {
				want = 2
			}
			if calls != want || !errors.As(err, &transport) || !transport.Timeout() {
				t.Fatalf("timeout: calls=%d err=%v", calls, err)
			}
		}
	})
}

func TestRetryValidation(t *testing.T) {
	for _, policy := range []jev.RetryPolicy{{MaxRetries: -1}, {BackoffInitial: -1}, {BackoffMax: -1}, {BackoffJitter: 2}, {MaxRetryAfter: -1}, {HTTPStatuses: []int{99}}} {
		if _, err := jev.NewClient(jev.Config{APIKey: "key", Retry: policy}); err == nil {
			t.Fatal("accepted invalid policy")
		}
		c := clientFor(t, func(http.ResponseWriter, *http.Request) { t.Error("invalid policy reached server") })
		if _, err := c.ListModels(t.Context(), jev.WithRetry(policy)); err == nil {
			t.Fatal("accepted invalid override")
		}
	}
}

func TestOptInLogging(t *testing.T) {
	for _, tc := range []struct {
		level    string
		bodies   bool
		wantBody bool
		wantLog  bool
	}{{"info", false, false, true}, {"debug", false, false, true}, {"debug", true, true, true}, {"off", true, false, false}} {
		t.Run(tc.level+map[bool]string{true: "Bodies", false: ""}[tc.bodies], func(t *testing.T) {
			var log bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&log, &slog.HandlerOptions{Level: slog.LevelDebug}))
			//nolint:gosec // Dummy credentials verify that injected logging redacts sensitive headers.
			c, err := jev.NewClient(jev.Config{APIKey: "credential-value", Logger: logger, LogLevel: tc.level, LogBodies: tc.bodies, Headers: http.Header{"X-Secret-Test": {"hidden-value"}}, HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{"Set-Cookie": {"hidden-cookie"}}, Body: io.NopCloser(strings.NewReader(oneResponse))}, nil
			})}})
			if err != nil {
				t.Fatal(err)
			}
			req := oneRequest()
			req.State = "body-marker"
			if _, err := c.SystemOne(t.Context(), req); err != nil {
				t.Fatal(err)
			}
			text := log.String()
			if strings.Contains(text, "credential-value") || strings.Contains(text, "hidden-value") || strings.Contains(text, "hidden-cookie") {
				t.Fatal("credential leaked")
			}
			if strings.Contains(text, "body-marker") != tc.wantBody || (text != "") != tc.wantLog {
				t.Fatalf("unexpected logging: %s", text)
			}
		})
	}
}
