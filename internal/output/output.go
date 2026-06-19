// Package output centralizes how guppy commands present their results.
//
// Every command routes its final, machine-relevant result through [Emit],
// which renders either human-readable text or a single JSON document depending
// on the root --output flag (see [FlagName]). The contract for JSON mode is
// strict: stdout carries exactly one JSON document and nothing else, so that
// callers (e.g. the smelt test harness) can decode stdout without scraping.
// All progress, prompts, warnings, and diagnostics must therefore go to stderr
// (via cmd.PrintErr*), never stdout, regardless of mode.
package output

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// FlagName is the name of the root persistent output-format flag.
const FlagName = "output"

// Supported --output values.
const (
	FormatText = "text"
	FormatJSON = "json"
)

// format returns the resolved --output value, defaulting to text.
func format(cmd *cobra.Command) string {
	v, err := cmd.Flags().GetString(FlagName)
	if err != nil || v == "" {
		return FormatText
	}
	return v
}

// Validate reports whether the --output flag holds a supported value. It is
// intended to be called from the root command's PersistentPreRunE.
func Validate(cmd *cobra.Command) error {
	switch format(cmd) {
	case FormatText, FormatJSON:
		return nil
	default:
		v, _ := cmd.Flags().GetString(FlagName)
		return fmt.Errorf("invalid --%s %q: must be %q or %q", FlagName, v, FormatText, FormatJSON)
	}
}

// IsJSON reports whether JSON output is selected.
func IsJSON(cmd *cobra.Command) bool { return format(cmd) == FormatJSON }

// Emit writes a command's final result to stdout.
//
// In JSON mode it encodes jsonValue as a single, compact JSON document
// (HTML escaping disabled so DIDs/CIDs render verbatim). In text mode it
// invokes text with the stdout writer; text may be nil for commands that are
// silent on success in text mode.
func Emit(cmd *cobra.Command, jsonValue any, text func(w io.Writer)) error {
	if IsJSON(cmd) {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetEscapeHTML(false)
		if err := enc.Encode(jsonValue); err != nil {
			return fmt.Errorf("encoding json output: %w", err)
		}
		return nil
	}
	if text != nil {
		text(cmd.OutOrStdout())
	}
	return nil
}
