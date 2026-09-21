package jev

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

const okBody = `{"model":"typesafe/jev-1.13","answers":{"q":{"type":"noul","noul":0.9}},"usage":{"input_tokens":1,"output_tokens":1}}`

var testProvider = Providers()[0]

func serverFailingThenOK(t *testing.T, status int, failures int32, header map[string]string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) <= failures {
			for key, value := range header {
				w.Header().Set(key, value)
			}
			w.WriteHeader(status)
			w.Write([]byte(`{"error":{"message":"transient"}}`))
			return
		}
		w.Write([]byte(okBody))
	}))
	t.Cleanup(server.Close)
	return server, &calls
}

func TestRetriesTransientStatusesUntilSuccess(t *testing.T) {
	transient := map[string]int{
		"rate limited":    http.StatusTooManyRequests,
		"request timeout": http.StatusRequestTimeout,
		"server error":    http.StatusInternalServerError,
		"overloaded":      529,
	}

	for name, status := range transient {
		t.Run(name, func(t *testing.T) {
			server, calls := serverFailingThenOK(t, status, 1, nil)
			client := NewClient(testProvider, server.URL, "key", 3, 5*time.Second)

			resp, err := client.Decide(context.Background(), Request{})

			if err != nil {
				t.Fatalf("want a decision after one retry, got error: %v", err)
			}
			if resp.Answers["q"].Type != TypeNoul {
				t.Errorf("want the noul answer decoded, got %+v", resp.Answers)
			}
			if got := calls.Load(); got != 2 {
				t.Errorf("want 2 requests (1 failure + 1 retry), got %d", got)
			}
		})
	}
}

func TestDoesNotRetryClientErrors(t *testing.T) {
	permanent := map[string]int{
		"bad request":   http.StatusBadRequest,
		"unauthorized":  http.StatusUnauthorized,
		"unprocessable": http.StatusUnprocessableEntity,
	}

	for name, status := range permanent {
		t.Run(name, func(t *testing.T) {
			server, calls := serverFailingThenOK(t, status, 99, nil)
			client := NewClient(testProvider, server.URL, "key", 3, 5*time.Second)

			_, err := client.Decide(context.Background(), Request{})

			apiErr, ok := errors.AsType[*APIError](err)
			if !ok {
				t.Fatalf("want an APIError, got %v", err)
			}
			if apiErr.StatusCode != status {
				t.Errorf("want status %d, got %d", status, apiErr.StatusCode)
			}
			if got := calls.Load(); got != 1 {
				t.Errorf("want exactly 1 request with no retry, got %d", got)
			}
		})
	}
}

func TestWaitsForRetryAfterMillisecondsHeader(t *testing.T) {
	const serverDirectedWait = 1200 * time.Millisecond

	server, _ := serverFailingThenOK(t, http.StatusTooManyRequests, 1,
		map[string]string{"retry-after-ms": "1200"})
	client := NewClient(testProvider, server.URL, "key", 2, 5*time.Second)

	start := time.Now()
	_, err := client.Decide(context.Background(), Request{})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("want a decision after the server-directed wait, got error: %v", err)
	}
	if elapsed < serverDirectedWait {
		t.Errorf("want the client to obey the %v the server asked for rather than its own backoff, waited %v",
			serverDirectedWait, elapsed)
	}
}

func TestWaitsForRetryAfterSecondsHeader(t *testing.T) {
	server, _ := serverFailingThenOK(t, http.StatusTooManyRequests, 1,
		map[string]string{"Retry-After": "1"})
	client := NewClient(testProvider, server.URL, "key", 2, 5*time.Second)

	start := time.Now()
	_, err := client.Decide(context.Background(), Request{})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("want a decision after the server-directed wait, got error: %v", err)
	}
	if elapsed < time.Second {
		t.Errorf("want the client to obey the 1s Retry-After rather than its own backoff, waited %v", elapsed)
	}
}

func TestGivesUpAfterConfiguredRetries(t *testing.T) {
	server, calls := serverFailingThenOK(t, http.StatusTooManyRequests, 99,
		map[string]string{"retry-after-ms": "1"})
	client := NewClient(testProvider, server.URL, "key", 2, 5*time.Second)

	_, err := client.Decide(context.Background(), Request{})

	if err == nil {
		t.Fatal("want an error once retries are exhausted, got nil")
	}
	if got := calls.Load(); got != 3 {
		t.Errorf("want 3 requests (1 initial + 2 retries), got %d", got)
	}
}

func TestStopsRetryingWhenContextIsCancelled(t *testing.T) {
	server, _ := serverFailingThenOK(t, http.StatusTooManyRequests, 99, nil)
	client := NewClient(testProvider, server.URL, "key", 5, 5*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := client.Decide(ctx, Request{})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("want the deadline to stop the retry loop, got %v", err)
	}
}

func TestParseRetryAfterReadsSecondsAndMilliseconds(t *testing.T) {
	cases := map[string]struct {
		header http.Header
		want   time.Duration
	}{
		"milliseconds wins": {http.Header{"Retry-After-Ms": {"1500"}, "Retry-After": {"9"}}, 1500 * time.Millisecond},
		"plain seconds":     {http.Header{"Retry-After": {"3"}}, 3 * time.Second},
		"absent":            {http.Header{}, 0},
		"unparseable":       {http.Header{"Retry-After": {"soon"}}, 0},
		"zero seconds":      {http.Header{"Retry-After": {"0"}}, 0},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := parseRetryAfter(tc.header); got != tc.want {
				t.Errorf("want %v, got %v", tc.want, got)
			}
		})
	}
}
