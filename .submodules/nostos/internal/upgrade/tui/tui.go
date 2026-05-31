// Package tui implements the Bubble Tea v2 screen for `nostos upgrade`.
//
// It renders the computed upgrade Plan (cluster, current/target versions, the
// per-minor step sequence, and which nodes each step touches) and lets the
// operator pick an action: Proceed (with an explicit confirm), Dry-run, or
// Quit. The screen hides version/schematic complexity by default; pressing `d`
// reveals per-node detail (IPs and, when available, factory schematic IDs).
//
// The Model is deliberately self-contained: it takes a prebuilt upgrade.Plan
// and never touches the network, a cluster, or a terminal — so it can be unit
// tested by driving Update with key messages and asserting on View output, the
// same way the dashboard TUI is tested.
package tui

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/yurifrl/nostos/internal/upgrade"
)

// Action is the operator's chosen outcome once the program exits.
type Action string

const (
	// ActionNone means the program exited without a decision (treated as quit).
	ActionNone Action = ""
	// ActionProceed runs the health-gated upgrade (only after an explicit confirm).
	ActionProceed Action = "proceed"
	// ActionDryRun prints the plan and changes nothing.
	ActionDryRun Action = "dry-run"
	// ActionQuit aborts with no changes.
	ActionQuit Action = "quit"
)

// footer action order.
var actionLabels = []struct {
	action Action
	label  string
}{
	{ActionProceed, "Proceed"},
	{ActionDryRun, "Dry-run"},
	{ActionQuit, "Quit"},
	// TODO(qfn.3): add "Customize" once per-node overrides are supported.
}

// Model is the upgrade-plan TUI state.
type Model struct {
	Plan upgrade.Plan

	Width  int
	Height int

	cursor     int  // selected footer action
	showDetail bool // reveal IPs + schematic IDs
	confirming bool // Proceed selected, awaiting y/n confirm

	action    Action
	confirmed bool
	done      bool
}

// New builds a Model around a prebuilt plan.
func New(plan upgrade.Plan) Model { return Model{Plan: plan} }

// Action returns the chosen action once the program is done ("" while running).
func (m Model) Action() Action { return m.action }

// Confirmed reports whether the operator explicitly confirmed a Proceed.
func (m Model) Confirmed() bool { return m.confirmed }

// Done reports whether the model has reached a terminal decision.
func (m Model) Done() bool { return m.done }

// Init satisfies tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update satisfies tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Confirm gate for Proceed: y commits, any other key cancels.
	if m.confirming {
		switch msg.String() {
		case "y", "Y", "enter":
			m.confirming = false
			m.action = ActionProceed
			m.confirmed = true
			m.done = true
			return m, tea.Quit
		default:
			m.confirming = false
			return m, nil
		}
	}

	switch msg.String() {
	case "q", "esc", "ctrl+c":
		m.action = ActionQuit
		m.done = true
		return m, tea.Quit
	case "d":
		m.showDetail = !m.showDetail
	case "left", "up", "h", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "right", "down", "l", "j":
		if m.cursor < len(actionLabels)-1 {
			m.cursor++
		}
	case "enter", " ":
		return m.selectAction()
	}
	return m, nil
}

func (m Model) selectAction() (tea.Model, tea.Cmd) {
	switch actionLabels[m.cursor].action {
	case ActionProceed:
		// Require an explicit second confirm before mutating the cluster.
		m.confirming = true
		return m, nil
	case ActionDryRun:
		m.action = ActionDryRun
		m.done = true
		return m, tea.Quit
	case ActionQuit:
		m.action = ActionQuit
		m.done = true
		return m, tea.Quit
	}
	return m, nil
}

// View renders the upgrade-plan screen.
func (m Model) View() tea.View {
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("\n\n")
	b.WriteString(m.steps())
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", w(m.Width, 72)))
	b.WriteString("\n")
	b.WriteString(m.nodes())
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", w(m.Width, 72)))
	b.WriteString("\n")
	b.WriteString(m.footer())
	return wrap(b.String())
}

func wrap(s string) tea.View {
	v := tea.NewView(s)
	v.AltScreen = true
	return v
}

func w(width, fallback int) int {
	if width <= 0 {
		return fallback
	}
	return width
}

func (m Model) header() string {
	name := m.Plan.Cluster
	if name == "" {
		name = "cluster"
	}
	title := lipgloss.NewStyle().Bold(true).Render("[ " + name + " — upgrade ]")
	cur := m.Plan.MinCurrent
	if cur == "" {
		cur = "?"
	}
	return fmt.Sprintf("%s\nCluster on %s, target %s", title, cur, m.Plan.Target)
}

func (m Model) steps() string {
	if len(m.Plan.Steps) == 0 {
		return "Nothing to do — all nodes already at or above target."
	}
	var seq []string
	for _, s := range m.Plan.Steps {
		seq = append(seq, s.Version)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Plan: %s\n", strings.Join(seq, " → "))
	for i, s := range m.Plan.Steps {
		var names []string
		for _, n := range s.Nodes {
			names = append(names, n.Name)
		}
		fmt.Fprintf(&b, "  %d. %s  →  %s  (controlplane last)\n",
			i+1, s.Version, strings.Join(names, ", "))
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) nodes() string {
	if len(m.Plan.Nodes) == 0 {
		return "  (no nodes)"
	}
	var b strings.Builder
	b.WriteString("Nodes:\n")
	for _, n := range m.Plan.Nodes {
		role := n.Role
		if role == "" {
			role = "worker"
		}
		cur := m.Plan.Current[n.Name]
		if cur == "" {
			cur = "—"
		}
		// Which step versions touch this node.
		var touches []string
		for _, s := range m.Plan.Steps {
			for _, sn := range s.Nodes {
				if sn.Name == n.Name {
					touches = append(touches, s.Version)
					break
				}
			}
		}
		steps := "up-to-date"
		if len(touches) > 0 {
			steps = strings.Join(touches, " → ")
		}
		fmt.Fprintf(&b, "  %-10s  %-13s  %-8s  %s\n", n.Name, role, cur, steps)
		if m.showDetail {
			detail := "    ip=" + n.IP
			if sch := m.Plan.Schematics[n.Name]; sch != "" {
				detail += "  schematic=" + sch
			}
			b.WriteString(detail + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) footer() string {
	if m.confirming {
		return lipgloss.NewStyle().Bold(true).Render(
			"Proceed with upgrade? nodes will reboot — press y to confirm, any other key to cancel")
	}
	var parts []string
	for i, a := range actionLabels {
		label := "[ " + a.label + " ]"
		if i == m.cursor {
			label = selectedStyle().Render("▸ " + label)
		} else {
			label = "  " + label
		}
		parts = append(parts, label)
	}
	keys := "←/→ move · enter select · d detail · q quit"
	return strings.Join(parts, "  ") + "\n" + keys
}

func selectedStyle() lipgloss.Style {
	if asciiMode() {
		return lipgloss.NewStyle().Bold(true)
	}
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#2ecc71"))
}

func asciiMode() bool {
	return os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb"
}
