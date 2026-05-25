package secrets

import (
	"fmt"

	"github.com/yurifrl/nostos/internal/config"
)

// BuildBackends returns the per-scheme Backend map derived from a Config.
// `env` and `file` are always available; the primary backend (op/sops) is
// chosen based on `secrets.backend`. The optional `tailscale` block, if
// present, registers the `tailscale://` scheme and chains its credential
// refs through the other backends (op:// resolves via the onepassword
// backend, etc).
func BuildBackends(cfg *config.Config) (map[string]Backend, error) {
	backends := map[string]Backend{
		"env":  EnvBackend{},
		"file": FileBackend{},
	}

	switch cfg.Secrets.Backend {
	case "onepassword":
		if cfg.Secrets.Onepassword == nil {
			return nil, fmt.Errorf("secrets.backend=onepassword but no onepassword block present")
		}
		backends["op"] = NewOnePassword(cfg.Secrets.Onepassword.Account)
	case "sops":
		age := ""
		if cfg.Secrets.Sops != nil {
			age = cfg.Secrets.Sops.AgeKeyFile
		}
		backends["sops"] = NewSops(age)
	case "env", "file":
		// already registered
	default:
		return nil, fmt.Errorf("unknown secrets backend: %s", cfg.Secrets.Backend)
	}

	if cfg.Secrets.Tailscale != nil {
		ts := cfg.Secrets.Tailscale
		resolver := RefResolverFunc(func(ref string) (string, error) {
			return ResolveRefVia(backends, ref)
		})
		backends["tailscale"] = NewTailscale(TailscaleConfig{
			OAuthClientIDRef:     ts.OAuthClientIDRef.String(),
			OAuthClientSecretRef: ts.OAuthClientSecretRef.String(),
			Tags:                 append([]string{}, ts.Tags...),
			ExpirySeconds:        ts.ExpirySeconds,
			Reusable:             ts.Reusable,
			Ephemeral:            ts.Ephemeral,
			Preauthorized:        ts.Preauthorized,
			Description:          ts.Description,
			Tailnet:              ts.Tailnet,
		}, resolver, "")
	}

	return backends, nil
}

// ResolveRefVia is the chained-resolve helper: given an arbitrary ref like
// op://vault/item/field or file:///path, dispatch to the matching backend.
// Returns a clear error when the scheme is not registered.
func ResolveRefVia(backends map[string]Backend, ref string) (string, error) {
	if ref == "" {
		return "", fmt.Errorf("empty ref")
	}
	for scheme, b := range backends {
		if len(ref) > len(scheme)+3 && ref[:len(scheme)+3] == scheme+"://" {
			return b.Resolve(ref)
		}
	}
	return "", fmt.Errorf("no backend registered for ref %q", ref)
}

// ValidateBackends runs Validate() on each registered backend, returning the
// first failure. Callers run this before any render to catch auth/install
// issues up front.
func ValidateBackends(backends map[string]Backend) error {
	for _, b := range backends {
		if err := b.Validate(); err != nil {
			return err
		}
	}
	return nil
}
