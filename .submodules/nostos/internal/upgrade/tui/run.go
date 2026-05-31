package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/yurifrl/nostos/internal/upgrade"
)

// Result is the outcome of an interactive upgrade-plan session.
type Result struct {
	Action    Action
	Confirmed bool
}

// Run launches the interactive upgrade-plan TUI for plan and blocks until the
// operator picks an action. It is the thin tea.Program wrapper around Model;
// unit tests drive Model directly instead of calling Run (no terminal needed).
func Run(ctx context.Context, plan upgrade.Plan) (Result, error) {
	m := New(plan)
	p := tea.NewProgram(m, tea.WithContext(ctx))
	out, err := p.Run()
	if err != nil {
		return Result{}, err
	}
	final, _ := out.(Model)
	return Result{Action: final.Action(), Confirmed: final.Confirmed()}, nil
}
