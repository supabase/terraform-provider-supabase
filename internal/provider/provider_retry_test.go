// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/supabase/cli/pkg/api"
)

func testRetryClient(t *testing.T, serverURL string) *api.Client {
	t.Helper()

	client, err := api.NewClient(
		serverURL,
		api.WithHTTPClient(newRetryableClient()),
		api.WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Bearer test")
			req.Header.Set("User-Agent", "TFProvider/test")
			return nil
		}),
		api.WithRequestEditorFn(stashRequestMethod),
	)
	if err != nil {
		t.Fatalf("failed to create retry client: %v", err)
	}

	return client
}

func TestRetry_GetSucceedsAfter502(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := attempts.Add(1)
		if attempt == 1 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("error code: 502"))
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := testRetryClient(t, server.URL)
	resp, err := client.V1GetProject(context.Background(), "project-ref")
	if err != nil {
		t.Fatalf("expected success after retry, got error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	if got := attempts.Load(); got != 2 {
		t.Fatalf("expected 2 attempts, got %d", got)
	}
}

func TestRetry_GetSucceedsAfterTransportError(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := attempts.Add(1)
		if attempt == 1 {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Error("response writer does not support hijacking")
				return
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Errorf("failed to hijack connection: %v", err)
				return
			}
			_ = conn.Close()
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := testRetryClient(t, server.URL)
	resp, err := client.V1GetProject(context.Background(), "project-ref")
	if err != nil {
		t.Fatalf("expected success after retry, got error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	if got := attempts.Load(); got != 2 {
		t.Fatalf("expected 2 attempts, got %d", got)
	}
}

func TestRetry_PostDoesNotRetry(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("error code: 502"))
	}))
	defer server.Close()

	client := testRetryClient(t, server.URL)
	resp, err := client.V1CreateAProjectWithBody(context.Background(), "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("expected HTTP response without retry, got error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected status 502, got %d", resp.StatusCode)
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("expected 1 attempt, got %d", got)
	}
}

func TestRetry_ExhaustedGetReturnsResponse(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("error code: 502"))
	}))
	defer server.Close()

	client := testRetryClient(t, server.URL)
	resp, err := client.V1GetProject(context.Background(), "project-ref")
	if err != nil {
		t.Fatalf("expected final HTTP response, got error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected status 502, got %d", resp.StatusCode)
	}
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		t.Fatalf("failed to read response body: %v", readErr)
	}
	if got := string(body); got != "error code: 502" {
		t.Fatalf("expected response body to be preserved, got %q", got)
	}
	if got := attempts.Load(); got != 4 {
		t.Fatalf("expected 4 attempts, got %d", got)
	}
}

func TestRetry_ExhaustedTransportErrorReturnsError(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("response writer does not support hijacking")
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Errorf("failed to hijack connection: %v", err)
			return
		}
		_ = conn.Close()
	}))
	defer server.Close()

	client := testRetryClient(t, server.URL)
	resp, err := client.V1GetProject(context.Background(), "project-ref")
	if err == nil {
		if resp != nil {
			resp.Body.Close()
		}
		t.Fatal("expected transport error after retries are exhausted")
	}
	if resp != nil {
		defer resp.Body.Close()
		t.Fatalf("expected nil response on transport error, got status %d", resp.StatusCode)
	}
	if got := attempts.Load(); got != 4 {
		t.Fatalf("expected 4 attempts, got %d", got)
	}
}

func TestRetry_ContextCancelledStopsRetries(t *testing.T) {
	var attempts atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := attempts.Add(1)
		if attempt == 1 {
			cancel()
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("temporarily unavailable"))
	}))
	defer server.Close()

	client := testRetryClient(t, server.URL)
	resp, err := client.V1GetProject(ctx, "project-ref")
	if !errors.Is(err, context.Canceled) {
		if resp != nil {
			resp.Body.Close()
		}
		t.Fatalf("expected context canceled error, got %v", err)
	}
	if resp != nil {
		defer resp.Body.Close()
		t.Fatalf("expected nil response after cancellation, got status %d", resp.StatusCode)
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("expected 1 attempt before cancellation, got %d", got)
	}
}

func TestGetOnlyRetryPolicy(t *testing.T) {
	ctxCanceled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name      string
		ctx       context.Context
		resp      *http.Response
		err       error
		wantRetry bool
		wantErr   error
	}{
		{
			name:      "context canceled without response returns ctx error",
			ctx:       ctxCanceled,
			wantRetry: false,
			wantErr:   context.Canceled,
		},
		{
			name:      "context canceled with existing response returns no error",
			ctx:       ctxCanceled,
			resp:      &http.Response{StatusCode: http.StatusBadGateway, Request: httptest.NewRequest(http.MethodGet, "http://example.com", nil)},
			wantRetry: false,
			wantErr:   nil,
		},
		{
			name:      "get retries on 502",
			ctx:       context.Background(),
			resp:      &http.Response{StatusCode: http.StatusBadGateway, Request: httptest.NewRequest(http.MethodGet, "http://example.com", nil)},
			wantRetry: true,
		},
		{
			name:      "get does not retry on 404",
			ctx:       context.Background(),
			resp:      &http.Response{StatusCode: http.StatusNotFound, Request: httptest.NewRequest(http.MethodGet, "http://example.com", nil)},
			wantRetry: false,
		},
		{
			name:      "post does not retry on 502",
			ctx:       context.Background(),
			resp:      &http.Response{StatusCode: http.StatusBadGateway, Request: httptest.NewRequest(http.MethodPost, "http://example.com", nil)},
			wantRetry: false,
		},
		{
			name:      "get retries recoverable transport error",
			ctx:       context.WithValue(context.Background(), retryMethodKey{}, http.MethodGet),
			err:       &url.Error{Op: "Get", URL: "http://example.com", Err: io.EOF},
			wantRetry: true,
		},
		{
			name:      "get does not retry permanent transport error",
			ctx:       context.WithValue(context.Background(), retryMethodKey{}, http.MethodGet),
			err:       &url.Error{Op: "Get", URL: "http://example.com", Err: errors.New(`unsupported protocol scheme "ftp"`)},
			wantRetry: false,
			wantErr:   &url.Error{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRetry, gotErr := getOnlyRetryPolicy(tt.ctx, tt.resp, tt.err)
			if gotRetry != tt.wantRetry {
				t.Fatalf("expected retry=%t, got %t", tt.wantRetry, gotRetry)
			}
			if tt.wantErr == nil && gotErr != nil {
				t.Fatalf("expected nil error, got %v", gotErr)
			}
			if tt.wantErr != nil {
				switch want := tt.wantErr.(type) {
				case *url.Error:
					var gotURL *url.Error
					if !errors.As(gotErr, &gotURL) {
						t.Fatalf("expected url.Error, got %v", gotErr)
					}
				default:
					if !errors.Is(gotErr, want) {
						t.Fatalf("expected error %v, got %v", want, gotErr)
					}
				}
			}
		})
	}
}

func TestCappedJitterBackoff_429UsesRetryAfter(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{"Retry-After": {"2"}},
	}

	wait := cappedJitterBackoff(500*time.Millisecond, 1500*time.Millisecond, 0, resp)
	if wait != 2*time.Second {
		t.Fatalf("expected Retry-After wait of 2s, got %s", wait)
	}
}

func TestCappedJitterBackoff_503UsesRetryAfter(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     http.Header{"Retry-After": {"3"}},
	}

	wait := cappedJitterBackoff(500*time.Millisecond, 1500*time.Millisecond, 0, resp)
	if wait != 3*time.Second {
		t.Fatalf("expected Retry-After wait of 3s, got %s", wait)
	}
}

func TestCappedJitterBackoff_CapsLargeRetryAfter(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{"Retry-After": {"120"}},
	}

	wait := cappedJitterBackoff(500*time.Millisecond, 1500*time.Millisecond, 0, resp)
	if wait != maxRetryAfterWait {
		t.Fatalf("expected capped wait of %s, got %s", maxRetryAfterWait, wait)
	}
}

func TestCappedJitterBackoff_InvalidRetryAfterFallsBackToJitter(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{"Retry-After": {"not-a-number"}},
	}

	wait := cappedJitterBackoff(500*time.Millisecond, 1500*time.Millisecond, 0, resp)
	assertDurationInRange(t, wait, 500*time.Millisecond, 1500*time.Millisecond)
}

func TestCappedJitterBackoff_NonRateLimitedStatusIgnoresRetryAfter(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusBadGateway,
		Header:     http.Header{"Retry-After": {"120"}},
	}

	wait := cappedJitterBackoff(500*time.Millisecond, 1500*time.Millisecond, 0, resp)
	assertDurationInRange(t, wait, 500*time.Millisecond, 1500*time.Millisecond)
}

func TestCappedJitterBackoff_AttemptScalingFallbackRange(t *testing.T) {
	resp := &http.Response{StatusCode: http.StatusBadGateway}

	wait := cappedJitterBackoff(500*time.Millisecond, 1500*time.Millisecond, 2, resp)
	assertDurationInRange(t, wait, 1500*time.Millisecond, 4500*time.Millisecond)
}

func assertDurationInRange(t *testing.T, got, minWait, maxWait time.Duration) {
	t.Helper()

	if got < minWait || got > maxWait {
		t.Fatalf("expected duration in range [%s, %s], got %s", minWait, maxWait, got)
	}
}
