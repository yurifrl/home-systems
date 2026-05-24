// Package redact provides a simple substring-replacement Scrubber and
// an EventEmitter wrapper that runs every Event.Message through it.
package redact

import (
	"sort"
	"strings"
	"sync"

	"github.com/yurifrl/nostos/internal/provisioner"
)

// Marker is the placeholder substituted for any matched secret value.
const Marker = "[REDACTED]"

// Scrubber maintains a set of secret strings and replaces every
// occurrence of any of them with Marker. It is safe for concurrent use.
type Scrubber struct {
	mu      sync.RWMutex
	secrets []string // sorted by length desc so longer matches win
}

// NewScrubber returns an empty Scrubber.
func NewScrubber() *Scrubber { return &Scrubber{} }

// AddAll adds every non-empty value to the secret table. Duplicates are
// ignored.
func (s *Scrubber) AddAll(values []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := make(map[string]struct{}, len(s.secrets)+len(values))
	for _, v := range s.secrets {
		seen[v] = struct{}{}
	}
	for _, v := range values {
		if v == "" {
			continue
		}
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		s.secrets = append(s.secrets, v)
	}
	sort.Slice(s.secrets, func(i, j int) bool {
		return len(s.secrets[i]) > len(s.secrets[j])
	})
}

// Scrub returns in with every registered secret value replaced by Marker.
// Replacement uses longest-first matching so that overlapping prefixes
// (e.g. "secret" and "secretX") both get redacted without exposing the
// shorter as a substring of the longer.
func (s *Scrubber) Scrub(in string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.secrets) == 0 || in == "" {
		return in
	}
	out := in
	for _, sec := range s.secrets {
		if sec == "" {
			continue
		}
		out = strings.ReplaceAll(out, sec, Marker)
	}
	return out
}

// WrapEmitter returns an EventEmitter that scrubs Event.Message before
// delegating to next. A nil scrubber returns next unchanged.
func WrapEmitter(next provisioner.EventEmitter, s *Scrubber) provisioner.EventEmitter {
	if next == nil {
		return func(provisioner.Event) {}
	}
	if s == nil {
		return next
	}
	return func(e provisioner.Event) {
		e.Message = s.Scrub(e.Message)
		next(e)
	}
}
