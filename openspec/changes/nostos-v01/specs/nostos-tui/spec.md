## ADDED Requirements

### Requirement: Charm v2 stack only
The system SHALL use Charm v2 libraries via `charm.land/...` imports. v1 (`github.com/charmbracelet/...`) SHALL NOT appear in `go.mod` or source.

#### Scenario: Imports are v2
- **WHEN** any `.go` file imports Charm libs
- **THEN** the import path begins with `charm.land/` (never `github.com/charmbracelet/`)

#### Scenario: go.mod pins v2 versions
- **WHEN** an operator runs `go list -m all` in `.submodules/nostos/`
- **THEN** the listed Charm modules are all at `v2.x.y` via `charm.land/` paths

### Requirement: View() returns tea.View
Every Bubble Tea model in the system SHALL implement `View() tea.View` (not `View() string`). Terminal features (alt-screen, mouse mode, window title, cursor) SHALL be declared on the returned `tea.View` struct, never via `tea.Cmd` commands.

#### Scenario: Models return tea.View
- **WHEN** the install-progress or status-watch model's `View()` method is called
- **THEN** it returns a `tea.View` (constructed via `tea.NewView` or struct literal)

#### Scenario: Alt-screen declared on view
- **WHEN** a model enables alt-screen
- **THEN** it sets `v.AltScreen = true` on the returned view, and does NOT return `tea.EnterAltScreen` as a command

### Requirement: Key handling via tea.KeyPressMsg
Key input in the system SHALL be matched via `tea.KeyPressMsg` (and `tea.KeyReleaseMsg` when keyboard enhancements are active). `tea.KeyMsg` (v1) SHALL NOT be used.

#### Scenario: Update switches on KeyPressMsg
- **WHEN** a model handles keyboard input
- **THEN** the type switch uses `case tea.KeyPressMsg` and matches via `msg.String()`

#### Scenario: Space key matched as "space"
- **WHEN** a model matches the space bar
- **THEN** it compares `msg.String()` to `"space"` (not `" "` as in v1)

### Requirement: Bubbles components use v2 patterns
Bubbles components in the system SHALL use functional-option constructors and getter/setter methods. Direct field assignment (e.g. `vp.Width = 40`) SHALL NOT be used.

#### Scenario: Viewport construction
- **WHEN** a viewport is created
- **THEN** it uses `viewport.New(viewport.WithWidth(80), viewport.WithHeight(24))`, not positional args

#### Scenario: Setter methods for dimensions
- **WHEN** a component's width or height is updated after construction
- **THEN** `SetWidth(n)` / `SetHeight(n)` are called, not direct field assignment

### Requirement: Adaptive colors via LightDark
Styling in the system SHALL handle light/dark terminals via `lipgloss.LightDark(isDark)(light, dark)` driven by `tea.BackgroundColorMsg`. `lipgloss.AdaptiveColor` (removed in v2) SHALL NOT be used.

#### Scenario: Background color detection
- **WHEN** a model handles `tea.BackgroundColorMsg`
- **THEN** it calls `lipgloss.LightDark(msg.IsDark())` to produce a color picker function and re-renders styles using it

### Requirement: Install progress view
The system SHALL render `nostos up <node>` progress via a Bubble Tea model that consumes `Event` values from the orchestrator's channel. The view SHALL show, in chronological order: stage transitions, download events, config-fetched, node-up, apid-up, bootstrapping, ready.

#### Scenario: Inline rendering by default
- **WHEN** `nostos up dell01` runs on a TTY without `--alt-screen`
- **THEN** events stream into the terminal without taking over the full screen (preserving scrollback)

#### Scenario: Alt-screen opt-in
- **WHEN** `nostos up dell01 --alt-screen` runs
- **THEN** the TUI uses Bubble Tea's alt-screen mode and restores the prior terminal state on exit

#### Scenario: Non-TTY fallback
- **WHEN** `nostos up dell01 > /tmp/log.txt` runs (output piped, non-TTY)
- **THEN** events are printed as plain newline-delimited lines with a timestamp, no ANSI codes, no cursor movement

#### Scenario: Ctrl+C restores terminal
- **WHEN** the operator presses Ctrl+C during `nostos up`
- **THEN** the PXE subprocesses are terminated AND the terminal is restored to its prior state (no dangling alt-screen, cursor visible)

### Requirement: Live cluster dashboard
The system SHALL provide `nostos status --watch` that renders a live-updating Bubble Tea dashboard showing per-node reachability, apid state, and Talos version. Refresh interval SHALL be 5 seconds; operator can trigger immediate refresh with `r`.

#### Scenario: Watch refreshes automatically
- **WHEN** `nostos status --watch` runs for 15 seconds
- **THEN** the table is refreshed at least 2 times (every ~5s)

#### Scenario: Manual refresh
- **WHEN** the operator presses `r` in the watch view
- **THEN** an immediate refresh is triggered regardless of the 5s interval

#### Scenario: Quit key
- **WHEN** the operator presses `q` or Ctrl+C
- **THEN** the TUI exits cleanly with exit code 0

### Requirement: Node-add wizard via Huh
The system SHALL implement `nostos node add <name>` as a Huh form with fields for MAC, IP, role (select), arch (select), install disk, template filename. Each field SHALL validate on submit (e.g. MAC shape check, IP parse).

#### Scenario: Validation prevents submission
- **WHEN** the operator types a malformed MAC in the form
- **THEN** the form displays an inline validation error and does not advance until corrected

#### Scenario: Template auto-suggest
- **WHEN** the template-filename field is focused
- **THEN** the default value is `<name>.yaml` based on the command argument

### Requirement: Shared Lipgloss style set
The system SHALL define a single `internal/tui/style.go` exporting named styles (header, dim, good, bad, warn, pill variants). Every TUI view SHALL use these styles for consistency.

#### Scenario: Consistent pill rendering across views
- **WHEN** the install progress view and the status dashboard both render a reachability pill
- **THEN** the rendered ANSI output is identical for the same state (e.g. both `up` pills are the same color + border style)
