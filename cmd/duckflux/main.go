package main

import (
	"fmt"
	"os"

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

var validateCmd = &cobra.Command{
	Use:   "validate <file.flow.yaml>",
	Short: "Lint a workflow and validate provided inputs",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(cmd.ErrOrStderr(), "validate: not yet implemented")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(lintCmd)
	rootCmd.AddCommand(validateCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
