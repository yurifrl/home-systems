package secrets

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// FileBackend resolves file://<path> by reading the file contents (trimmed).
type FileBackend struct{}

func (FileBackend) Scheme() string  { return "file" }
func (FileBackend) Validate() error { return nil }

func (FileBackend) Resolve(uri string) (string, error) {
	if !strings.HasPrefix(uri, "file://") {
		return "", &ResolveError{URI: uri, Reason: "not a file:// URI"}
	}
	u, err := url.Parse(uri)
	if err != nil {
		return "", &ResolveError{URI: uri, Reason: fmt.Sprintf("parse: %v", err)}
	}
	path := u.Path
	if u.Host != "" && u.Host != "." {
		return "", &ResolveError{
			URI:    uri,
			Reason: fmt.Sprintf("file:// URIs must be local (got authority %q)", u.Host),
		}
	}
	if u.Host == "." {
		path = "." + path
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", &ResolveError{URI: uri, Reason: fmt.Sprintf("read %s: %v", path, err)}
	}
	return strings.TrimRight(string(data), " \t\r\n"), nil
}
