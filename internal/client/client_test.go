package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestDoUpdate(t *testing.T) {
	var gotPath, gotAuth, gotMethod, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		gotAuth = r.Header.Get("Authorization")
		gotMethod = r.Method
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"summary":{"scanned":8,"updated":2,"failed":0},"timing":{"duration_ms":1250,"duration":"1.25s"},"timestamp":"2025-01-20T11:30:45Z","api_version":"v1"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "mytoken", 5*time.Second, false)
	q := url.Values{}
	q.Set("image", "foo/bar:1.0,foo/baz")
	res, err := c.Do(context.Background(), http.MethodPost, "/v1/update", q)
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method: %s", gotMethod)
	}
	if gotAuth != "Bearer mytoken" {
		t.Errorf("auth: %q", gotAuth)
	}
	if gotUA != UserAgent {
		t.Errorf("user-agent: %q", gotUA)
	}
	parsed, _ := url.Parse(gotPath)
	if parsed.Path != "/v1/update" {
		t.Errorf("path: %s", parsed.Path)
	}
	if got := parsed.Query().Get("image"); got != "foo/bar:1.0,foo/baz" {
		t.Errorf("image param: %q", got)
	}
	if !strings.Contains(string(res.Body), `"updated":2`) {
		t.Errorf("body: %s", res.Body)
	}
}

func TestDoAsyncAccepted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "t", 5*time.Second, false)
	q := url.Values{}
	q.Set("async", "true")
	res, err := c.Do(context.Background(), http.MethodPost, "/v1/update", q)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != http.StatusAccepted {
		t.Errorf("status: %d", res.Status)
	}
}

func TestDoUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid token","api_version":"v1","timestamp":"2025-01-20T11:30:45Z"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "wrong", 5*time.Second, false)
	res, err := c.Do(context.Background(), http.MethodGet, "/v1/containers", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != http.StatusUnauthorized {
		t.Errorf("status: %d", res.Status)
	}
	if !strings.Contains(string(res.Body), "invalid token") {
		t.Errorf("body: %s", res.Body)
	}
}

func TestDoRetryAfter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":"another update is already running"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "t", 5*time.Second, false)
	res, err := c.Do(context.Background(), http.MethodPost, "/v1/update", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Header.Get("Retry-After") != "30" {
		t.Errorf("Retry-After: %q", res.Header.Get("Retry-After"))
	}
}

func TestMetricsRawText(t *testing.T) {
	const payload = "watchtower_scanned_total 42\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/metrics" {
			t.Errorf("path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(payload))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "t", 5*time.Second, false)
	res, err := c.Do(context.Background(), http.MethodGet, "/v1/metrics", nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(res.Body) != payload {
		t.Errorf("body: %q", res.Body)
	}
}

func TestHealthNoAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/livez" {
			t.Errorf("path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("auth: %q", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "", 5*time.Second, false)
	res, err := c.Do(context.Background(), http.MethodGet, "/livez", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != http.StatusOK {
		t.Errorf("status: %d", res.Status)
	}
}

func TestStreamUsesEventsTokenAndAccept(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer events-secret" {
			t.Errorf("auth: %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("accept: %q", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("event: scan_completed\ndata: {}\n\n"))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "api-token", 5*time.Second, false).WithToken("events-secret")
	status, body, err := c.Stream(context.Background(), http.MethodGet, "/v1/events")
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	if status != http.StatusOK {
		t.Errorf("status: %d", status)
	}
	b, _ := io.ReadAll(body)
	if !strings.Contains(string(b), "scan_completed") {
		t.Errorf("body: %s", b)
	}
}

func TestProbe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	c := NewClient(srv.URL, "t", 5*time.Second, false)
	report, err := c.Probe(context.Background(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if report.ConnectErr != "" {
		t.Errorf("connect error: %s", report.ConnectErr)
	}
	if report.Host == "" {
		t.Error("no host")
	}
}
