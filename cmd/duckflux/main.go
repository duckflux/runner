package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/duckflux/runner/internal/engine"
	"github.com/duckflux/runner/internal/parser"
	"github.com/duckflux/runner/internal/participant"
)

// version is set at build time via -ldflags; defaults to "dev".
var version = "dev"

// logLevel holds the slog level selected by --verbose / --quiet flags.
var logLevel = new(slog.LevelVar)

// run-command flags
var (
	runInputs    []string
	runInputFile string
)

var rootCmd = &cobra.Command{
	Use:   "duckflux",
	Short: "duckflux — cross-platform workflow runner",
	Long:  "duckflux is a cross-platform runner for the duckflux workflow DSL.",
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		verbose, _ := cmd.Flags().GetBool("verbose")
		quiet, _ := cmd.Flags().GetBool("quiet")
		if verbose && quiet {
			return fmt.Errorf("--verbose and --quiet are mutually exclusive")
		}
		switch {
		case verbose:
			logLevel.Set(slog.LevelDebug)
		case quiet:
			logLevel.Set(slog.LevelError)
		default:
			logLevel.Set(slog.LevelInfo)
		}
		return nil
	},
}

var runCmd = &cobra.Command{
	Use:   "run <file.flow.yaml>",
	Short: "Parse, validate, and execute a workflow",
	Args:  cobra.ExactArgs(1),
	RunE:  runWorkflow,
}

var lintCmd = &cobra.Command{
	Use:   "lint <file.flow.yaml>",
	Short: "Parse and validate a workflow without executing it",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f, err := os.Open(args[0])
		if err != nil {
			return fmt.Errorf("opening file: %w", err)
		}
		defer f.Close()

		if _, err := parser.Parse(f); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "OK")
		return nil
	},
}

// validateInputFlags and validateInputFile are populated by cobra flag binding.
var (
	validateInputFlags []string
	validateInputFile  string
)

var validateCmd = &cobra.Command{
	Use:   "validate <file.flow.yaml>",
	Short: "Lint a workflow and validate provided inputs",
	Long: `Lint a workflow definition and validate the supplied input values.

Performs the same checks as 'duckflux lint' (JSON Schema validation and
semantic checks), then additionally validates every --input value against
the workflow's declared inputs schema:

  - Required fields must be provided (or have a default value).
  - Values must match the declared type (string, integer, number, boolean).
  - String values with a declared format (date, date-time, uri, email) must
    conform to that format.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Step 1 — lint: parse + schema validation + semantic checks.
		f, err := os.Open(args[0])
		if err != nil {
			return fmt.Errorf("opening file: %w", err)
		}
		defer f.Close()

		wf, err := parser.Parse(f)
		if err != nil {
			return err
		}

		// Step 2 — resolve inputs from flags and optional file.
		inputs, err := parseInputFlags(validateInputFlags, validateInputFile)
		if err != nil {
			return err
		}

		// Step 3 — validate inputs against the workflow's declared schema.
		if errs := parser.ValidateInputs(wf, inputs); errs != nil {
			return errs
		}

		fmt.Fprintln(cmd.OutOrStdout(), "OK")
		return nil
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the duckflux version",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "duckflux %s\n", version)
	},
}

func init() {
	validateCmd.Flags().StringArrayVar(&validateInputFlags, "input", nil,
		"Input value in key=value format (repeatable)")
	validateCmd.Flags().StringVar(&validateInputFile, "input-file", "",
		"Path to a JSON file containing input values")
	// Persistent flags available to all subcommands.
	rootCmd.PersistentFlags().Bool("verbose", false, "Enable verbose (debug) logging")
	rootCmd.PersistentFlags().Bool("quiet", false, "Suppress all output except errors")

	// run-specific flags.
	runCmd.Flags().StringArrayVar(&runInputs, "input", nil, "Input value as key=value (repeatable)")
	runCmd.Flags().StringVar(&runInputFile, "input-file", "", "JSON file containing workflow inputs")

	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(lintCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(versionCmd)
}

// parseInputFlags merges inputs from an optional JSON file and --input
// key=value flag values into a single map[string]any.  Flag values always
// override file values for the same key.
func parseInputFlags(flags []string, inputFile string) (map[string]any, error) {
	inputs := make(map[string]any)

	if inputFile != "" {
		data, err := os.ReadFile(inputFile)
		if err != nil {
			return nil, fmt.Errorf("reading input file: %w", err)
		}
		if err := json.Unmarshal(data, &inputs); err != nil {
			return nil, fmt.Errorf("parsing input file %q: %w", inputFile, err)
		}
	}

	for _, kv := range flags {
		k, v, found := strings.Cut(kv, "=")
		if !found {
			return nil, fmt.Errorf("invalid --input value %q: expected key=value format", kv)
		}
		// --input values are always stored as strings.  They override any typed
		// value from --input-file for the same key.  The type validator handles
		// string values for numeric and boolean fields via parsing.
		inputs[k] = v
	}

	return inputs, nil
}

func main() {
	// Initialize slog with the dynamic level; default level is Info until
	// PersistentPreRunE adjusts it based on --verbose / --quiet.
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})
	slog.SetDefault(slog.New(handler))

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// runWorkflow implements the "run" subcommand.
func runWorkflow(cmd *cobra.Command, args []string) error {
	filePath := args[0]

	// ── 1. Parse and validate ────────────────────────────────────────────────
	slog.Debug("parsing workflow", "file", filePath)
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("opening workflow file: %w", err)
	}
	defer f.Close()

	wf, err := parser.Parse(f)
	if err != nil {
		return fmt.Errorf("workflow validation failed:\n%w", err)
	}
	slog.Debug("workflow parsed", "id", wf.ID, "name", wf.Name)

	// ── 2. Resolve inputs ────────────────────────────────────────────────────
	inputs, err := resolveInputs(runInputs, runInputFile)
	if err != nil {
		return fmt.Errorf("resolving inputs: %w", err)
	}

	// ── 2a. Validate inputs against the workflow's declared schema ───────────
	if errs := parser.ValidateInputs(wf, inputs); errs != nil {
		return errs
	}

	// ── 3. Collect process environment ──────────────────────────────────────
	env := collectEnv()

	// ── 4. Build participant registry ────────────────────────────────────────
	slog.Debug("building participant registry")
	runnerFn := makeSubWorkflowRunner(filepath.Dir(filePath))
	reg, err := participant.BuildRegistry(wf, env, runnerFn)
	if err != nil {
		return fmt.Errorf("building participant registry: %w", err)
	}

	// ── 5. Execute ──────────────────────────────────────────────────────────
	slog.Info("running workflow", "id", wf.ID)
	out, err := engine.Run(context.Background(), wf, inputs, env, reg)
	if err != nil {
		return fmt.Errorf("workflow execution failed: %w", err)
	}
	slog.Info("workflow completed", "id", wf.ID)

	// ── 6. Print output ──────────────────────────────────────────────────────
	return printOutput(cmd, out)
}

// resolveInputs merges inputs from stdin (JSON), --input-file, and --input flags.
// Priority (highest wins): --input flags > --input-file > stdin.
func resolveInputs(inputFlags []string, inputFile string) (map[string]any, error) {
	merged := map[string]any{}

	// Layer 1: stdin — only when stdin is piped (not a TTY) and no other
	// sources were explicitly provided.
	if isStdinPiped() && inputFile == "" && len(inputFlags) == 0 {
		stdinInputs, err := readJSONInputs(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("reading stdin inputs: %w", err)
		}
		for k, v := range stdinInputs {
			merged[k] = v
		}
	}

	// Layer 2: --input-file.
	if inputFile != "" {
		fileData, err := os.ReadFile(inputFile)
		if err != nil {
			return nil, fmt.Errorf("reading input file %q: %w", inputFile, err)
		}
		var fileInputs map[string]any
		if err := json.Unmarshal(fileData, &fileInputs); err != nil {
			return nil, fmt.Errorf("parsing input file %q: %w", inputFile, err)
		}
		for k, v := range fileInputs {
			merged[k] = v
		}
	}

	// Layer 3: --input key=value flags.
	for _, kv := range inputFlags {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return nil, fmt.Errorf("invalid --input value %q: expected key=value", kv)
		}
		merged[k] = v
	}

	return merged, nil
}

// isStdinPiped reports whether os.Stdin is a pipe / redirected file (not a TTY).
func isStdinPiped() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) == 0
}

// readJSONInputs reads a JSON object from r and returns it as map[string]any.
func readJSONInputs(r *os.File) (map[string]any, error) {
	var m map[string]any
	if err := json.NewDecoder(r).Decode(&m); err != nil {
		return nil, err
	}
	return m, nil
}

// collectEnv captures the current process environment as a map[string]string.
func collectEnv() map[string]string {
	environ := os.Environ()
	m := make(map[string]string, len(environ))
	for _, e := range environ {
		k, v, _ := strings.Cut(e, "=")
		m[k] = v
	}
	return m
}

// makeSubWorkflowRunner builds the SubWorkflowRunnerFunc used by
// WorkflowParticipant. It wires parser.Parse → participant.BuildRegistry →
// engine.Run for recursive sub-workflow execution, breaking the
// participant → engine import cycle by living in the cmd layer.
// callerDir is the directory of the workflow that contains the sub-workflow
// reference; relative paths are resolved against it.
func makeSubWorkflowRunner(callerDir string) participant.SubWorkflowRunnerFunc {
	var fn participant.SubWorkflowRunnerFunc
	fn = func(ctx context.Context, path string, inputs map[string]any, env map[string]string) (any, error) {
		// Resolve the path relative to the calling workflow's directory when it
		// is not an absolute path, so that sub-workflows can be co-located with
		// their parent regardless of the process working directory.
		if !filepath.IsAbs(path) {
			path = filepath.Join(callerDir, path)
		}

		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("opening sub-workflow %q: %w", path, err)
		}
		defer f.Close()

		wf, err := parser.Parse(f)
		if err != nil {
			return nil, fmt.Errorf("parsing sub-workflow %q: %w", path, err)
		}

		// Build the nested runner using the sub-workflow's own directory so that
		// any further nesting resolves paths correctly.
		subRunnerFn := makeSubWorkflowRunner(filepath.Dir(path))
		reg, err := participant.BuildRegistry(wf, env, subRunnerFn)
		if err != nil {
			return nil, fmt.Errorf("building registry for sub-workflow %q: %w", path, err)
		}

		return engine.Run(ctx, wf, inputs, env, reg)
	}
	return fn
}

// printOutput writes the workflow output to cmd's stdout.
// Maps are serialised as indented JSON (json.Encoder adds a trailing newline).
// Strings are written verbatim — shell participants (exec) typically include
// their own trailing newline, and CEL-computed strings are emitted as-is so
// callers can post-process the output without stripping an unwanted newline.
// All other scalar types are formatted via %v.
func printOutput(cmd *cobra.Command, out any) error {
	if out == nil {
		return nil
	}
	w := cmd.OutOrStdout()
	switch v := out.(type) {
	case map[string]any:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(v); err != nil {
			return fmt.Errorf("encoding output as JSON: %w", err)
		}
	case string:
		fmt.Fprint(w, v)
	default:
		fmt.Fprintf(w, "%v", v)
	}
	return nil
}
