package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const UserAgent = "watchtower-cli/0.2.0"

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func NewClient(baseURL, token string, timeout time.Duration, insecure bool) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if insecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTP:    &http.Client{Timeout: timeout, Transport: transport},
	}
}

// WithToken returns a shallow copy that authenticates with token.
func (c *Client) WithToken(token string) *Client {
	out := *c
	out.Token = token
	return &out
}

// WithTimeout returns a shallow copy whose HTTP client uses timeout.
// Zero means no client timeout.
func (c *Client) WithTimeout(timeout time.Duration) *Client {
	httpClient := *c.HTTP
	httpClient.Timeout = timeout
	out := *c
	out.HTTP = &httpClient
	return &out
}

// WithRootCAs returns a shallow copy that trusts pool as its only CA set.
func (c *Client) WithRootCAs(pool *x509.CertPool) *Client {
	if pool == nil {
		return c
	}
	transport := c.HTTP.Transport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: pool}
	httpClient := *c.HTTP
	httpClient.Transport = transport
	out := *c
	out.HTTP = &httpClient
	return &out
}

// LoadRootCAs reads PEM certificates from path. The file replaces the system
// trust store for the client that uses it.
func LoadRootCAs(path string) (*x509.CertPool, error) {
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading CA file %s: %w", path, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemBytes) {
		return nil, fmt.Errorf("CA file %s contains no certificates", path)
	}
	return pool, nil
}

type Result struct {
	Status int
	Body   []byte
	Header http.Header
}

// Do performs a request and returns the HTTP status and raw body.
// Transport-level errors are returned; HTTP error statuses are not.
func (c *Client) Do(ctx context.Context, method, path string, q url.Values) (*Result, error) {
	endpoint := c.BaseURL + path
	if len(q) > 0 {
		endpoint += "?" + q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &Result{Status: resp.StatusCode, Header: resp.Header}, err
	}
	return &Result{Status: resp.StatusCode, Body: body, Header: resp.Header}, nil
}

// Stream returns the raw response body for streaming endpoints (e.g. events).
// The HTTP client timeout is disabled so the stream can stay open; cancel ctx to stop.
func (c *Client) Stream(ctx context.Context, method, path string) (int, io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, nil)
	if err != nil {
		return 0, nil, err
	}
	c.setHeaders(req)
	req.Header.Set("Accept", "text/event-stream")

	httpClient := *c.HTTP
	httpClient.Timeout = 0

	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, resp.Body, nil
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("User-Agent", UserAgent)
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
}

// ProbeReport describes a raw TCP connectivity check against the base URL.
type ProbeReport struct {
	URL             string   `json:"url"`
	Host            string   `json:"host"`
	Port            string   `json:"port"`
	Resolved        []string `json:"resolved"`
	ConnectErr      string   `json:"connect_error,omitempty"`
	Proxy           string   `json:"proxy,omitempty"`
	ProxyConfigured bool     `json:"proxy_configured"`
}

// Probe resolves the base URL host and attempts a TCP dial, reporting what was
// reached. It never sends the API token. The dial is direct TCP and does not
// use an HTTP proxy; Proxy is what the HTTP client would use.
func (c *Client) Probe(ctx context.Context, timeout time.Duration) (*ProbeReport, error) {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL %q: %w", c.BaseURL, err)
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}

	proxy := proxyFor(u)
	report := &ProbeReport{
		URL:             c.BaseURL,
		Host:            host,
		Port:            port,
		Proxy:           proxy,
		ProxyConfigured: proxy != "",
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		report.ConnectErr = fmt.Sprintf("DNS lookup failed: %v", err)
		return report, nil
	}
	report.Resolved = addrs

	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		report.ConnectErr = err.Error()
		return report, nil
	}
	conn.Close()

	return report, nil
}

func proxyFor(u *url.URL) string {
	req := &http.Request{URL: u}
	p, err := http.ProxyFromEnvironment(req)
	if err != nil || p == nil {
		return ""
	}
	return p.String()
}
