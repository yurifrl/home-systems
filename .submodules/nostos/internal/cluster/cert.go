package cluster

import (
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"

	"github.com/yurifrl/nostos/internal/config"
	"github.com/yurifrl/nostos/internal/paths"
)

// TODO: native admin cert regeneration (crypto/ed25519 + crypto/x509 with
// Talos custom extension OID 1.3.6.1.4.1.58107.1.1 for os:admin role).
// This is a substantial piece; v0.1 ships with a placeholder that reads an
// existing valid talosconfig from elsewhere or returns a clear error.

// RefreshAdminCert is not yet wired. Documented limitation for v0.1.
func RefreshAdminCert(cfg *config.Config, p paths.Paths, node config.Node, hours int) error {
	return fmt.Errorf("admin-cert regeneration is not implemented in v0.1 of the Go rewrite; " +
		"copy an existing talosconfig to %s, or use the archived Python prototype (`git checkout python`) " +
		"which implements this via `talosctl gen` shell-out", p.Talosconfig())
}

// ExtractCAFromRenderedConfig reads the base64-decoded machine.ca (cert + key)
// from a rendered machineconfig. Exported for use by a future native cert-gen
// implementation.
func ExtractCAFromRenderedConfig(path string) (certPEM, keyPEM []byte, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var doc struct {
		Machine struct {
			CA struct {
				Crt string `yaml:"crt"`
				Key string `yaml:"key"`
			} `yaml:"ca"`
		} `yaml:"machine"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, nil, err
	}
	if doc.Machine.CA.Crt == "" || doc.Machine.CA.Key == "" {
		return nil, nil, fmt.Errorf("no machine.ca block in %s", path)
	}
	certPEM, err = base64.StdEncoding.DecodeString(doc.Machine.CA.Crt)
	if err != nil {
		return nil, nil, fmt.Errorf("decode ca.crt: %w", err)
	}
	keyPEM, err = base64.StdEncoding.DecodeString(doc.Machine.CA.Key)
	if err != nil {
		return nil, nil, fmt.Errorf("decode ca.key: %w", err)
	}
	// sanity: certPEM must be PEM
	if b, _ := pem.Decode(certPEM); b == nil {
		return nil, nil, fmt.Errorf("ca.crt is not PEM")
	}
	return certPEM, keyPEM, nil
}
