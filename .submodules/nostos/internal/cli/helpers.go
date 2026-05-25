package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/yurifrl/nostos/internal/cli/dryrun"
	"github.com/yurifrl/nostos/internal/cli/errs"
	"github.com/yurifrl/nostos/internal/cli/inputx"
	"github.com/yurifrl/nostos/internal/cli/jsonio"
	"github.com/yurifrl/nostos/internal/config"
	"github.com/yurifrl/nostos/internal/paths"
)

// loadConfig resolves + parses the config.yaml. Cobra-level errors are printed.
func loadConfig() (*config.Config, paths.Paths, error) {
	if err := inputx.ValidateConfigPath(configPath); err != nil {
		return nil, paths.Paths{}, err
	}
	p, err := config.FindConfig(configPath, "")
	if err != nil {
		return nil, paths.Paths{}, errs.NotFound("E_CONFIG_NOT_FOUND", err.Error()).
			WithHint("run `nostos init` to scaffold one, or pass --config <path>")
	}
	cfg, err := config.Load(p)
	if err != nil {
		return nil, paths.Paths{}, errs.Validation("E_CONFIG_PARSE", err.Error()).
			WithHint("run `nostos schema` to see the expected shape")
	}
	return cfg, paths.New(p), nil
}

// outWriter is the destination for outputJSON/emit*. Defaults to os.Stdout but
// is rewired to cmd.OutOrStdout() inside PersistentPreRunE so test harnesses
// using cobra.SetOut() capture the JSON.
var outWriter io.Writer = os.Stdout

// outputJSON prints v as pretty JSON to outWriter.
func outputJSON(v any) error {
	return jsonio.EncodePretty(outWriter, v)
}

// outputJSONTo prints v as pretty JSON to w.
func outputJSONTo(w io.Writer, v any) error {
	return jsonio.EncodePretty(w, v)
}

// emitListOutput decides between JSON pretty output and a human-text fallback.
func emitListOutput(slice any, schema []string, fields []string, textFn func()) error {
	return emitListOutputTo(outWriter, slice, schema, fields, textFn)
}

func emitListOutputTo(w io.Writer, slice any, schema []string, fields []string, textFn func()) error {
	if outputMode == "json" {
		if len(fields) > 0 {
			projected, err := jsonio.ProjectSlice(slice, fields)
			if err != nil {
				return err
			}
			return jsonio.EncodeNDJSON(w, projected)
		}
		return jsonio.EncodeNDJSON(w, slice)
	}
	textFn()
	return nil
}

func emitObjectOutput(v any, fields []string, textFn func()) error {
	return emitObjectOutputTo(outWriter, v, fields, textFn)
}

func emitObjectOutputTo(w io.Writer, v any, fields []string, textFn func()) error {
	if outputMode == "json" {
		m, err := jsonio.ToMap(v)
		if err != nil {
			return err
		}
		if len(fields) > 0 {
			m = jsonio.ProjectFields(m, fields)
		}
		return jsonio.EncodePretty(w, m)
	}
	textFn()
	return nil
}

// emitDryRun writes the preview Plan in the canonical shape to w.
func emitDryRun(p *dryrun.Plan) error {
	return emitDryRunTo(outWriter, p)
}

func emitDryRunTo(w io.Writer, p *dryrun.Plan) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(p)
}

// fieldsValue holds the parsed --fields flag for a command.
// Each command that wants projection registers a flag bound to a *string and
// validates against its schema list inside RunE.
func parseFields(raw string, schema []string) ([]string, error) {
	return inputx.ValidateFieldsMask(raw, schema)
}

// runEFunc wraps a typed-error returning RunE in the cobra signature.
// Errors flow up to the root error handler which routes JSON-on-stdout +
// hint-on-stderr + correct exit code.
func runEFunc(fn func(cmd *cobra.Command, args []string) error) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		return fn(cmd, args)
	}
}

// runEFuncSimple is retained for back-compat with existing call sites that
// don't yet use typed errors. New code should prefer runEFunc + errs.*.
func runEFuncSimple(fn func(cmd *cobra.Command, args []string) error) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		return fn(cmd, args)
	}
}

// HandleExit is the top-level error router invoked from main.
// Routes typed errors to JSON-on-stdout + hint-on-stderr; legacy errors fall
// back to a stderr "Error: ..." message; exits with the matching code.
func HandleExit(err error) {
	if err == nil {
		os.Exit(0)
	}
	var typed *errs.Error
	if asTyped(err, &typed) {
		if outputMode == "json" {
			errs.Emit(os.Stdout, os.Stderr, typed)
		} else {
			fmt.Fprintf(os.Stderr, "Error: %s\n", typed.Message)
			if typed.Hint != "" {
				fmt.Fprintln(os.Stderr, "hint: "+typed.Hint)
			}
		}
		os.Exit(typed.Exit())
	}
	// Heuristic classify legacy errors and emit consistent shape under --output json.
	classified := errs.FromGo(err)
	if outputMode == "json" {
		errs.Emit(os.Stdout, os.Stderr, classified)
	} else {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
	}
	os.Exit(classified.Exit())
}

// asTyped is a tiny errors.As wrapper to avoid pulling errors into helpers.go.
func asTyped(err error, target **errs.Error) bool {
	for cur := err; cur != nil; {
		if t, ok := cur.(*errs.Error); ok {
			*target = t
			return true
		}
		u, ok := cur.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		cur = u.Unwrap()
	}
	return false
}
