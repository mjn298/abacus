package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
)

// writer returns the configured output writer (defaults to os.Stdout).
// Commands should use cmd.OutOrStdout() when available; this is for
// package-level helpers.
var writer io.Writer = os.Stdout

// PrintJSON marshals v as indented JSON and writes it to w.
func PrintJSON(w io.Writer, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}

// PrintTable writes headers and rows using aligned columns.
func PrintTable(w io.Writer, headers []string, rows [][]string) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(headers, "\t"))
	for _, row := range rows {
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	tw.Flush()
}

// Info prints an informational message, respecting the quiet flag.
func Info(w io.Writer, msg string) {
	if !quiet {
		fmt.Fprintln(w, msg)
	}
}

// Warn prints a warning message to the given writer.
func Warn(w io.Writer, msg string) {
	fmt.Fprintln(w, "WARNING:", msg)
}
