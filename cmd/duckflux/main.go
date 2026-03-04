package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/duckflux/runner/internal/parser"
)

var rootCmd = &cobra.Command{
	Use:   "duckflux",
	Short: "duckflux — cross-platform workflow runner",
	Long:  "duckflux is a cross-platform runner for the duckflux workflow DSL.",
}

var runCmd = &cobra.Command{
	Use:   "run <file.flow.yaml>",
	Short: "Parse, validate, and execute a workflow",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(cmd.ErrOrStderr(), "run: not yet implemented")
		return nil
	},
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

func init() {
	validateCmd.Flags().StringArrayVar(&validateInputFlags, "input", nil,
		"Input value in key=value format (repeatable)")
	validateCmd.Flags().StringVar(&validateInputFile, "input-file", "",
		"Path to a JSON file containing input values")

	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(lintCmd)
	rootCmd.AddCommand(validateCmd)
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
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
