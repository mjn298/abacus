package scanner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/mjn/abacus/internal/config"
)

// Runner executes scanner subprocesses and collects their output.
type Runner struct {
	timeout time.Duration
}

// NewRunner creates a Runner with the given per-scanner timeout.
func NewRunner(timeout time.Duration) *Runner {
	return &Runner{timeout: timeout}
}

// RunScanner executes a single scanner command, pipes ScanInput as JSON to
// stdin, and parses the ScanOutput JSON from stdout. The command string is
// split on whitespace into executable + args.
func (r *Runner) RunScanner(ctx context.Context, command string, input ScanInput, knownNodeIDs map[string]bool) (*ScanOutput, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	parts := strings.Fields(command)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty scanner command")
	}

	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("marshaling scanner input: %w", err)
	}
	cmd.Stdin = bytes.NewReader(inputJSON)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("scanner timed out after %s", r.timeout)
		}
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr != "" {
			return nil, fmt.Errorf("scanner exited with error: %s (stderr: %s)", err, stderrStr)
		}
		return nil, fmt.Errorf("scanner exited with error: %s", err)
	}

	var output ScanOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		// Include a snippet of the raw output for debugging.
		raw := stdout.String()
		if len(raw) > 200 {
			raw = raw[:200] + "..."
		}
		return nil, fmt.Errorf("parsing scanner output: %w (raw: %s)", err, raw)
	}

	if validationErrs := ValidateOutput(&output, knownNodeIDs); len(validationErrs) > 0 {
		return nil, fmt.Errorf("scanner output validation failed: %s", strings.Join(validationErrs, "; "))
	}

	return &output, nil
}

// RunAll executes all configured scanners, merges their results, and returns
// a MergedScanOutput. If a scanner fails, the error is captured in
// MergedScanOutput.Errors and remaining scanners continue.
func (r *Runner) RunAll(ctx context.Context, projectRoot string, configs []config.ScannerConfig, ignorePaths []string) (*MergedScanOutput, error) {
	merged := &MergedScanOutput{}

	for _, cfg := range configs {
		input := ScanInput{
			Version:     1,
			ProjectRoot: projectRoot,
			Options:     toAnyMap(cfg.Options),
			IgnorePaths: ignorePaths,
		}

		out, err := r.RunScanner(ctx, cfg.Command, input, nil)
		if err != nil {
			scannerID := cfg.Command // best identifier we have
			merged.Errors = append(merged.Errors, ScannerError{
				ScannerID: scannerID,
				Error:     err.Error(),
			})
			merged.Stats.TotalErrors++
			continue
		}

		merged.Nodes = append(merged.Nodes, out.Nodes...)
		merged.Edges = append(merged.Edges, out.Edges...)
		merged.Warnings = append(merged.Warnings, out.Warnings...)
		merged.Stats.TotalFilesScanned += out.Stats.FilesScanned
		merged.Stats.TotalNodesFound += out.Stats.NodesFound
		merged.Stats.TotalEdgesFound += out.Stats.EdgesFound
		merged.Stats.TotalErrors += out.Stats.Errors
		merged.Stats.ScannerCount++
	}

	return merged, nil
}

// toAnyMap converts map[string]interface{} to map[string]any (they're the same
// type but this makes the conversion explicit for the ScanInput field).
func toAnyMap(m map[string]interface{}) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}
