// Package tui implements the Bubble Tea v2 dashboard model.
//
// Per the spec, v0.3 is a read-only status board. Mutating action handlers
// (i, r, d, capital G) ship in v0.4.
package tui

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/yurifrl/nostos/internal/dashboard/playbooks"
	"github.com/yurifrl/nostos/internal/dashboard/snapshot"
)

// Model is the TUI state.
type Model struct {
	Snap        snapshot.Snapshot
	Cursor      int    // selected row in the inventory
	ShowHelp    bool
	ShowHidden  bool
	ShowDocs    bool
	DocsBody    string
	Filter      string
	Width       int
	Height      int
	asciiOnly   bool
}

// Init satisfies tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update satisfies tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
	case SnapshotMsg:
		m.Snap = snapshot.Snapshot(msg)
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// SnapshotMsg is sent by the refresh loop with a fresh snapshot.
type SnapshotMsg snapshot.Snapshot

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.ShowHelp {
		m.ShowHelp = false
		return m, nil
	}
	if m.ShowDocs {
		m.ShowDocs = false
		return m, nil
	}
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.ShowHelp = true
	case "H":
		m.ShowHidden = !m.ShowHidden
	case "j", "down":
		if m.Cursor < len(m.rows())-1 {
			m.Cursor++
		}
	case "k", "up":
		if m.Cursor > 0 {
			m.Cursor--
		}
	case "/":
		m.Filter = "" // simple toggle; full inline editor deferred
	case "s":
		m.ShowDocs = true
		m.DocsBody = m.renderPlaybookForCursor()
	case "n":
		// Emit a config patch on stdout (per spec — v0.3 emits, v0.4 writes).
		row := m.cursorRow()
		if row != nil && row.bucket == "unknown" {
			fmt.Fprintf(os.Stderr, "# %s — paste into config.yaml under nodes:\n", row.label)
			fmt.Fprintf(os.Stderr, "# <new-name>:\n#   ip: %s\n#   role: worker\n#   arch: amd64\n#   install_disk: /dev/nvme0n1\n#   template: dell01.yaml\n", row.label)
		}
	case "u":
		// preview-only: no shell-out from the TUI layer in v0.3
		m.ShowDocs = true
		m.DocsBody = "# upgrade preview\n\nrun `nostos cluster upgrade --dry-run` from your shell.\n"
	case "G":
		m.ShowDocs = true
		m.DocsBody = "# guide\n\nopen `docs/nostos-guide.md` in your editor.\n"
	}
	return m, nil
}

// View renders the dashboard.
func (m Model) View() tea.View {
	if m.ShowHelp {
		return wrap(m.helpPanel())
	}
	if m.ShowDocs {
		return wrap(m.DocsBody)
	}
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("\n")
	b.WriteString(m.summary())
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", w(m.Width, 80)))
	b.WriteString("\n")
	b.WriteString(m.inventory())
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", w(m.Width, 80)))
	b.WriteString("\n")
	b.WriteString(m.checksPanel())
	b.WriteString("\n")
	b.WriteString(m.footer())
	return wrap(b.String())
}

func wrap(s string) tea.View {
	v := tea.NewView(s)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// --- rendering helpers ---

func w(width, fallback int) int {
	if width <= 0 {
		return fallback
	}
	return width
}

func (m Model) header() string {
	state := string(m.Snap.AggregateState)
	if state == "" {
		state = "UNCONFIGURED"
	}
	badge := badgeStyle(state).Render("[" + state + "]")
	title := lipgloss.NewStyle().Bold(true).Render("[ " + m.Snap.Cluster.Name + " ]")
	return fmt.Sprintf("%s   %s   [q quit · ? help]", title, badge)
}

func (m Model) summary() string {
	c := m.Snap.Cluster
	parts := []string{
		fmt.Sprintf("Cluster: %d/%d reachable", c.NodesReady, c.NodesConfigured),
	}
	if !c.KubeconfigPresent {
		parts = append(parts, "kubeconfig not found — cluster checks degraded")
	}
	if m.Snap.Imperative != "" {
		parts = append(parts, m.Snap.Imperative)
	}
	return strings.Join(parts, " · ")
}

type rowSpec struct {
	bucket string
	label  string
	body   string
	sev    snapshot.Severity
}

func (m Model) rows() []rowSpec {
	var out []rowSpec
	for _, n := range m.Snap.Nodes {
		ts := n.Tailscale
		if ts == "" {
			ts = "—"
		}
		ver := n.Version
		if ver == "" {
			ver = "—"
		}
		out = append(out, rowSpec{
			bucket: "known",
			label:  n.Name,
			body: fmt.Sprintf("%-10s  %-16s  %-12s  %-7s  %s",
				n.Name, n.IP, n.Role, ver, n.Arch),
			sev: n.Severity,
		})
	}
	for _, d := range m.Snap.Discoveries {
		bucket := d.Bucket
		label := d.IP
		if d.Hostname != "" {
			label = d.Hostname
		}
		hint := "unknown — press n to name"
		if bucket == "orphan" {
			hint = "configured but unreachable"
		}
		out = append(out, rowSpec{
			bucket: bucket,
			label:  label,
			body:   fmt.Sprintf("%-10s  %-16s  %s", label, d.IP, hint),
			sev:    snapshot.SevWarn,
		})
	}
	return out
}

func (m Model) cursorRow() *rowSpec {
	rows := m.rows()
	if m.Cursor < 0 || m.Cursor >= len(rows) {
		return nil
	}
	return &rows[m.Cursor]
}

func (m Model) inventory() string {
	rows := m.rows()
	if len(rows) == 0 {
		return "  (no nodes configured — run `nostos init`)"
	}
	var b strings.Builder
	for i, r := range rows {
		sym := symbolFor(r.sev, r.bucket)
		cur := "  "
		if i == m.Cursor {
			cur = "▸ "
		}
		fmt.Fprintf(&b, "%s%s  %s\n", cur, sym, r.body)
	}
	return b.String()
}

func (m Model) checksPanel() string {
	if len(m.Snap.Checks) == 0 {
		return "  (no checks)"
	}
	var b strings.Builder
	b.WriteString("[ checks ]\n")
	for _, c := range m.Snap.Checks {
		fmt.Fprintf(&b, "  %s %s\n", symbolFor(c.Severity, ""), c.Message)
	}
	return b.String()
}

func (m Model) footer() string {
	row := m.cursorRow()
	keys := []string{"[?]help", "[H]hidden", "[u]upgrade", "[s]docs", "[/]search", "[q]quit"}
	if row != nil {
		switch row.bucket {
		case "unknown":
			keys = append([]string{"[n]ame"}, keys...)
		}
	}
	return strings.Join(keys, "  ")
}

func (m Model) helpPanel() string {
	return strings.Join([]string{
		"# nostos dashboard — keymap",
		"",
		"  q       quit",
		"  ?       this help",
		"  j / k   move cursor",
		"  H       toggle hidden devices",
		"  /       filter (toggle, full edit deferred)",
		"  s       open vendor playbook for selected row",
		"  u       preview `nostos cluster upgrade --dry-run`",
		"  G       open `docs/nostos-guide.md` (v0.3 read-only)",
		"  n       on an unknown row, emit config patch to stderr",
		"",
		"  (i, r, d action handlers ship in v0.4)",
	}, "\n")
}

// --- styles ---

func badgeStyle(state string) lipgloss.Style {
	style := lipgloss.NewStyle().Bold(true).Padding(0, 1)
	switch state {
	case string(snapshot.StateAllGreen):
		return style.Foreground(lipgloss.Color("#2ecc71"))
	case string(snapshot.StateDegraded):
		return style.Foreground(lipgloss.Color("#f1c40f"))
	case string(snapshot.StateBroken):
		return style.Foreground(lipgloss.Color("#e74c3c"))
	case string(snapshot.StateTransitioning):
		return style.Foreground(lipgloss.Color("#3498db"))
	}
	return style.Foreground(lipgloss.Color("#7f8c8d"))
}

// symbolFor maps severity to a short display token.
//
// Honors NO_COLOR and --ascii by switching to bracketed ASCII.
func symbolFor(sev snapshot.Severity, bucket string) string {
	if asciiMode() {
		switch sev {
		case snapshot.SevInfo:
			return "[OK]"
		case snapshot.SevWarn:
			return "[WARN]"
		case snapshot.SevFail:
			return "[FAIL]"
		}
		if bucket == "unknown" {
			return "[?]"
		}
		return "[?]"
	}
	switch sev {
	case snapshot.SevInfo:
		return "✓"
	case snapshot.SevWarn:
		return "⚠"
	case snapshot.SevFail:
		return "✗"
	}
	if bucket == "unknown" {
		return "?"
	}
	return "?"
}

func asciiMode() bool {
	if os.Getenv("NO_COLOR") != "" {
		return true
	}
	if os.Getenv("TERM") == "dumb" {
		return true
	}
	return false
}

func (m Model) renderPlaybookForCursor() string {
	row := m.cursorRow()
	if row == nil {
		return "no row selected\n"
	}
	id := guessPlaybookID(row.label)
	out, _ := playbooks.Render(id, w(m.Width, 100))
	return out
}

// guessPlaybookID maps a row label to an embedded playbook id. v0.3 uses a
// crude prefix match; richer mapping ships in v0.4.
func guessPlaybookID(name string) string {
	low := strings.ToLower(name)
	if strings.HasPrefix(low, "dell") {
		return "dell-optiplex-3080m"
	}
	if strings.HasPrefix(low, "tp") {
		return "turing-rk1"
	}
	return "dell-optiplex-3080m"
}
