// Package dryrun defines the structured "preview" payload returned by
// every mutating command when invoked with --dry-run.
//
// Contract: zero subprocess invocations, exit code 0, stdout is JSON
// shaped {"status":"preview","would_execute":[{"phase":...}]}.
package dryrun

// Step is one planned action.
type Step struct {
	Phase   string         `json:"phase"`
	Detail  string         `json:"detail"`
	Argv    []string       `json:"argv,omitempty"`
	EnvKeys []string       `json:"env_keys,omitempty"`
	Meta    map[string]any `json:"meta,omitempty"`
}

// Plan is the dry-run output envelope.
type Plan struct {
	Status        string `json:"status"` // always "preview"
	Method        string `json:"method,omitempty"`
	WouldExecute  []Step `json:"would_execute"`
}

// New returns a fresh preview Plan for method.
func New(method string) *Plan {
	return &Plan{Status: "preview", Method: method, WouldExecute: []Step{}}
}

// Add appends a step.
func (p *Plan) Add(phase, detail string) *Plan {
	p.WouldExecute = append(p.WouldExecute, Step{Phase: phase, Detail: detail})
	return p
}

// AddArgv appends a step capturing the argv template (no values).
func (p *Plan) AddArgv(phase, detail string, argv []string, envKeys []string) *Plan {
	p.WouldExecute = append(p.WouldExecute, Step{
		Phase: phase, Detail: detail, Argv: argv, EnvKeys: envKeys,
	})
	return p
}
