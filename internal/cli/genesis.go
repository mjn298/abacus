package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/mjn/abacus/internal/db"
	"github.com/mjn/abacus/internal/graph"
	"github.com/mjn/abacus/internal/match"
	"github.com/spf13/cobra"
)

var genesisCmd = &cobra.Command{
	Use:   "genesis [step-text]",
	Short: "Resolve a step, entity, or node to its implementation context",
	Long: `Genesis traces a Gherkin step, entity name, or node ID through the
application graph, returning classified results with delegation chains
and source file mappings.

For step text input, the result includes a classification (covered, wirable,
or new) based on whether matching actions and infrastructure exist.

For entity or node input, the result shows the full implementation context
including all delegation chains from routes through modules to entities.`,
	Example: `  $ abacus genesis "the user creates an account"
  $ abacus genesis --entity User
  $ abacus genesis --node "route:POST-/users"
  $ abacus genesis --entity User --depth 3 --json`,
	Args: cobra.MaximumNArgs(1),
	RunE: genesisRunE,
}

func init() {
	genesisCmd.Flags().String("entity", "", "Entity name to show implementation pattern for")
	genesisCmd.Flags().String("node", "", "Node ID to traverse from directly")
	genesisCmd.Flags().IntP("depth", "d", 5, "Maximum traversal depth")
	genesisCmd.Flags().BoolP("verbose", "v", false, "Show progress messages")
	rootCmd.AddCommand(genesisCmd)
}

func genesisRunE(cmd *cobra.Command, args []string) error {
	w := cmd.OutOrStdout()
	jsonFlag, _ := cmd.Flags().GetBool("json")
	entityFlag, _ := cmd.Flags().GetString("entity")
	nodeFlag, _ := cmd.Flags().GetString("node")
	depthFlag, _ := cmd.Flags().GetInt("depth")
	verbose, _ := cmd.Flags().GetBool("verbose")

	// Determine input mode.
	var stepText string
	if len(args) > 0 {
		stepText = args[0]
	}

	if stepText == "" && entityFlag == "" && nodeFlag == "" {
		return fmt.Errorf("provide step text as argument, or use --entity or --node")
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Opening database...\n")
	}

	database, err := db.OpenDB(dbPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer database.Close()

	if verbose {
		fmt.Fprintf(os.Stderr, "Loading action index...\n")
	}

	repo := graph.NewGraphRepository(database)
	actions := graph.NewActionService(repo)
	matcher := match.NewMatchService(repo, actions, match.MatchOptions{})
	adapter := match.NewGenesisAdapter(matcher)
	svc := graph.NewGenesisService(repo, adapter)

	input := graph.GenesisInput{
		Step:   stepText,
		Entity: entityFlag,
		NodeID: nodeFlag,
		Depth:  depthFlag,
	}

	if verbose {
		switch {
		case entityFlag != "":
			fmt.Fprintf(os.Stderr, "Resolving entity %q (depth %d)...\n", entityFlag, depthFlag)
		case nodeFlag != "":
			fmt.Fprintf(os.Stderr, "Resolving node %q (depth %d)...\n", nodeFlag, depthFlag)
		default:
			fmt.Fprintf(os.Stderr, "Resolving step %q (depth %d)...\n", stepText, depthFlag)
		}
	}

	result, err := svc.Genesis(input)
	if err != nil {
		return fmt.Errorf("genesis: %w", err)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Done.\n")
	}

	if jsonFlag {
		return PrintJSON(w, result)
	}

	printGenesisResult(w, result)
	return nil
}

func printGenesisResult(w interface {
	Write([]byte) (int, error)
}, result *graph.GenesisResult) {
	// Classification and match tier (step mode).
	if result.Classification != "" {
		fmt.Fprintf(w, "Classification: %s\n", strings.ToUpper(result.Classification))
	}
	if result.MatchTier != "" {
		fmt.Fprintf(w, "Match Tier: %s\n", result.MatchTier)
	}

	// Action info (exact match).
	if result.Action != nil {
		fmt.Fprintf(w, "Action: %s (%s)\n", result.Action.Name, result.Action.ID)
		if len(result.Action.GherkinPatterns) > 0 {
			fmt.Fprintf(w, "  Patterns: %s\n", strings.Join(result.Action.GherkinPatterns, ", "))
		}
	}

	// Check if there are any nodes at all.
	hasNodes := len(result.Routes) > 0 || len(result.Entities) > 0 ||
		len(result.Modules) > 0 || len(result.Pages) > 0

	if !hasNodes {
		if result.Classification == "new" {
			fmt.Fprintf(w, "\nNo matching infrastructure found. Run abacus genesis --entity <similar-entity> to discover implementation patterns.\n")
		}
		return
	}

	// Routes.
	if len(result.Routes) > 0 {
		fmt.Fprintf(w, "\nRoutes (%d):\n", len(result.Routes))
		headers := []string{"  ID", "NAME", "SOURCE"}
		rows := make([][]string, len(result.Routes))
		for i, r := range result.Routes {
			rows[i] = []string{"  " + r.ID, r.Name, r.SourceFile}
		}
		PrintTable(w, headers, rows)
	}

	// Entities.
	if len(result.Entities) > 0 {
		fmt.Fprintf(w, "\nEntities (%d):\n", len(result.Entities))
		headers := []string{"  ID", "NAME", "SOURCE"}
		rows := make([][]string, len(result.Entities))
		for i, e := range result.Entities {
			rows[i] = []string{"  " + e.ID, e.Name, e.SourceFile}
		}
		PrintTable(w, headers, rows)
	}

	// Modules.
	if len(result.Modules) > 0 {
		fmt.Fprintf(w, "\nModules (%d):\n", len(result.Modules))
		headers := []string{"  ID", "NAME", "SOURCE"}
		rows := make([][]string, len(result.Modules))
		for i, m := range result.Modules {
			rows[i] = []string{"  " + m.ID, m.Name, m.SourceFile}
		}
		PrintTable(w, headers, rows)
	}

	// Pages.
	if len(result.Pages) > 0 {
		fmt.Fprintf(w, "\nPages (%d):\n", len(result.Pages))
		headers := []string{"  ID", "NAME", "SOURCE"}
		rows := make([][]string, len(result.Pages))
		for i, p := range result.Pages {
			rows[i] = []string{"  " + p.ID, p.Name, p.SourceFile}
		}
		PrintTable(w, headers, rows)
	}

	// Delegation chains.
	if len(result.DelegationChains) > 0 {
		fmt.Fprintf(w, "\nDelegation Chains:\n")
		for _, chain := range result.DelegationChains {
			fmt.Fprintf(w, "  %s\n", strings.Join(chain, " \u2192 "))
		}
	}
}
