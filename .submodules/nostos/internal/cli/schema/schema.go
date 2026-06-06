// Package schema is the single source of truth for the AI-friendly CLI surface.
//
// Schema descriptors are built by walking the cobra command tree at runtime
// (so flags can never drift) and merging hand-authored metadata
// (description, destructive, idempotent, stdout_schema) keyed by method ID.
//
// Method ID convention: dot-separated cobra command path with the leading
// "nostos" stripped, e.g. `node.install`, `secrets.keys.list`, `schema`.
package schema

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/yurifrl/nostos/internal/cli/errs"
)

// Arg describes a positional argument.
type Arg struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description,omitempty"`
}

// Flag describes a cobra flag in a JSON-Schema-friendly shape.
type Flag struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Default     any      `json:"default,omitempty"`
	Values      []string `json:"values,omitempty"`
	Description string   `json:"description,omitempty"`
}

// Method is the schema descriptor returned by `nostos schema <method>`.
type Method struct {
	Method          string            `json:"method"`
	Description     string            `json:"description"`
	Args            []Arg             `json:"args"`
	Flags           []Flag            `json:"flags"`
	ExitCodes       map[string]string `json:"exit_codes"`
	StdoutSchema    map[string]any    `json:"stdout_schema,omitempty"`
	StderrFormat    string            `json:"stderr_format,omitempty"`
	Idempotent      bool              `json:"idempotent"`
	Destructive     bool              `json:"destructive"`
	RequiresConfirm bool              `json:"requires_confirm"`
}

// Meta is the hand-authored side-table keyed by method ID.
type Meta struct {
	Description     string
	Idempotent      bool
	Destructive     bool
	RequiresConfirm bool
	StdoutSchema    map[string]any
	StderrFormat    string
	Args            []Arg
}

// Registry holds the hand-authored metadata for every nostos command.
//
// Method IDs MUST match the dot-path computed by methodID().
var Registry = map[string]Meta{
	"init": {
		Description:  "Scaffold a new nostos project (config.yaml and templates/).",
		Idempotent:   true,
		StdoutSchema: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}},
		Args:         []Arg{{Name: "dir", Type: "string", Required: false, Description: "target directory (default: cwd)"}},
	},
	"node.list": {
		Description:  "List registered nodes with live reachability.",
		Idempotent:   true,
		StdoutSchema: nodeStatusSchema(),
	},
	"node.remove": {
		Description:     "Remove a node from config.yaml.",
		Destructive:     true,
		RequiresConfirm: true,
		Args:            []Arg{{Name: "name", Type: "string", Required: true, Description: "node name from config.yaml"}},
	},
	"node.install": {
		Description:     "End-to-end install for NAME (method-dispatched: pxe|tpi).",
		Destructive:     true,
		RequiresConfirm: true,
		Args:            []Arg{{Name: "name", Type: "string", Required: true, Description: "node name from config.yaml"}},
		StdoutSchema:    map[string]any{"type": "object", "properties": map[string]any{"events": map[string]any{"type": "array"}}},
	},
	"render": {
		Description: "Render NODE's machineconfig with secrets injected.",
		Idempotent:  true,
		Args:        []Arg{{Name: "node", Type: "string", Required: true}},
	},
	"apply": {
		Description:     "Render + apply machineconfig to a running NODE (config-only changes). --all applies to every node.",
		Destructive:     true,
		RequiresConfirm: false,
		Args:            []Arg{{Name: "node", Type: "string", Required: false, Description: "node name; omit when using --all"}},
		StdoutSchema:    map[string]any{"type": "object", "properties": map[string]any{"status": map[string]any{"type": "string"}, "mode": map[string]any{"type": "string"}, "nodes": map[string]any{"type": "array"}}},
	},
	"build": {
		Description: "Download Talos assets + build iPXE binary.",
		Idempotent:  true,
	},
	"flash": {
		Description:     "Build a flashable Talos disk image for NODE (downloads raw image, mints Tailscale key, renders config, optionally writes to a device).",
		Destructive:     true,  // when --device is used
		RequiresConfirm: false, // flash asks for --yes only when --device is set
		Args:            []Arg{{Name: "node", Type: "string", Required: true, Description: "node name from config.yaml"}},
		StdoutSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status": map[string]any{"type": "string"},
				"node":   map[string]any{"type": "string"},
				"image":  map[string]any{"type": "string"},
				"config": map[string]any{"type": "string"},
				"eeprom": map[string]any{"type": "string"},
			},
		},
	},
	"pxe": {
		Description: "Start PXE server (HTTP + dnsmasq) until Ctrl+C.",
	},
	"pxe.setup": {
		Description:     "Install a scoped NOPASSWD sudoers drop-in so `nostos pxe` runs dnsmasq without a password prompt.",
		Idempotent:      true,
		Destructive:     false,
		RequiresConfirm: false,
		StdoutSchema:    map[string]any{"type": "object", "properties": map[string]any{"status": map[string]any{"type": "string"}, "path": map[string]any{"type": "string"}, "dnsmasq": map[string]any{"type": "string"}}},
	},
	"pxe.doctor": {
		Description:  "Preflight self-diagnosis: viable interfaces, gateway collisions, HTTP port + self-test fetch, dnsmasq + sudoers readiness.",
		Idempotent:   true,
		Destructive:  false,
		StdoutSchema: map[string]any{"type": "object", "properties": map[string]any{"ok": map[string]any{"type": "boolean"}, "interfaces": map[string]any{"type": "array"}, "checks": map[string]any{"type": "array"}}},
	},
	"pxe.status": {
		Description:  "Show per-node PXE boot lifecycle state derived from the event stream.",
		Idempotent:   true,
		StdoutSchema: map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{"mac": map[string]any{"type": "string"}, "ip": map[string]any{"type": "string"}, "interface": map[string]any{"type": "string"}, "phase": map[string]any{"type": "string"}, "known": map[string]any{"type": "boolean"}, "enrollable": map[string]any{"type": "boolean"}}}},
	},
	"status": {
		Description:  "Show per-node reachability + Talos version.",
		Idempotent:   true,
		StdoutSchema: nodeStatusSchema(),
	},
	"wipe": {
		Description:     "Queue a one-shot disk wipe for NODE on its next PXE boot.",
		Destructive:     true,
		RequiresConfirm: false,
		Args:            []Arg{{Name: "node", Type: "string", Required: true}},
	},
	"bootstrap": {
		Description:     "Bootstrap etcd on NODE (first controlplane only).",
		Destructive:     true,
		RequiresConfirm: false,
		Args:            []Arg{{Name: "node", Type: "string", Required: true}},
	},
	"up": {
		Description: "[deprecated] alias for `nostos node install`.",
		Destructive: true,
		Args:        []Arg{{Name: "node", Type: "string", Required: true}},
	},
	"kubeconfig": {
		Description: "Refresh kubeconfig from a running controlplane.",
		Idempotent:  true,
		Args:        []Arg{{Name: "node", Type: "string", Required: false}},
	},
	"nuke": {
		Description:     "Remove nostos runtime cache entirely (regenerable from config.yaml).",
		Destructive:     true,
		RequiresConfirm: true,
	},
	"config": {Description: "Config subcommands."},
	"config.refresh": {
		Description: "Mint a fresh admin talosconfig from the cluster CA (offline; no cluster mutation).",
		Idempotent:  true,
	},
	"secrets": {Description: "Inspect and validate secret backends."},
	"secrets.list": {
		Description: "List configured secret backends with Validate() status.",
		Idempotent:  true,
	},
	"secrets.test": {
		Description: "Run Validate() against one (or all) backends.",
		Idempotent:  true,
		Args:        []Arg{{Name: "scheme", Type: "string", Required: false}},
	},
	"secrets.keys": {Description: "Tailscale auth-key inspection."},
	"secrets.keys.list": {
		Description: "List Tailscale auth keys.",
		Idempotent:  true,
	},
	"secrets.keys.revoke": {
		Description:     "Delete a Tailscale auth key by id.",
		Destructive:     true,
		RequiresConfirm: false,
		Args:            []Arg{{Name: "key_id", Type: "string", Required: true}},
	},
	"cluster": {Description: "Cluster-level operations."},
	"cluster.cleanup": {
		Description:     "Reconcile k8s + Tailscale state with nostos config.",
		Destructive:     true,
		RequiresConfirm: true,
	},
	"node": {Description: "Manage node registrations."},
	"schema": {
		Description: "Print machine-readable schema descriptors for nostos commands.",
		Idempotent:  true,
		Args:        []Arg{{Name: "method", Type: "string", Required: false}},
	},
	"mcp": {
		Description: "Run JSON-RPC MCP server over stdio (one tool per cobra command).",
	},
	"dashboard": {
		Description:  "Live single-pane TUI for cluster + nodes + ArgoCD apps.",
		Idempotent:   true,
		StdoutSchema: map[string]any{"$ref": "dashboard.snapshot"},
	},
}

func nodeStatusSchema() map[string]any {
	return map[string]any{
		"type": "array",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":    map[string]any{"type": "string"},
				"ip":      map[string]any{"type": "string"},
				"role":    map[string]any{"type": "string"},
				"ping":    map[string]any{"type": "string"},
				"apid":    map[string]any{"type": "string"},
				"version": map[string]any{"type": "string"},
			},
		},
	}
}

// MethodID returns the dot-separated path for c with the root name dropped.
func MethodID(c *cobra.Command) string {
	parts := []string{}
	for cur := c; cur != nil; cur = cur.Parent() {
		if cur.Parent() == nil {
			break // skip root
		}
		name := strings.SplitN(cur.Use, " ", 2)[0]
		parts = append([]string{name}, parts...)
	}
	return strings.Join(parts, ".")
}

// Build constructs a Method descriptor for c, merging cobra-reflected info
// with hand-authored metadata from Registry.
func Build(c *cobra.Command) Method {
	id := MethodID(c)
	m := Method{
		Method:    id,
		ExitCodes: errs.Catalog(),
		Args:      []Arg{},
		Flags:     []Flag{},
	}
	if meta, ok := Registry[id]; ok {
		m.Description = meta.Description
		m.Idempotent = meta.Idempotent
		m.Destructive = meta.Destructive
		m.RequiresConfirm = meta.RequiresConfirm
		m.StdoutSchema = meta.StdoutSchema
		m.StderrFormat = meta.StderrFormat
		if len(meta.Args) > 0 {
			m.Args = meta.Args
		}
	}
	if m.Description == "" {
		m.Description = c.Short
	}
	if m.StderrFormat == "" {
		m.StderrFormat = "human-readable hints"
	}
	c.Flags().VisitAll(func(f *pflag.Flag) {
		m.Flags = append(m.Flags, flagOf(f))
	})
	c.InheritedFlags().VisitAll(func(f *pflag.Flag) {
		m.Flags = append(m.Flags, flagOf(f))
	})
	sort.Slice(m.Flags, func(i, j int) bool { return m.Flags[i].Name < m.Flags[j].Name })
	return m
}

func flagOf(f *pflag.Flag) Flag {
	out := Flag{
		Name:        f.Name,
		Type:        f.Value.Type(),
		Description: f.Usage,
	}
	if out.Type == "string" || out.Type == "bool" || out.Type == "int" || out.Type == "duration" {
		out.Default = f.DefValue
	} else {
		out.Default = f.DefValue
	}
	if f.Name == "output" {
		out.Type = "enum"
		out.Values = []string{"text", "json"}
	}
	return out
}

// All walks the cobra tree and returns every leaf-or-internal command's Method.
// The map is keyed by method ID.
func All(root *cobra.Command) map[string]Method {
	out := map[string]Method{}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if c.Hidden || c.Name() == "help" {
			return
		}
		if c != root {
			out[MethodID(c)] = Build(c)
		}
		for _, child := range c.Commands() {
			walk(child)
		}
	}
	walk(root)
	return out
}

// IDs returns sorted method IDs from All.
func IDs(root *cobra.Command) []string {
	all := All(root)
	out := make([]string, 0, len(all))
	for k := range all {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
