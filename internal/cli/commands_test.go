package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestMissingCommand(t *testing.T) {
	_, err := exec(t, "--url", "http://127.0.0.1:1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "command required") {
		t.Errorf("error: %v", err)
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
