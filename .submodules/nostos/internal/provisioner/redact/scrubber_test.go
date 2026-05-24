package redact_test

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/yurifrl/nostos/internal/provisioner"
	"github.com/yurifrl/nostos/internal/provisioner/redact"
)

func TestScrubBasic(t *testing.T) {
	s := redact.NewScrubber()
	s.AddAll([]string{"hunter2"})
	got := s.Scrub("login: alice / hunter2 OK")
	if strings.Contains(got, "hunter2") {
		t.Fatalf("not scrubbed: %q", got)
	}
	if !strings.Contains(got, redact.Marker) {
		t.Fatalf("missing marker: %q", got)
	}
}

func TestScrubOverlappingPrefixes(t *testing.T) {
	s := redact.NewScrubber()
	s.AddAll([]string{"secret", "secretX"})
	for _, in := range []string{"secret", "secretX", "ZZsecretXyy", "asecretb"} {
		got := s.Scrub(in)
		if strings.Contains(got, "secret") {
			t.Errorf("input %q -> %q still contains 'secret'", in, got)
		}
	}
}

// Property test (table+random): for many random strings, after Scrub no
// secret t in T appears as a substring.
func TestScrubProperty(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	secrets := []string{"alpha", "alphabet", "S3CR3T", "p@ss", "longer-secret-1234"}
	s := redact.NewScrubber()
	s.AddAll(secrets)

	alphabet := []byte("abcdefghijklmnopqrstuvwxyzS3@-1234 ")

	for i := 0; i < 500; i++ {
		n := rng.Intn(120) + 1
		buf := make([]byte, n)
		for j := range buf {
			buf[j] = alphabet[rng.Intn(len(alphabet))]
		}
		// Splice random secrets in at random positions.
		for k := 0; k < rng.Intn(3); k++ {
			sec := secrets[rng.Intn(len(secrets))]
			pos := rng.Intn(len(buf) + 1)
			buf = append(buf[:pos], append([]byte(sec), buf[pos:]...)...)
		}
		out := s.Scrub(string(buf))
		for _, sec := range secrets {
			if strings.Contains(out, sec) {
				t.Fatalf("iter %d: secret %q leaked in output %q (input %q)",
					i, sec, out, string(buf))
			}
		}
	}
}

func TestWrapEmitterScrubsMessage(t *testing.T) {
	s := redact.NewScrubber()
	s.AddAll([]string{"S3CR3T-DO-NOT-LEAK"})

	var seen []provisioner.Event
	wrapped := redact.WrapEmitter(func(e provisioner.Event) { seen = append(seen, e) }, s)
	wrapped(provisioner.Event{Message: "tpi power: S3CR3T-DO-NOT-LEAK"})

	if len(seen) != 1 {
		t.Fatalf("emits=%d", len(seen))
	}
	if strings.Contains(seen[0].Message, "S3CR3T") {
		t.Fatalf("leaked: %q", seen[0].Message)
	}
}

func TestWrapEmitterNilSafe(t *testing.T) {
	em := redact.WrapEmitter(nil, redact.NewScrubber())
	em(provisioner.Event{Message: "x"}) // must not panic
}
