package cli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dpan/watchtower-cli/internal/config"
)

func exec(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := NewRoot()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestCommands(t *testing.T) {
	var lastURI string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastURI = r.URL.RequestURI()
		switch r.URL.Path {
		case "/v1/update":
			if r.URL.Query().Get("async") == "true" {
				w.WriteHeader(http.StatusAccepted)
				w.Write([]byte(`{"status":"accepted"}`))
				return
			}
			if r.Method != http.MethodPost {
				t.Errorf("update method: %s", r.Method)
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"summary":{"scanned":8,"updated":0,"failed":0,"restarted":0,"skipped":2},"timing":{"duration_ms":1250,"duration":"1.25s"},"timestamp":"2025-01-20T11:30:45Z","api_version":"v1"}`))
		case "/v1/check":
			w.Write([]byte(`{"containers":[{"name":"nginx","image":"nginx:latest","update_available":true}],"count":1,"timestamp":"2025-01-20T11:30:45Z","api_version":"v1"}`))
		case "/v1/containers":
			w.Write([]byte(`{"containers":[{"name":"nginx","image":"nginx:latest","image_id":"sha256:1111"}],"count":1,"timestamp":"2025-01-20T11:30:45Z","api_version":"v1"}`))
		case "/v1/containers/details":
			w.Write([]byte(`{"containers":[{"name":"nginx","running":true,"enabled":true}],"count":1,"timestamp":"2025-01-20T11:30:45Z","api_version":"v1"}`))
		case "/v1/history":
			w.Write([]byte(`{"entries":[{"timestamp":"2025-01-20T11:30:45Z","scanned":8}],"count":1,"timestamp":"2025-01-20T11:35:00Z","api_version":"v1"}`))
		case "/v1/images":
			w.Write([]byte(`{"images":[{"name":"nginx:latest","containers":3}],"count":1,"timestamp":"2025-01-20T11:30:45Z","api_version":"v1"}`))
		case "/v1/config":
			w.Write([]byte(`{"config":{"monitor_only":false,"cleanup":true},"timestamp":"2025-01-20T11:30:45Z","api_version":"v1"}`))
		case "/v1/status":
			w.Write([]byte(`{"summary":{"scanned":8},"timestamp":"2025-01-20T11:30:45Z","api_version":"v1"}`))
		case "/v1/metrics":
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte("watchtower_scanned_total 42\n"))
		case "/livez", "/readyz", "/startupz":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"not found"}`))
		}
	}))
	defer srv.Close()

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"update", []string{"update"}, "1.25s"},
		{"update-async", []string{"update", "--async"}, "accepted"},
		{"check", []string{"check"}, "update_available"},
		{"containers", []string{"containers"}, "sha256:1111"},
		{"container-details", []string{"containers", "details"}, "running"},
		{"history", []string{"history", "--limit", "10"}, "scanned"},
		{"images", []string{"images"}, "containers"},
		{"config", []string{"config"}, "monitor_only"},
		{"status", []string{"status"}, "summary"},
		{"metrics", []string{"metrics"}, "watchtower_scanned_total"},
		{"liveness", []string{"health", "live"}, "/livez 200 OK"},
		{"readiness", []string{"health", "ready"}, "/readyz 200 OK"},
		{"startup", []string{"health", "startup"}, "/startupz 200 OK"},
		{"liveness-alias", []string{"health", "liveness"}, "/livez 200 OK"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--url", srv.URL}, tc.args...)
			out, err := exec(t, args...)
			if err != nil {
				t.Fatalf("exec(%v): %v\n%s", tc.args, err, out)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("output for %v missing %q, got: %s", tc.args, tc.want, out)
			}
		})
	}

	t.Run("update-api-timeout", func(t *testing.T) {
		_, err := exec(t, "--url", srv.URL, "update", "--api-timeout", "2m")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(lastURI, "timeout=2m") {
			t.Errorf("uri missing timeout: %s", lastURI)
		}
	})
}

func TestHelpDoesNotRequireURL(t *testing.T) {
	t.Setenv("WATCHTOWER_HTTP_API_URL", "")
	out, err := exec(t, "help", "update")
	if err != nil {
		t.Fatalf("help: %v\n%s", err, out)
	}
	if !strings.Contains(out, "POST /v1/update") {
		t.Errorf("output: %s", out)
	}
}

func TestNoCommandShowsHelp(t *testing.T) {
	t.Setenv("WATCHTOWER_HTTP_API_URL", "")
	cases := [][]string{
		nil,
		{"--url", "http://127.0.0.1:1"},
	}
	for _, args := range cases {
		out, err := exec(t, args...)
		if err != nil {
			t.Fatalf("args %v: %v\n%s", args, err, out)
		}
		if !strings.Contains(out, "Available Commands") || !strings.Contains(out, "update") {
			t.Errorf("args %v help:\n%s", args, out)
		}
	}
}

func TestMissingURL(t *testing.T) {
	t.Setenv("WATCHTOWER_HTTP_API_URL", "")
	_, err := exec(t, "status")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "URL required") {
		t.Errorf("error: %v", err)
	}
}

func TestEventsRequiresToken(t *testing.T) {
	t.Setenv("WATCHTOWER_HTTP_API_EVENTS_TOKEN", "")
	_, err := exec(t, "--url", "http://127.0.0.1:1", "events")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "events token required") {
		t.Errorf("error: %v", err)
	}
}

func TestHTTPErrorRetryAfter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":"another update is already running"}`))
	}))
	defer srv.Close()

	_, err := exec(t, "--url", srv.URL, "update")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "retry after 30") {
		t.Errorf("error: %v", err)
	}
}

func TestRunWithTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, config.ConfigFileName)
	content := `[[target]]
name = "prod"
url = "http://127.0.0.1:1"
token = "from-config"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WATCHTOWER_CLI_CONFIG", path)

	_, err := exec(t, "--target", "nope", "status")
	if err == nil {
		t.Fatal("expected error for unknown target")
	}

	_, err = exec(t, "--target", "prod", "status")
	if err == nil {
		t.Fatal("expected connection error against port 1")
	}
	if !strings.Contains(err.Error(), "127.0.0.1:1") {
		t.Errorf("expected config URL in error, got: %v", err)
	}
}

func TestStatusNoContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	out, err := exec(t, "--url", srv.URL, "status")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no update scan has run yet") {
		t.Errorf("output: %s", out)
	}
}

func TestHandlerDeadline(t *testing.T) {
	cases := []struct {
		client, api, want time.Duration
	}{
		{10 * time.Minute, 0, 10*time.Minute + handlerGrace},
		{10 * time.Minute, 15 * time.Minute, 15*time.Minute + handlerGrace},
		{10 * time.Minute, 2 * time.Minute, 10*time.Minute + handlerGrace},
		{0, 5 * time.Minute, 0},
	}
	for _, tc := range cases {
		if got := handlerDeadline(tc.client, tc.api); got != tc.want {
			t.Errorf("handlerDeadline(%s, %s) = %s, want %s", tc.client, tc.api, got, tc.want)
		}
	}
}

func TestAPITimeoutValidation(t *testing.T) {
	_, err := exec(t, "--url", "http://127.0.0.1:1", "update", "--api-timeout", "2min")
	if err == nil || !strings.Contains(err.Error(), "invalid argument") {
		t.Fatalf("2min: %v", err)
	}
	_, err = exec(t, "--url", "http://127.0.0.1:1", "update", "--api-timeout", "-1s")
	if err == nil || !strings.Contains(err.Error(), "--api-timeout must be positive") {
		t.Fatalf("-1s: %v", err)
	}
}

func TestEventsCancel(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}
		_, _ = w.Write([]byte("data: hello\n\n"))
		flusher.Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := NewRoot()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"--url", srv.URL, "--events-token", "tok", "events"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	errCh := make(chan error, 1)
	go func() {
		errCh <- cmd.Execute()
	}()

	select {
	case <-started:
	case err := <-errCh:
		t.Fatalf("events exited before the stream started: %v\n%s", err, out.String())
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the event stream")
	}
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("cancel: %v\n%s", err, out.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for events to stop")
	}
}

func TestCACertFlags(t *testing.T) {
	_, err := exec(t, "--url", "https://example.com", "--insecure", "--cacert", "x", "status")
	if err == nil || !strings.Contains(err.Error(), "cannot be used") {
		t.Fatalf("insecure+cacert: %v", err)
	}
	missing := filepath.Join(t.TempDir(), "missing.pem")
	_, err = exec(t, "--url", "https://example.com", "--cacert", missing, "status")
	if err == nil || !strings.Contains(err.Error(), "reading CA file") {
		t.Fatalf("missing CA: %v", err)
	}
}

func TestTargetFileModeWarning(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, config.ConfigFileName)
	content := "[[target]]\nname = \"prod\"\nurl = \"http://127.0.0.1:1\"\ntoken = \"from-config\"\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WATCHTOWER_CLI_CONFIG", path)
	out, err := exec(t, "--target", "prod", "status")
	if err == nil {
		t.Fatal("expected connection error")
	}
	if strings.Contains(out, "readable by group") || strings.Contains(err.Error(), "readable by group") {
		t.Errorf("permission notice leaked into command output: %s\nerr: %v", out, err)
	}
}

func TestTargetsCommand(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, config.ConfigFileName)
	content := "[[target]]\nname = \"prod\"\nurl = \"http://127.0.0.1:1\"\ntoken = \"secret\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WATCHTOWER_CLI_CONFIG", path)

	out, err := exec(t, "targets")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "readable by group") || !strings.Contains(out, "prod") {
		t.Fatalf("output: %s", out)
	}
	if strings.Contains(out, "secret") || strings.Contains(out, "127.0.0.1") {
		t.Fatalf("target secrets leaked: %s", out)
	}

	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err = exec(t, "targets")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "readable by group") {
		t.Fatalf("output: %s", out)
	}
	if !strings.Contains(out, "prod") {
		t.Fatalf("output: %s", out)
	}
}
