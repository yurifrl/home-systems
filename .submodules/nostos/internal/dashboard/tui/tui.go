// Package tui implements the Bubble Tea v2 dashboard model.
//
// v0.3 ships read-only by default but mutating action handlers (i, r, d,
// capital G) are wired through the actions.Dispatcher seam. The Real-cluster
// path requires a two-keystroke confirm: a sentinel keypress (e.g. `i`) puts
// the model into a 3-second armed window during which `y` confirms; any other
// key cancels.
package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/yurifrl/nostos/internal/dashboard/actions"
	"github.com/yurifrl/nostos/internal/dashboard/playbooks"
	"github.com/yurifrl/nostos/internal/dashboard/snapshot"
)

// Model is the TUI state.
type Model struct {
	Snap        snapshot.Snapshot
	Cursor      int  // selected row in the inventory
	ShowHelp    bool
	ShowHidden  bool
	ShowDocs    bool
	DocsBody    string
	Filter      string
	Width       int
	Height      int
	asciiOnly   bool

	// Cached indicates the current Snap was loaded from on-disk cache and
	// hasn't been refreshed yet. The TUI prefixes such row labels with `~`.
	Cached bool

	// Dispatcher backs i/r/d/G. When nil, those keys no-op (with a stderr line).
	Dispatcher actions.Dispatcher

	// pending is the armed two-keystroke action; expires PendingExpiry.
	pending       actions.Kind
	pendingArgv   []string
	pendingTarget actions.Target
	pendingErr    error
	PendingExpiry time.Time

	// StatusPane is the last action result (rendered under inventory).
	StatusPane string

	// ConfirmTimeout is the chord window. Defaults to 3 * time.Second.
	ConfirmTimeout time.Duration

	// Now lets tests inject a clock.
	Now func() time.Time
}

func (m Model) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m Model) confirmTimeout() time.Duration {
	if m.ConfirmTimeout > 0 {
		return m.ConfirmTimeout
	}
	return 3 * time.Second
}

// Pending returns the currently armed action kind ("" when not armed).
func (m Model) Pending() actions.Kind { return m.pending }

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

	// Two-keystroke confirm path: if armed, `y` (within timeout) commits;
	// any other key cancels.
	if m.pending != "" {
		if m.now().After(m.PendingExpiry) {
			m.pending = ""
			m.StatusPane = "action timed out—press the action key again"
			return m, nil
		}
		if msg.String() == "y" || msg.String() == "Y" {
			return m.commitPending()
		}
		m.pending = ""
		m.StatusPane = "cancelled"
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
	case "i":
		return m.armIdentify()
	case "r":
		return m.armReinstall()
	case "d":
		return m.armDelete()
	case "G":
		return m.armGoFix()
	}
	return m, nil
}

// --- two-keystroke arming -----------------------------------------------------

func (m Model) armIdentify() (tea.Model, tea.Cmd) {
	row := m.cursorRow()
	if row == nil {
		m.StatusPane = "no row selected"
		return m, nil
	}
	t := m.targetFor(row)
	// Pre-flight: synthesize argv via the same dispatcher logic so the prompt
	// shows what would actually run.
	dry := &actions.ExecDispatcher{DryRun: true}
	res, err := dry.Identify(context.Background(), t)
	if err != nil {
		fmt.Fprintf(os.Stderr, "identify: %v\n", err)
		m.StatusPane = fmt.Sprintf("identify: %v", err)
		return m, nil
	}
	m.pending = actions.KindIdentify
	m.pendingArgv = res.Argv
	m.pendingTarget = t
	m.PendingExpiry = m.now().Add(m.confirmTimeout())
	m.StatusPane = fmt.Sprintf("about to: %s — press y to confirm (3s)", strings.Join(res.Argv, " "))
	return m, nil
}

func (m Model) armReinstall() (tea.Model, tea.Cmd) {
	row := m.cursorRow()
	if row == nil {
		m.StatusPane = "no row selected"
		return m, nil
	}
	t := m.targetFor(row)
	if t.Bucket == "unknown" || t.Bucket == "" || t.Name == "" {
		m.StatusPane = "reinstall only works on known nodes (press n to name first)"
		return m, nil
	}
	dry := &actions.ExecDispatcher{DryRun: true}
	res, err := dry.Reinstall(context.Background(), t)
	if err != nil {
		m.StatusPane = fmt.Sprintf("reinstall: %v", err)
		return m, nil
	}
	m.pending = actions.KindReinstall
	m.pendingArgv = res.Argv
	m.pendingTarget = t
	m.PendingExpiry = m.now().Add(m.confirmTimeout())
	m.StatusPane = fmt.Sprintf("about to: %s — press y to confirm (3s)", strings.Join(res.Argv, " "))
	return m, nil
}

func (m Model) armDelete() (tea.Model, tea.Cmd) {
	row := m.cursorRow()
	if row == nil {
		m.StatusPane = "no row selected"
		return m, nil
	}
	t := m.targetFor(row)
	if !t.IsOrphan && t.Bucket != "orphan" {
		m.StatusPane = "delete only allowed on orphan rows"
		return m, nil
	}
	dry := &actions.ExecDispatcher{DryRun: true}
	res, err := dry.Delete(context.Background(), t)
	if err != nil {
		m.StatusPane = fmt.Sprintf("delete: %v", err)
		return m, nil
	}
	m.pending = actions.KindDelete
	m.pendingArgv = res.Argv
	m.pendingTarget = t
	m.PendingExpiry = m.now().Add(m.confirmTimeout())
	m.StatusPane = fmt.Sprintf("about to: %s — press y to confirm (3s)", strings.Join(res.Argv, " "))
	return m, nil
}

func (m Model) armGoFix() (tea.Model, tea.Cmd) {
	dry := &actions.ExecDispatcher{DryRun: true}
	res, err := dry.GoFix(context.Background(), m.Snap)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		m.StatusPane = err.Error()
		return m, nil
	}
	m.pending = actions.KindGoFix
	m.pendingArgv = res.Argv
	m.PendingExpiry = m.now().Add(m.confirmTimeout())
	m.StatusPane = fmt.Sprintf("about to: %s — press y to confirm (3s)", strings.Join(res.Argv, " "))
	return m, nil
}

func (m Model) commitPending() (tea.Model, tea.Cmd) {
	k := m.pending
	m.pending = ""
	if m.Dispatcher == nil {
		m.StatusPane = fmt.Sprintf("%s: no dispatcher bound (would run: %s)", k, strings.Join(m.pendingArgv, " "))
		return m, nil
	}
	ctx := context.Background()
	var (
		res actions.Result
		err error
	)
	switch k {
	case actions.KindIdentify:
		res, err = m.Dispatcher.Identify(ctx, m.pendingTarget)
	case actions.KindReinstall:
		res, err = m.Dispatcher.Reinstall(ctx, m.pendingTarget)
	case actions.KindDelete:
		res, err = m.Dispatcher.Delete(ctx, m.pendingTarget)
	case actions.KindGoFix:
		res, err = m.Dispatcher.GoFix(ctx, m.Snap)
	}
	if err != nil {
		m.StatusPane = fmt.Sprintf("%s failed: %v", k, err)
		return m, nil
	}
	tag := "dispatched"
	if res.DryRun {
		tag = "would run"
	}
	m.StatusPane = fmt.Sprintf("%s [%s]: %s", tag, k, strings.Join(res.Argv, " "))
	return m, nil
}

// targetFor maps a row to an actions.Target. Boot method + tpi slot are looked
// up from the snapshot's Nodes (which carry the resolved config view).
func (m Model) targetFor(row *rowSpec) actions.Target {
	t := actions.Target{Name: row.label, Bucket: row.bucket, Severity: row.sev}
	for _, n := range m.Snap.Nodes {
		if n.Name == row.label || "~"+n.Name == row.label {
			t.IP = n.IP
			t.BootMethod = bootHint(n)
			break
		}
	}
	for _, d := range m.Snap.Discoveries {
		if d.IP == row.label || d.Hostname == row.label {
			t.IP = d.IP
			if d.Bucket == "orphan" {
				t.IsOrphan = true
				t.Bucket = "orphan"
			}
			break
		}
	}
	return t
}

// bootHint guesses the boot method from a snapshot.Node row. The TUI doesn't
// carry the full config; it falls back to "pxe" for non-RK1 hosts.
func bootHint(n snapshot.Node) string {
	if strings.HasPrefix(strings.ToLower(n.Name), "tp") {
		return "tpi"
	}
	return "pxe"
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
	keys := []string{"[i]dentify", "[r]einstall", "[d]elete", "[G]o-fix", "[?]help", "[H]hidden", "[u]upgrade", "[s]docs", "[/]search", "[q]quit"}
	if row != nil {
		switch row.bucket {
		case "unknown":
			keys = append([]string{"[n]ame"}, keys...)
		}
	}
	line := strings.Join(keys, "  ")
	if m.StatusPane != "" {
		line = m.StatusPane + "\n" + line
	}
	return line
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
		"  i       identify selected node (chord: i then y)",
		"  r       reinstall selected node (chord: r then y)",
		"  d       delete orphan/stale row (chord: d then y)",
		"  G       go-fix worst remediable check (chord: G then y)",
		"  n       on an unknown row, emit config patch to stderr",
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

// guessPlaybookID maps a row label to an embedded playbook id. Crude prefix
// match — richer mapping (config-aware) ships later.
func guessPlaybookID(name string) string {
	low := strings.TrimPrefix(strings.ToLower(name), "~")
	switch {
	case strings.HasPrefix(low, "dell"):
		return "dell-optiplex-3080m"
	case strings.HasPrefix(low, "tp"), strings.HasPrefix(low, "rk1"):
		return "turing-rk1"
	case strings.HasPrefix(low, "pi"), strings.HasPrefix(low, "rpi"), strings.Contains(low, "raspberry"):
		return "raspberry-pi-5"
	case strings.HasPrefix(low, "pc"), strings.HasPrefix(low, "nuc"), strings.HasPrefix(low, "amd"):
		return "generic-amd64"
	}
	return "generic-amd64"
}
