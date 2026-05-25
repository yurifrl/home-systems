package tpi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Typed BMC pre-flight errors. tpi.Provisioner.Preflight wraps these with
// provisioner-level context so the orchestrator surfaces the verbatim
// message instead of a kernel-level red herring like "Device not configured".
var (
	ErrBMCUnreachable       = errors.New("bmc unreachable")
	ErrBMCAuthFailed        = errors.New("bmc auth failed")
	ErrBMCAPIVersionTooOld  = errors.New("bmc api version too old")
	ErrBMCMalformedResponse = errors.New("bmc returned malformed response")
	ErrBMCUnauthorizedHost  = errors.New("bmc probe refused: host outside configured allow-list")
)

// MinBMCAPIVersion is the floor for /api/bmc/info; older Turing-Pi 2 firmware
// has a different shape and is not supported.
const MinBMCAPIVersion = "2.0.1"

// BMCInfo is the parsed /api/bmc/info payload. Field names mirror the JSON.
type BMCInfo struct {
	IP        string `json:"ip"`
	MAC       string `json:"mac"`
	API       string `json:"api"`
	Version   string `json:"version"`
	BuildTime string `json:"buildtime"`
}

// dialer + http seams (vars so tests can substitute).
var (
	bmcDial = func(network, addr string, d time.Duration) (net.Conn, error) {
		return net.DialTimeout(network, addr, d)
	}
	bmcHTTPClient = &http.Client{Timeout: 5 * time.Second}
)

// Per-host token bucket: 1 probe / sec, burst 3.
//
// Why per-host: a shared global limiter would let a single chatty host
// starve probes of every other BMC; per-host limits keep blast-radius small.
type bmcBucket struct {
	mu     sync.Mutex
	tokens float64
	last   time.Time
}

const (
	bmcRate  = 1.0 // tokens / sec
	bmcBurst = 3.0
)

var (
	bmcBuckets   = map[string]*bmcBucket{}
	bmcBucketsMu sync.Mutex
)

func bmcAcquire(ctx context.Context, host string) error {
	bmcBucketsMu.Lock()
	b, ok := bmcBuckets[host]
	if !ok {
		b = &bmcBucket{tokens: bmcBurst, last: time.Now()}
		bmcBuckets[host] = b
	}
	bmcBucketsMu.Unlock()

	for {
		b.mu.Lock()
		now := time.Now()
		elapsed := now.Sub(b.last).Seconds()
		b.tokens += elapsed * bmcRate
		if b.tokens > bmcBurst {
			b.tokens = bmcBurst
		}
		b.last = now
		if b.tokens >= 1.0 {
			b.tokens -= 1.0
			b.mu.Unlock()
			return nil
		}
		// time until 1 token is available
		need := (1.0 - b.tokens) / bmcRate
		b.mu.Unlock()
		wait := time.Duration(need * float64(time.Second))
		if wait <= 0 {
			wait = 10 * time.Millisecond
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}

// resetBMCBuckets is a test-only helper.
func resetBMCBuckets() {
	bmcBucketsMu.Lock()
	defer bmcBucketsMu.Unlock()
	bmcBuckets = map[string]*bmcBucket{}
}

// isRFC1918 reports whether host (literal IP) is in 10/8, 172.16/12, or 192.168/16.
func isRFC1918(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	v4 := ip.To4()
	if v4 == nil {
		return false
	}
	switch {
	case v4[0] == 10:
		return true
	case v4[0] == 172 && v4[1] >= 16 && v4[1] <= 31:
		return true
	case v4[0] == 192 && v4[1] == 168:
		return true
	}
	return false
}

// ProbeBMC performs a TCP-connect pre-flight (host:80, 3s) and an HTTP GET
// /api/bmc/info (2s) and returns parsed BMCInfo or one of the typed errors.
//
// configuredHost is the host nostos is currently driving (from
// node.Boot.TPI.Host). When host is RFC1918 and != configuredHost we
// refuse to probe to avoid LAN scanning fingerprint.
//
// Per-host rate limiting: 1 probe / sec, burst 3.
func ProbeBMC(ctx context.Context, host, configuredHost string, timeout time.Duration) (*BMCInfo, error) {
	if host == "" {
		return nil, fmt.Errorf("%w: empty host", ErrBMCUnreachable)
	}
	if isRFC1918(host) && configuredHost != "" && host != configuredHost {
		return nil, fmt.Errorf("%w: %s != configured %s", ErrBMCUnauthorizedHost, host, configuredHost)
	}
	if err := bmcAcquire(ctx, host); err != nil {
		return nil, err
	}

	// TCP pre-flight (3s).
	tcpDeadline := 3 * time.Second
	if timeout > 0 && timeout < tcpDeadline {
		tcpDeadline = timeout
	}
	conn, err := bmcDial("tcp", net.JoinHostPort(host, "80"), tcpDeadline)
	if err != nil {
		return nil, fmt.Errorf("%w: tcp %s:80: %v", ErrBMCUnreachable, host, err)
	}
	_ = conn.Close()

	// HTTP probe (2s).
	httpDeadline := 2 * time.Second
	if timeout > 0 && timeout < httpDeadline {
		httpDeadline = timeout
	}
	hctx, cancel := context.WithTimeout(ctx, httpDeadline)
	defer cancel()
	url := "http://" + net.JoinHostPort(host, "80") + "/api/bmc/info"
	req, err := http.NewRequestWithContext(hctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %v", ErrBMCMalformedResponse, err)
	}
	resp, err := bmcHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: GET %s: %v", ErrBMCUnreachable, url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	switch {
	case resp.StatusCode == 401:
		return nil, fmt.Errorf("%w: HTTP 401", ErrBMCAuthFailed)
	case resp.StatusCode == 403:
		return nil, fmt.Errorf("%w: HTTP 403", ErrBMCAuthFailed)
	case resp.StatusCode == 404:
		return nil, fmt.Errorf("%w: HTTP 404 (no /api/bmc/info — likely too-old firmware)", ErrBMCAPIVersionTooOld)
	case resp.StatusCode >= 500:
		return nil, fmt.Errorf("%w: HTTP %d: %s", ErrBMCUnreachable, resp.StatusCode, truncate(string(body), 200))
	case resp.StatusCode != 200:
		return nil, fmt.Errorf("%w: unexpected HTTP %d", ErrBMCMalformedResponse, resp.StatusCode)
	}

	info, err := parseBMCInfo(body)
	if err != nil {
		return nil, err
	}
	if !versionAtLeast(info.Version, MinBMCAPIVersion) {
		return info, fmt.Errorf("%w: %s < %s", ErrBMCAPIVersionTooOld, info.Version, MinBMCAPIVersion)
	}
	return info, nil
}

// parseBMCInfo accepts either a flat object or {response:[{...}]} (Turing-Pi 2 wraps it).
func parseBMCInfo(body []byte) (*BMCInfo, error) {
	body = []byte(strings.TrimSpace(string(body)))
	if len(body) == 0 {
		return nil, fmt.Errorf("%w: empty body", ErrBMCMalformedResponse)
	}
	// Try wrapper first.
	var wrap struct {
		Response []BMCInfo `json:"response"`
	}
	if err := json.Unmarshal(body, &wrap); err == nil && len(wrap.Response) > 0 && wrap.Response[0].Version != "" {
		bi := wrap.Response[0]
		return &bi, nil
	}
	var bi BMCInfo
	if err := json.Unmarshal(body, &bi); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBMCMalformedResponse, err)
	}
	if bi.Version == "" {
		return nil, fmt.Errorf("%w: missing version field", ErrBMCMalformedResponse)
	}
	return &bi, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// (silence unused import in some build matrices)
var _ = strconv.Itoa
