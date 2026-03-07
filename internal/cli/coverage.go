package cli

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/mjn/abacus/internal/db"
	"github.com/mjn/abacus/internal/graph"
	"github.com/mjn/abacus/internal/match"
	"github.com/spf13/cobra"
)

var coverageCmd = &cobra.Command{
	Use:   "coverage [glob]",
	Short: "Show Gherkin step coverage report",
	Args:  cobra.MaximumNArgs(1),
	RunE:  coverageRunE,
}

func init() {
	rootCmd.AddCommand(coverageCmd)
}

type coverageResult struct {
	TotalSteps   int            `json:"total_steps"`
	ExactMatches int            `json:"exact_matches"`
	FuzzyMatches int            `json:"fuzzy_matches"`
	Suggestions  int            `json:"suggestions"`
	CoveragePct  float64        `json:"coverage_pct"`
	Files        []fileCoverage `json:"files"`
}

type fileCoverage struct {
	File         string  `json:"file"`
	TotalSteps   int     `json:"total_steps"`
	ExactMatches int     `json:"exact_matches"`
	CoveragePct  float64 `json:"coverage_pct"`
}

func coverageRunE(cmd *cobra.Command, args []string) error {
	w := cmd.OutOrStdout()
	jsonFlag, _ := cmd.Flags().GetBool("json")

	globPattern := "**/*.feature"
	if len(args) > 0 {
		globPattern = args[0]
	}

	// Find feature files (supports ** recursive globbing via WalkDir)
	files, err := globFeatureFiles(globPattern)
	if err != nil {
		return fmt.Errorf("glob pattern: %w", err)
	}
	if len(files) == 0 {
		Info(w, "No feature files found matching pattern: "+globPattern)
		return nil
	}

	// Open DB
	database, err := db.OpenDB(dbPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer database.Close()

	repo := graph.NewGraphRepository(database)
	actions := graph.NewActionService(repo)
	matcher := match.NewMatchService(repo, actions, match.MatchOptions{})

	result := coverageResult{}

	for _, file := range files {
		steps, err := parseFeatureFile(file)
		if err != nil {
			Warn(w, fmt.Sprintf("skipping %s: %v", file, err))
			continue
		}

		fc := fileCoverage{
			File:       file,
			TotalSteps: len(steps),
		}

		for _, step := range steps {
			mr, err := matcher.Match(step.Text)
			if err != nil {
				Warn(w, fmt.Sprintf("match error for %q: %v", step.Text, err))
				continue
			}
			result.TotalSteps++
			switch mr.Tier {
			case "exact":
				result.ExactMatches++
				fc.ExactMatches++
			case "fuzzy":
				result.FuzzyMatches++
			case "suggest":
				result.Suggestions++
			}
		}

		if fc.TotalSteps > 0 {
			fc.CoveragePct = float64(fc.ExactMatches) / float64(fc.TotalSteps) * 100
		}
		result.Files = append(result.Files, fc)
	}

	if result.TotalSteps > 0 {
		result.CoveragePct = float64(result.ExactMatches) / float64(result.TotalSteps) * 100
	}

	if jsonFlag {
		return PrintJSON(w, result)
	}

	// Table output
	fmt.Fprintf(w, "\nCoverage Report\n")
	fmt.Fprintf(w, "===============\n\n")

	headers := []string{"File", "Steps", "Exact", "Coverage"}
	var rows [][]string
	for _, fc := range result.Files {
		rows = append(rows, []string{
			fc.File,
			fmt.Sprintf("%d", fc.TotalSteps),
			fmt.Sprintf("%d", fc.ExactMatches),
			fmt.Sprintf("%.1f%%", fc.CoveragePct),
		})
	}
	PrintTable(w, headers, rows)

	fmt.Fprintf(w, "\nTotal: %d steps, %d exact, %d fuzzy, %d suggestions\n",
		result.TotalSteps, result.ExactMatches, result.FuzzyMatches, result.Suggestions)
	fmt.Fprintf(w, "Coverage: %.1f%%\n", result.CoveragePct)

	return nil
}

// globFeatureFiles finds files matching a glob pattern, supporting ** for
// recursive directory traversal (which filepath.Glob doesn't handle).
func globFeatureFiles(pattern string) ([]string, error) {
	if !strings.Contains(pattern, "**") {
		return filepath.Glob(pattern)
	}

	// Split pattern on ** to get prefix dir and suffix pattern
	parts := strings.SplitN(pattern, "**", 2)
	root := filepath.Clean(parts[0])
	if root == "" || root == "." {
		root = "."
	}
	suffix := strings.TrimPrefix(parts[1], "/")
	if suffix == "" {
		suffix = "*"
	}

	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip inaccessible dirs
		}
		if d.IsDir() {
			return nil
		}
		name := filepath.Base(path)
		matched, _ := filepath.Match(suffix, name)
		if matched {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}
