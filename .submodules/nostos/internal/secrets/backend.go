// Package secrets resolves <scheme>://... URIs in templates via pluggable backends.
package secrets

import (
	"fmt"
	"regexp"
	"strings"
)

// Backend resolves a URI of a specific scheme into a secret value.
// Implementations must never log resolved values.
type Backend interface {
	Scheme() string
	Validate() error
	Resolve(uri string) (string, error)
}

// URIPattern matches `<scheme>://...` where the scheme has a known backend.
// We stop at whitespace, quotes, commas, and closing YAML delimiters.
var URIPattern = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*)://[^\s"',\]}]+`)

// ResolveError wraps a backend failure with the URI for troubleshooting.
// Never embeds the resolved value.
type ResolveError struct {
	URI    string
	Reason string
}

func (e *ResolveError) Error() string {
	return fmt.Sprintf("failed to resolve %s: %s", e.URI, e.Reason)
}

// FindURIs returns every URI in text whose scheme has a registered backend.
// URIs with unknown schemes (e.g. https://) are ignored.
func FindURIs(text string, known map[string]Backend) []string {
	var out []string
	for _, m := range URIPattern.FindAllStringSubmatchIndex(text, -1) {
		scheme := text[m[2]:m[3]]
		if _, ok := known[scheme]; ok {
			out = append(out, text[m[0]:m[1]])
		}
	}
	return out
}

// ResolveTemplate replaces every backend-owned URI in text with its resolved value.
// URIs whose scheme has no registered backend pass through unchanged (e.g. https://).
func ResolveTemplate(text string, backends map[string]Backend) (string, error) {
	var firstErr error
	out := URIPattern.ReplaceAllStringFunc(text, func(uri string) string {
		if firstErr != nil {
			return uri
		}
		idx := strings.Index(uri, "://")
		if idx < 0 {
			return uri
		}
		scheme := uri[:idx]
		backend, ok := backends[scheme]
		if !ok {
			return uri
		}
		val, err := backend.Resolve(uri)
		if err != nil {
			firstErr = err
			return uri
		}
		return val
	})
	if firstErr != nil {
		return "", firstErr
	}
	return out, nil
}
