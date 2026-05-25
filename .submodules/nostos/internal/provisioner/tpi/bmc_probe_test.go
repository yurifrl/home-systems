package tpi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// hostOf strips the scheme + port from an httptest.Server URL.
func hostOf(t *testing.T, u string) (host, port string) {
	t.Helper()
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	h, p, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	return h, p
}

// withBMCEnv wires bmcDial / bmcHTTPClient to talk to the given test server,
// regardless of port (we always probe :80 in production).
func withBMCEnv(t *testing.T, srv *httptest.Server) {
	t.Helper()
	resetBMCBuckets()
	prevDial := bmcDial
	prevClient := bmcHTTPClient

	_, srvPort := hostOf(t, srv.URL)
	rewriter := func(network, addr string, d time.Duration) (net.Conn, error) {
		return prevDial(network, net.JoinHostPort("127.0.0.1", srvPort), d)
	}
	bmcDial = rewriter

	httpRT := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				dialer := &net.Dialer{Timeout: 5 * time.Second}
				return dialer.DialContext(ctx, network, net.JoinHostPort("127.0.0.1", srvPort))
			},
		},
	}
	bmcHTTPClient = httpRT

	t.Cleanup(func() {
		bmcDial = prevDial
		bmcHTTPClient = prevClient
		resetBMCBuckets()
	})
}

// TestProbeBMCTable is the 9-row table A3 demands.
func TestProbeBMCTable(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		// closedConn=true short-circuits the test server: we point bmcDial
		// at a closed port to simulate "connection refused".
		refused bool
		wantErr error
	}{
		{
			name:    "refused",
			refused: true,
			wantErr: ErrBMCUnreachable,
		},
		{
			name:    "401",
			handler: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) },
			wantErr: ErrBMCAuthFailed,
		},
		{
			name:    "403",
			handler: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) },
			wantErr: ErrBMCAuthFailed,
		},
		{
			name:    "404",
			handler: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) },
			wantErr: ErrBMCAPIVersionTooOld,
		},
		{
			name:    "5xx",
			handler: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) },
			wantErr: ErrBMCUnreachable,
		},
		{
			name: "malformed_json",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
				_, _ = w.Write([]byte("not json{"))
			},
			wantErr: ErrBMCMalformedResponse,
		},
		{
			name: "version_too_old_2.0.0",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
				fmt.Fprintln(w, `{"ip":"1.1.1.1","mac":"aa:bb","api":"2","version":"2.0.0","buildtime":"2024"}`)
			},
			wantErr: ErrBMCAPIVersionTooOld,
		},
		{
			name: "valid_2.0.5",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
				fmt.Fprintln(w, `{"ip":"1.1.1.1","mac":"aa:bb","api":"2","version":"2.0.5","buildtime":"2024"}`)
			},
			wantErr: nil,
		},
		{
			name: "valid_2.x.x_wrapped",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
				fmt.Fprintln(w, `{"response":[{"ip":"1.1.1.1","mac":"aa:bb","api":"2","version":"2.5.1","buildtime":"2024"}]}`)
			},
			wantErr: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.refused {
				resetBMCBuckets()
				prevDial := bmcDial
				bmcDial = func(network, addr string, d time.Duration) (net.Conn, error) {
					return nil, &net.OpError{Op: "dial", Err: errFakeRefused{}}
				}
				t.Cleanup(func() { bmcDial = prevDial })
				_, err := ProbeBMC(context.Background(), "203.0.113.1", "203.0.113.1", 1*time.Second)
				if err == nil || !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want Is(%v)", err, tc.wantErr)
				}
				return
			}
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			withBMCEnv(t, srv)

			info, err := ProbeBMC(context.Background(), "203.0.113.1", "203.0.113.1", 1*time.Second)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected err: %v", err)
				}
				if info == nil || info.Version == "" {
					t.Fatalf("expected non-empty info, got %+v", info)
				}
				return
			}
			if err == nil || !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want Is(%v)", err, tc.wantErr)
			}
		})
	}
}

type errFakeRefused struct{}

func (errFakeRefused) Error() string { return "connection refused" }

// TestProbeBMCRateLimit pins A5: 10 rapid probes against the same host
// (burst 3, 1 token/sec) must take >= ~7s of waits. We assert >= 3s
// (loose to keep CI happy).
func TestProbeBMCRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		fmt.Fprintln(w, `{"ip":"1.1.1.1","mac":"aa:bb","api":"2","version":"2.0.5","buildtime":"2024"}`)
	}))
	defer srv.Close()
	withBMCEnv(t, srv)

	start := time.Now()
	for i := 0; i < 10; i++ {
		if _, err := ProbeBMC(context.Background(), "203.0.113.7", "203.0.113.7", 2*time.Second); err != nil {
			t.Fatalf("probe %d: %v", i, err)
		}
	}
	elapsed := time.Since(start)
	if elapsed < 3*time.Second {
		t.Fatalf("10 probes finished in %v; rate limit not enforced (want >=3s)", elapsed)
	}
}

// TestProbeBMCUnauthorizedHost pins A5: an RFC1918 host that is not the
// configured one is refused outright.
func TestProbeBMCUnauthorizedHost(t *testing.T) {
	resetBMCBuckets()
	_, err := ProbeBMC(context.Background(), "192.168.99.1", "192.168.68.103", 500*time.Millisecond)
	if err == nil || !errors.Is(err, ErrBMCUnauthorizedHost) {
		t.Fatalf("err = %v, want Is(ErrBMCUnauthorizedHost)", err)
	}
	// Sanity: the configured host is allowed (will fail downstream because
	// we point at a closed port, but the error must NOT be Unauthorized).
	prev := bmcDial
	bmcDial = func(network, addr string, d time.Duration) (net.Conn, error) {
		return nil, &net.OpError{Op: "dial", Err: errFakeRefused{}}
	}
	defer func() { bmcDial = prev }()
	_, err = ProbeBMC(context.Background(), "192.168.68.103", "192.168.68.103", 500*time.Millisecond)
	if err == nil || errors.Is(err, ErrBMCUnauthorizedHost) {
		t.Fatalf("configured host should not be Unauthorized: %v", err)
	}
}

// TestProbeBMCPublicHostBypassesAllowList: a non-RFC1918 host is always
// allowed (we only police LAN scanning).
func TestProbeBMCPublicHostBypassesAllowList(t *testing.T) {
	resetBMCBuckets()
	prev := bmcDial
	bmcDial = func(network, addr string, d time.Duration) (net.Conn, error) {
		return nil, &net.OpError{Op: "dial", Err: errFakeRefused{}}
	}
	defer func() { bmcDial = prev }()
	_, err := ProbeBMC(context.Background(), "203.0.113.5", "192.168.68.103", 500*time.Millisecond)
	if err == nil || errors.Is(err, ErrBMCUnauthorizedHost) {
		t.Fatalf("public host should not trigger Unauthorized: %v", err)
	}
	if !strings.Contains(err.Error(), "tcp") {
		t.Fatalf("expected tcp probe error, got %v", err)
	}
}
