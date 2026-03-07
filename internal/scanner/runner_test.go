package scanner

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/mjn/abacus/internal/config"
)

func echoScannerPath(t *testing.T) string {
	t.Helper()
	// Resolve path relative to this test file's location.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "testdata", "scanners", "echo-scanner.sh")
}

func TestRunScanner_Success(t *testing.T) {
	r := NewRunner(10 * time.Second)
	input := ScanInput{
		Version:     1,
		ProjectRoot: "/tmp/test-project",
		Options:     map[string]any{},
	}

	out, err := r.RunScanner(context.Background(), echoScannerPath(t), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if out.Version != 1 {
		t.Errorf("expected version 1, got %d", out.Version)
	}
	if out.Scanner.ID != "echo-scanner" {
		t.Errorf("expected scanner id 'echo-scanner', got %q", out.Scanner.ID)
	}
	if len(out.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(out.Nodes))
	}
	if len(out.Edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(out.Edges))
	}
}

func TestRunScanner_ValidationErrors(t *testing.T) {
	// Create an inline script that outputs invalid JSON (bad version)
	r := NewRunner(10 * time.Second)
	input := ScanInput{Version: 1, ProjectRoot: "/tmp", Options: map[string]any{}}

	// Use a command that outputs invalid scanner output (version 99)
	cmd := `bash -c 'echo "{\"version\":99,\"scanner\":{\"id\":\"\"},\"nodes\":[],\"edges\":[],\"warnings\":[],\"stats\":{}}"'`
	_, err := r.RunScanner(context.Background(), cmd, input)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

func TestRunScanner_BadJSON(t *testing.T) {
	r := NewRunner(10 * time.Second)
	input := ScanInput{Version: 1, ProjectRoot: "/tmp", Options: map[string]any{}}

	cmd := `bash -c 'echo "not json at all"'`
	_, err := r.RunScanner(context.Background(), cmd, input)
	if err == nil {
		t.Fatal("expected JSON parse error, got nil")
	}
}

func TestRunScanner_NonZeroExit(t *testing.T) {
	r := NewRunner(10 * time.Second)
	input := ScanInput{Version: 1, ProjectRoot: "/tmp", Options: map[string]any{}}

	cmd := `bash -c 'echo "scanner failed" >&2; exit 1'`
	_, err := r.RunScanner(context.Background(), cmd, input)
	if err == nil {
		t.Fatal("expected error from non-zero exit, got nil")
	}
}

func TestRunScanner_Timeout(t *testing.T) {
	r := NewRunner(100 * time.Millisecond)
	input := ScanInput{Version: 1, ProjectRoot: "/tmp", Options: map[string]any{}}

	cmd := `bash -c 'sleep 10'`
	_, err := r.RunScanner(context.Background(), cmd, input)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestRunAll_MergesResults(t *testing.T) {
	scannerPath := echoScannerPath(t)
	r := NewRunner(10 * time.Second)

	configs := []config.ScannerConfig{
		{Command: scannerPath, Options: map[string]interface{}{}},
	}

	merged, err := r.RunAll(context.Background(), "/tmp/test-project", configs, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(merged.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(merged.Nodes))
	}
	if len(merged.Edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(merged.Edges))
	}
	if merged.Stats.ScannerCount != 1 {
		t.Errorf("expected scanner count 1, got %d", merged.Stats.ScannerCount)
	}
	if len(merged.Errors) != 0 {
		t.Errorf("expected no errors, got %v", merged.Errors)
	}
}

func TestRunAll_CapturesErrorsContinues(t *testing.T) {
	scannerPath := echoScannerPath(t)
	r := NewRunner(10 * time.Second)

	configs := []config.ScannerConfig{
		{Command: `bash -c 'echo fail >&2; exit 1'`, Options: map[string]interface{}{}},
		{Command: scannerPath, Options: map[string]interface{}{}},
	}

	merged, err := r.RunAll(context.Background(), "/tmp/test-project", configs, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(merged.Errors) != 1 {
		t.Fatalf("expected 1 scanner error, got %d", len(merged.Errors))
	}
	// Second scanner should have succeeded
	if len(merged.Nodes) != 2 {
		t.Errorf("expected 2 nodes from second scanner, got %d", len(merged.Nodes))
	}
}
