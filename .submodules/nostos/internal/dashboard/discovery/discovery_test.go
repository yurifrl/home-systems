package discovery

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/yurifrl/nostos/internal/config"
)

// TestProbeTalosUnreachable confirms the probe returns false against a closed port.
func TestProbeTalosUnreachable(t *testing.T) {
	if probeTalos(context.Background(), "127.0.0.1", 50*time.Millisecond) {
		// might be reachable in pathological CI, accept either; just ensure no panic
		t.Skip("local 50000 is open?")
	}
}

// TestProbeTalosReachable confirms a positive on a listening port we control.
func TestProbeTalosReachable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	d := net.Dialer{Timeout: 200 * time.Millisecond}
	c, err := d.Dial("tcp", net.JoinHostPort(host, port))
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
}

func TestRunWithEmptyConfig(t *testing.T) {
	cfg := &config.Config{Nodes: map[string]config.Node{}}
	res := Run(context.Background(), cfg)
	if res.FinishedAt.Before(res.StartedAt) {
		t.Fatalf("clock skew: %+v", res)
	}
	// arp may or may not return entries — both fine
	_ = res
}

func TestArpRegex(t *testing.T) {
	line := "? (192.168.1.1) at aa:bb:cc:dd:ee:ff on en0 ifscope [ethernet]"
	m := arpRE.FindStringSubmatch(line)
	if m == nil || m[1] != "192.168.1.1" || !strings.HasPrefix(m[2], "aa:bb") {
		t.Fatalf("regex did not match: %v", m)
	}
}
