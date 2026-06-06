package pxe

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestAggregateOK(t *testing.T) {
	tests := []struct {
		name   string
		checks []Check
		want   bool
	}{
		{"empty is ok", nil, true},
		{"all ok", []Check{{OK: true}, {OK: true}}, true},
		{"one fails", []Check{{OK: true}, {OK: false}, {OK: true}}, false},
		{"all fail", []Check{{OK: false}, {OK: false}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := aggregateOK(tt.checks); got != tt.want {
				t.Fatalf("aggregateOK = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGatewayCollisionCheck(t *testing.T) {
	tests := []struct {
		name       string
		candidates []NetworkInfo
		filtered   []NetworkInfo
		wantOK     bool
		wantInDet  string // substring expected in Detail
	}{
		{
			name:       "no collision",
			candidates: []NetworkInfo{{Interface: "en0", IP: "192.168.68.55"}},
			filtered:   []NetworkInfo{{Interface: "en0", IP: "192.168.68.55"}},
			wantOK:     true,
			wantInDet:  "no interface advertises its router IP",
		},
		{
			name:       "collision but other viable NIC remains",
			candidates: []NetworkInfo{{Interface: "en0", IP: "192.168.68.1"}, {Interface: "en1", IP: "192.168.68.55"}},
			filtered:   []NetworkInfo{{Interface: "en1", IP: "192.168.68.55"}},
			wantOK:     true,
			wantInDet:  "192.168.68.1",
		},
		{
			name:       "collision leaves zero viable NICs",
			candidates: []NetworkInfo{{Interface: "en0", IP: "192.168.68.1"}},
			filtered:   nil,
			wantOK:     false,
			wantInDet:  "no viable NIC remains",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gatewayCollisionCheck(tt.candidates, tt.filtered)
			if c.Name != "gateway-collision" {
				t.Fatalf("Name = %q, want gateway-collision", c.Name)
			}
			if c.OK != tt.wantOK {
				t.Fatalf("OK = %v, want %v (detail=%q)", c.OK, tt.wantOK, c.Detail)
			}
			if tt.wantInDet != "" && !contains(c.Detail, tt.wantInDet) {
				t.Fatalf("Detail = %q, want substring %q", c.Detail, tt.wantInDet)
			}
			if !tt.wantOK && c.Hint == "" {
				t.Fatalf("failing collision check should carry a hint")
			}
		})
	}
}

func TestHTTPSelfTestFetch(t *testing.T) {
	dir := t.TempDir()
	want := []byte("#!ipxe\nchain http://example/boot\n")
	if err := os.WriteFile(filepath.Join(dir, "boot.ipxe"), want, 0o644); err != nil {
		t.Fatalf("write fake boot.ipxe: %v", err)
	}
	code, body, err := httpSelfTestFetch(dir)
	if err != nil {
		t.Fatalf("httpSelfTestFetch error: %v", err)
	}
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if len(body) == 0 {
		t.Fatalf("body is empty")
	}
	if string(body) != string(want) {
		t.Fatalf("body = %q, want %q", body, want)
	}
}

func TestHTTPSelfTestCheckMissingAsset(t *testing.T) {
	dir := t.TempDir() // no boot.ipxe written
	c := httpSelfTestCheck(dir)
	if c.OK {
		t.Fatalf("expected self-test to fail when boot.ipxe is absent; detail=%q", c.Detail)
	}
	if c.Hint == "" {
		t.Fatalf("failing self-test should carry a hint")
	}
}

func TestHTTPSelfTestCheckOK(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "boot.ipxe"), []byte("#!ipxe\n"), 0o644); err != nil {
		t.Fatalf("write fake boot.ipxe: %v", err)
	}
	c := httpSelfTestCheck(dir)
	if !c.OK {
		t.Fatalf("expected self-test OK; detail=%q", c.Detail)
	}
}

// contains is a tiny substring helper to keep the test free of extra imports.
func contains(s, sub string) bool {
	if sub == "" {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
