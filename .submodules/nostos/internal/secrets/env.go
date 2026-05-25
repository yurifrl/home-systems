package secrets

import (
	"fmt"
	"os"
	"strings"
)

// EnvBackend resolves env://VARNAME from the process environment.
type EnvBackend struct{}

func (EnvBackend) Scheme() string   { return "env" }
func (EnvBackend) Validate() error  { return nil }

func (EnvBackend) Resolve(uri string) (string, error) {
	if !strings.HasPrefix(uri, "env://") {
		return "", &ResolveError{URI: uri, Reason: "not an env:// URI"}
	}
	name := strings.TrimPrefix(uri, "env://")
	if name == "" {
		return "", &ResolveError{URI: uri, Reason: "empty variable name"}
	}
	val, ok := os.LookupEnv(name)
	if !ok {
		return "", &ResolveError{URI: uri, Reason: fmt.Sprintf("environment variable %q is not set", name)}
	}
	return val, nil
}
