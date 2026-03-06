package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/mjn/abacus/internal/db"
	"github.com/mjn/abacus/internal/graph"
	"github.com/mjn/abacus/internal/match"
	"github.com/spf13/cobra"
)

var matchCmd = &cobra.Command{
	Use:   "match [step-text]",
	Short: "Match a Gherkin step to an Action",
	Args:  cobra.MaximumNArgs(1),
	RunE:  matchRunE,
}

func init() {
	matchCmd.Flags().StringP("file", "f", "", "Feature file to match all steps")
	matchCmd.Flags().StringP("keyword", "k", "", "Step keyword (Given/When/Then)")
	matchCmd.Flags().Bool("create", false, "Auto-create Action from suggestion")
	matchCmd.Flags().Float64("threshold", 0, "Fuzzy match threshold")
	rootCmd.AddCommand(matchCmd)
}

func matchRunE(cmd *cobra.Command, args []string) error {
	w := cmd.OutOrStdout()

	fileFlag, _ := cmd.Flags().GetString("file")
	createFlag, _ := cmd.Flags().GetBool("create")
	threshold, _ := cmd.Flags().GetFloat64("threshold")
	jsonFlag, _ := cmd.Flags().GetBool("json")

	// Open DB
	database, err := db.OpenDB(dbPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer database.Close()

	repo := graph.NewGraphRepository(database)
	actions := graph.NewActionService(repo)

	opts := match.MatchOptions{
		FuzzyThreshold: threshold,
	}
	matcher := match.NewMatchService(repo, actions, opts)

	// File mode
	if fileFlag != "" {
		return matchFileMode(cmd, matcher, actions, fileFlag, createFlag, jsonFlag)
	}

	// Single step mode
	if len(args) == 0 {
		return fmt.Errorf("provide step text as argument or use --file")
	}
	stepText := args[0]

	result, err := matcher.Match(stepText)
	if err != nil {
		return fmt.Errorf("matching: %w", err)
	}

	if jsonFlag {
		return PrintJSON(w, matchResultToJSON(stepText, result))
	}

	printMatchResult(w, stepText, result)

	// Auto-create if suggestion and --create
	if createFlag && result.Tier == "suggest" && result.Suggestion != nil {
		created, err := actions.Create(result.Suggestion.Input)
		if err != nil {
			return fmt.Errorf("creating action: %w", err)
		}
		Info(w, fmt.Sprintf("Created action: %s", created.Action.ID))
	}

	return nil
}

type matchResultJSON struct {
	StepText   string            `json:"step_text"`
	Tier       string            `json:"tier"`
	ActionID   string            `json:"action_id,omitempty"`
	ActionName string            `json:"action_name,omitempty"`
	Parameters map[string]string `json:"parameters,omitempty"`
	Candidates []candidateJSON   `json:"candidates,omitempty"`
	Suggestion *suggestionJSON   `json:"suggestion,omitempty"`
}

type candidateJSON struct {
	ActionID   string  `json:"action_id"`
	ActionName string  `json:"action_name"`
	Score      float64 `json:"score"`
}

type suggestionJSON struct {
	Name     string   `json:"name"`
	Label    string   `json:"label"`
	Patterns []string `json:"patterns"`
}

func matchResultToJSON(stepText string, r *match.MatchResult) matchResultJSON {
	out := matchResultJSON{
		StepText:   stepText,
		Tier:       r.Tier,
		Parameters: r.Parameters,
	}
	if r.Action != nil {
		out.ActionID = r.Action.Node.ID
		out.ActionName = r.Action.Node.Name
	}
	if len(r.Candidates) > 0 {
		out.Candidates = make([]candidateJSON, len(r.Candidates))
		for i, c := range r.Candidates {
			out.Candidates[i] = candidateJSON{
				ActionID:   c.Action.ID,
				ActionName: c.Action.Name,
				Score:      c.Score,
			}
		}
	}
	if r.Suggestion != nil {
		out.Suggestion = &suggestionJSON{
			Name:     r.Suggestion.Input.Name,
			Label:    r.Suggestion.Input.Label,
			Patterns: r.Suggestion.Input.GherkinPatterns,
		}
	}
	return out
}

func printMatchResult(w io.Writer, stepText string, r *match.MatchResult) {
	switch r.Tier {
	case "exact":
		fmt.Fprintf(w, "Tier: exact\n")
		if r.Action != nil {
			fmt.Fprintf(w, "Action: %s (%s)\n", r.Action.Node.Name, r.Action.Node.ID)
		}
		if len(r.Parameters) > 0 {
			fmt.Fprintf(w, "Parameters:\n")
			for k, v := range r.Parameters {
				fmt.Fprintf(w, "  %s = %s\n", k, v)
			}
		}
	case "fuzzy":
		fmt.Fprintf(w, "Tier: fuzzy\n")
		fmt.Fprintf(w, "Candidates:\n")
		for _, c := range r.Candidates {
			fmt.Fprintf(w, "  %.4f  %s (%s)\n", c.Score, c.Action.Name, c.Action.ID)
		}
	case "suggest":
		fmt.Fprintf(w, "Tier: suggest\n")
		if r.Suggestion != nil {
			fmt.Fprintf(w, "Suggested action: %s\n", r.Suggestion.Input.Name)
			fmt.Fprintf(w, "Pattern: %s\n", strings.Join(r.Suggestion.Input.GherkinPatterns, ", "))
		}
	}
}

func matchFileMode(cmd *cobra.Command, matcher *match.MatchService, actions *graph.ActionService, filePath string, createFlag bool, jsonFlag bool) error {
	w := cmd.OutOrStdout()

	steps, err := parseFeatureFile(filePath)
	if err != nil {
		return fmt.Errorf("parsing feature file: %w", err)
	}

	results, err := matcher.MatchScenario(steps)
	if err != nil {
		return fmt.Errorf("matching scenario: %w", err)
	}

	if jsonFlag {
		jsonResults := make([]matchResultJSON, len(results))
		for i, r := range results {
			jsonResults[i] = matchResultToJSON(steps[i].Text, &r)
		}
		return PrintJSON(w, jsonResults)
	}

	for i, r := range results {
		fmt.Fprintf(w, "\n[%s] %s\n", steps[i].Keyword, steps[i].Text)
		printMatchResult(w, steps[i].Text, &r)

		if createFlag && r.Tier == "suggest" && r.Suggestion != nil {
			created, err := actions.Create(r.Suggestion.Input)
			if err != nil {
				Warn(w, fmt.Sprintf("failed to create action: %v", err))
			} else {
				Info(w, fmt.Sprintf("Created action: %s", created.Action.ID))
			}
		}
	}

	return nil
}

// stepLineRe matches Gherkin step lines: optional whitespace, keyword, then text.
var stepLineRe = regexp.MustCompile(`^\s*(Given|When|Then|And|But)\s+(.+)$`)

// parseFeatureFile extracts Gherkin steps from a feature file using regex parsing.
func parseFeatureFile(filePath string) ([]match.Step, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	var steps []match.Step
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		matches := stepLineRe.FindStringSubmatch(line)
		if len(matches) == 3 {
			steps = append(steps, match.Step{
				Keyword: matches[1],
				Text:    strings.TrimSpace(matches[2]),
			})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	return steps, nil
}
