// Package config parses and validates the consumer's config.yaml.
package config

import (
	"fmt"
	"net"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v3"
)

// Cluster holds cluster-level settings.
type Cluster struct {
	Name         string `yaml:"name"          validate:"required"`
	Endpoint     string `yaml:"endpoint"      validate:"required,startswith=https://"`
	TalosVersion string `yaml:"talos_version" validate:"required"`
	SchematicID  string `yaml:"schematic_id"  validate:"required,len=64"`
	// ImageDigests pins sha256 of factory.talos.dev image artifacts.
	// Key format: "<schematic>/<version>/<arch>". Value: "sha256:<hex>".
	ImageDigests map[string]string `yaml:"image_digests,omitempty"`
}

// OnepasswordConfig is populated when secrets.backend == "onepassword".
type OnepasswordConfig struct {
	Account string `yaml:"account" validate:"required"`
	Vault   string `yaml:"vault"   validate:"required"`
}

// SopsConfig is populated when secrets.backend == "sops".
type SopsConfig struct {
	AgeKeyFile string `yaml:"age_key_file,omitempty"`
}

// TailscaleConfig is populated when the operator wants the `tailscale://`
// URI scheme registered. Optional: when absent, templates that reference
// tailscale:// fail loud at render time.
type TailscaleConfig struct {
	OAuthClientIDRef     Ref      `yaml:"oauth_client_id_ref"     validate:"required"`
	OAuthClientSecretRef Ref      `yaml:"oauth_client_secret_ref" validate:"required"`
	Tags                 []string `yaml:"tags,omitempty"`
	ExpirySeconds        int      `yaml:"expiry,omitempty"`
	Reusable             bool     `yaml:"reusable,omitempty"`
	Ephemeral            bool     `yaml:"ephemeral,omitempty"`
	Preauthorized        bool     `yaml:"preauthorized,omitempty"`
	Description          string   `yaml:"description,omitempty"`
	Tailnet              string   `yaml:"tailnet,omitempty"`
}

// Secrets selects the active backend for URI resolution.
type Secrets struct {
	Backend     string             `yaml:"backend" validate:"required,oneof=onepassword sops env file"`
	Onepassword *OnepasswordConfig `yaml:"onepassword,omitempty"`
	Sops        *SopsConfig        `yaml:"sops,omitempty"`
	Tailscale   *TailscaleConfig   `yaml:"tailscale,omitempty"`
}

// Ref is a typed-string for secret references. Allowed schemes are
// op://, sops://, file://. env:// is rejected for credential refs.
type Ref string

// UnmarshalYAML enforces the ref scheme allowlist.
func (r *Ref) UnmarshalYAML(node *yaml.Node) error {
	var s string
	if err := node.Decode(&s); err != nil {
		return err
	}
	if s == "" {
		*r = ""
		return nil
	}
	switch {
	case strings.HasPrefix(s, "op://"),
		strings.HasPrefix(s, "sops://"),
		strings.HasPrefix(s, "file://"):
		*r = Ref(s)
		return nil
	case strings.HasPrefix(s, "env://"):
		return fmt.Errorf("env:// scheme not allowed for credential refs (got %q)", s)
	default:
		return fmt.Errorf("ref %q: must start with op://, sops://, or file://", s)
	}
}

// String returns the raw URI.
func (r Ref) String() string { return string(r) }

// TPIBoot describes Turing-Pi BMC settings for a node.
type TPIBoot struct {
	Host            string `yaml:"host"             validate:"required"`
	Slot            int    `yaml:"slot"             validate:"required,min=1,max=4"`
	UsernameRef     Ref    `yaml:"username_ref,omitempty"`
	PasswordRef     Ref    `yaml:"password_ref,omitempty"`
	IdentityFileRef Ref    `yaml:"identity_file_ref,omitempty"`
}

// Boot selects the install method for a node. Default Method is "pxe".
type Boot struct {
	Method string   `yaml:"method,omitempty" validate:"omitempty,oneof=pxe tpi"`
	TPI    *TPIBoot `yaml:"tpi,omitempty"`
}

// Node is one declared bare-metal or VM node.
type Node struct {
	MAC         string `yaml:"mac,omitempty" validate:"omitempty,mac"`
	IP          string `yaml:"ip"           validate:"required,ip4_addr"`
	Role        string `yaml:"role"         validate:"required,oneof=controlplane worker"`
	Arch        string `yaml:"arch"         validate:"required,oneof=amd64 arm64"`
	InstallDisk string `yaml:"install_disk" validate:"required,startswith=/dev/"`
	Template    string `yaml:"template"     validate:"required"`
	Boot        Boot   `yaml:"boot,omitempty"`
	// SchematicID overrides Cluster.SchematicID for this node when set.
	// Required for SBCs that need a different overlay than the cluster default
	// (e.g. Turing RK1 needs siderolabs/sbc-rockchip overlay; x86 nodes don't).
	SchematicID string `yaml:"schematic_id,omitempty" validate:"omitempty,len=64"`
}

// EffectiveSchematic returns the node's SchematicID when set, otherwise the
// cluster default. Centralizes the override rule so callers don't pick wrong.
func (n Node) EffectiveSchematic(cluster Cluster) string {
	if n.SchematicID != "" {
		return n.SchematicID
	}
	return cluster.SchematicID
}

// MACHyphen returns the MAC in iPXE ${mac:hexhyp} form: d0-94-66-d9-eb-a5.
func (n Node) MACHyphen() string {
	return strings.ReplaceAll(strings.ToLower(n.MAC), ":", "-")
}

// Config is the root document parsed from config.yaml.
type Config struct {
	Cluster Cluster         `yaml:"cluster" validate:"required"`
	Secrets Secrets         `yaml:"secrets" validate:"required"`
	Nodes   map[string]Node `yaml:"nodes,omitempty" validate:"dive"`
}

var nodeNameRE = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Load reads, parses, and validates a config.yaml. Returns a human-readable
// error on any validation failure.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, fmt.Errorf("%s is empty", path)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	// Normalize before validation: lowercase MAC, strip surrounding whitespace,
	// default Boot.Method to "pxe" when omitted.
	for name, node := range cfg.Nodes {
		node.MAC = strings.ToLower(strings.TrimSpace(node.MAC))
		if node.Boot.Method == "" {
			node.Boot.Method = "pxe"
		}
		cfg.Nodes[name] = node
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Validate runs schema + semantic checks on an already-unmarshaled Config.
func (c *Config) Validate() error {
	v := validator.New(validator.WithRequiredStructEnabled())

	if err := v.Struct(c); err != nil {
		return translate(err)
	}

	// Backend-specific config required.
	if c.Secrets.Backend == "onepassword" && c.Secrets.Onepassword == nil {
		return fmt.Errorf("secrets.backend=onepassword requires secrets.onepassword block")
	}

	// Node names: kebab-case.
	for name := range c.Nodes {
		if !nodeNameRE.MatchString(name) {
			return fmt.Errorf(
				"invalid node name %q: must start with a lowercase letter and contain only lowercase letters, digits, and hyphens",
				name,
			)
		}
	}

	// No duplicate MACs across nodes (empty MACs ignored).
	macToNames := map[string][]string{}
	for name, node := range c.Nodes {
		if node.MAC == "" {
			continue
		}
		macToNames[node.MAC] = append(macToNames[node.MAC], name)
	}
	var dupes []string
	for mac, names := range macToNames {
		if len(names) > 1 {
			sort.Strings(names)
			dupes = append(dupes, fmt.Sprintf("  %s: %s", mac, strings.Join(names, ", ")))
		}
	}
	if len(dupes) > 0 {
		sort.Strings(dupes)
		return fmt.Errorf("duplicate MAC addresses across nodes:\n%s", strings.Join(dupes, "\n"))
	}

	// IPv4 sanity is already covered by validator, but parse once to catch weird cases.
	for name, node := range c.Nodes {
		if net.ParseIP(node.IP) == nil {
			return fmt.Errorf("node %s has invalid IP %q", name, node.IP)
		}
	}

	// Boot-method-specific validation.
	type hostSlot struct {
		host string
		slot int
	}
	hostSlotToNames := map[hostSlot][]string{}
	for name, node := range c.Nodes {
		switch node.Boot.Method {
		case "", "pxe":
			// no extra validation
		case "tpi":
			if node.Boot.TPI == nil {
				return fmt.Errorf("node %s: boot.method=tpi requires boot.tpi block", name)
			}
			tpi := node.Boot.TPI
			// Credential refs are all optional; when absent the tpi provider
			// falls back to the tpi CLI's cached token / interactive prompt.
			key := hostSlot{tpi.Host, tpi.Slot}
			hostSlotToNames[key] = append(hostSlotToNames[key], name)
		}
	}

	// PXE method requires MAC (for iPXE matching). tpi method does not.
	for name, node := range c.Nodes {
		method := node.Boot.Method
		if method == "" {
			method = "pxe"
		}
		if method == "pxe" && node.MAC == "" {
			return fmt.Errorf("node %s: boot.method=pxe requires mac", name)
		}
	}
	var collisions []string
	for key, names := range hostSlotToNames {
		if len(names) > 1 {
			sort.Strings(names)
			collisions = append(collisions, fmt.Sprintf("  %s slot %d: %s", key.host, key.slot, strings.Join(names, ", ")))
		}
	}
	if len(collisions) > 0 {
		sort.Strings(collisions)
		return fmt.Errorf("duplicate (host, slot) across tpi-method nodes:\n%s", strings.Join(collisions, "\n"))
	}

	return nil
}

// translate converts validator's errors into a single human-readable message.
func translate(err error) error {
	if err == nil {
		return nil
	}
	valErrs, ok := err.(validator.ValidationErrors)
	if !ok {
		return err
	}
	msgs := make([]string, 0, len(valErrs))
	for _, e := range valErrs {
		msgs = append(msgs, fmt.Sprintf("  %s: %s", e.Namespace(), describeRule(e)))
	}
	return fmt.Errorf("validation failed:\n%s", strings.Join(msgs, "\n"))
}

func describeRule(e validator.FieldError) string {
	switch e.Tag() {
	case "required":
		return "is required"
	case "mac":
		return fmt.Sprintf("must be a MAC address (got %q)", e.Value())
	case "ip4_addr":
		return fmt.Sprintf("must be an IPv4 address (got %q)", e.Value())
	case "oneof":
		return fmt.Sprintf("must be one of %s (got %q)", e.Param(), e.Value())
	case "startswith":
		return fmt.Sprintf("must start with %q (got %q)", e.Param(), e.Value())
	case "len":
		return fmt.Sprintf("must be exactly %s characters (got %d)", e.Param(), len(fmt.Sprint(e.Value())))
	default:
		return fmt.Sprintf("failed %s rule", e.Tag())
	}
}
