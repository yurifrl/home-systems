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

// Secrets selects the active backend for URI resolution.
type Secrets struct {
	Backend     string             `yaml:"backend" validate:"required,oneof=onepassword sops env file"`
	Onepassword *OnepasswordConfig `yaml:"onepassword,omitempty"`
	Sops        *SopsConfig        `yaml:"sops,omitempty"`
}

// Node is one declared bare-metal or VM node.
type Node struct {
	MAC         string `yaml:"mac"          validate:"required,mac"`
	IP          string `yaml:"ip"           validate:"required,ip4_addr"`
	Role        string `yaml:"role"         validate:"required,oneof=controlplane worker"`
	Arch        string `yaml:"arch"         validate:"required,oneof=amd64 arm64"`
	InstallDisk string `yaml:"install_disk" validate:"required,startswith=/dev/"`
	Template    string `yaml:"template"     validate:"required"`
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

	// Normalize before validation: lowercase MAC and strip surrounding whitespace.
	for name, node := range cfg.Nodes {
		node.MAC = strings.ToLower(strings.TrimSpace(node.MAC))
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

	// No duplicate MACs across nodes.
	macToNames := map[string][]string{}
	for name, node := range c.Nodes {
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
