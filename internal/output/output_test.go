package output_test

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/fil-forge/guppy/internal/output"
)

// newCmd returns a command wired with the --output flag set to format, plus
// captured stdout/stderr buffers.
func newCmd(t *testing.T, format string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().StringP(output.FlagName, "o", output.FormatText, "")
	if err := cmd.Flags().Set(output.FlagName, format); err != nil {
		t.Fatalf("setting --output: %v", err)
	}
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	return cmd, &out, &errb
}

type sample struct {
	Name string `json:"name"`
	N    int    `json:"n"`
}

func TestEmitJSON(t *testing.T) {
	cmd, out, _ := newCmd(t, output.FormatJSON)

	textCalled := false
	err := output.Emit(cmd, sample{Name: "a&b", N: 3}, func(w io.Writer) {
		textCalled = true
		_, _ = io.WriteString(w, "TEXT")
	})
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if textCalled {
		t.Fatal("text renderer must not run in JSON mode")
	}

	// Exactly one JSON document on stdout (json.Encoder appends a single newline).
	got := out.String()
	if strings.Count(strings.TrimRight(got, "\n"), "\n") != 0 {
		t.Fatalf("expected a single-line JSON document, got %q", got)
	}
	var decoded sample
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("stdout is not valid JSON (%v): %q", err, got)
	}
	if decoded != (sample{Name: "a&b", N: 3}) {
		t.Fatalf("round-trip mismatch: %+v", decoded)
	}
	// HTML escaping disabled: "&" must survive verbatim.
	if !strings.Contains(got, "a&b") {
		t.Fatalf("expected unescaped '&' in output, got %q", got)
	}
}

func TestEmitText(t *testing.T) {
	cmd, out, _ := newCmd(t, output.FormatText)

	err := output.Emit(cmd, sample{Name: "x", N: 1}, func(w io.Writer) {
		_, _ = io.WriteString(w, "human line\n")
	})
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if got := out.String(); got != "human line\n" {
		t.Fatalf("text output = %q, want %q", got, "human line\n")
	}
}

func TestEmitTextNilRenderer(t *testing.T) {
	cmd, out, _ := newCmd(t, output.FormatText)
	if err := output.Emit(cmd, sample{}, nil); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if got := out.String(); got != "" {
		t.Fatalf("expected no output for nil text renderer, got %q", got)
	}
}

func TestEmitJSONNilRenderer(t *testing.T) {
	cmd, out, _ := newCmd(t, output.FormatJSON)
	if err := output.Emit(cmd, []sample{{Name: "only", N: 9}}, nil); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	var decoded []sample
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("stdout is not valid JSON array (%v): %q", err, out.String())
	}
	if len(decoded) != 1 || decoded[0].Name != "only" {
		t.Fatalf("unexpected decoded array: %+v", decoded)
	}
}

func TestIsJSON(t *testing.T) {
	jsonCmd, _, _ := newCmd(t, output.FormatJSON)
	if !output.IsJSON(jsonCmd) {
		t.Fatal("IsJSON should be true for --output json")
	}
	textCmd, _, _ := newCmd(t, output.FormatText)
	if output.IsJSON(textCmd) {
		t.Fatal("IsJSON should be false for --output text")
	}
}

func TestValidate(t *testing.T) {
	for _, format := range []string{output.FormatText, output.FormatJSON} {
		cmd, _, _ := newCmd(t, format)
		if err := output.Validate(cmd); err != nil {
			t.Fatalf("Validate(%q) unexpected error: %v", format, err)
		}
	}
	bad, _, _ := newCmd(t, "yaml")
	if err := output.Validate(bad); err == nil {
		t.Fatal("Validate should reject an unknown format")
	}
}
