package cli

import (
	"fmt"

	"github.com/mjn/abacus/internal/db"
	"github.com/mjn/abacus/internal/graph"
	"github.com/spf13/cobra"
)

var actionsCmd = &cobra.Command{
	Use:   "actions",
	Short: "List or search action nodes",
	RunE:  actionsRunE,
}

var actionsCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new action node",
	Args:  cobra.ExactArgs(1),
	RunE:  actionsCreateRunE,
}

func init() {
	actionsCmd.Flags().StringP("match", "m", "", "FTS5 search query")
	actionsCmd.Flags().IntP("limit", "l", 50, "Maximum number of results")

	actionsCreateCmd.Flags().StringSlice("gherkin", nil, "Gherkin cucumber expression patterns")
	actionsCreateCmd.Flags().String("label", "", "Human-readable label")
	actionsCreateCmd.Flags().StringSlice("route-ref", nil, "Route node IDs to link")
	actionsCreateCmd.Flags().StringSlice("entity-ref", nil, "Entity node IDs to link")
	actionsCreateCmd.Flags().StringSlice("page-ref", nil, "Page node IDs to link")

	actionsCmd.AddCommand(actionsCreateCmd)
	rootCmd.AddCommand(actionsCmd)
}

func actionsRunE(cmd *cobra.Command, args []string) error {
	w := cmd.OutOrStdout()
	jsonFlag, _ := cmd.Flags().GetBool("json")
	matchQuery, _ := cmd.Flags().GetString("match")
	limit, _ := cmd.Flags().GetInt("limit")

	if limit <= 0 {
		limit = 50
	}

	database, err := db.OpenDB(dbPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer database.Close()

	repo := graph.NewGraphRepository(database)
	actions := graph.NewActionService(repo)

	if matchQuery != "" {
		suggestions, err := actions.Suggest(graph.SuggestContext{Query: matchQuery, Limit: limit})
		if err != nil {
			return fmt.Errorf("searching actions: %w", err)
		}

		if jsonFlag {
			return PrintJSON(w, suggestions)
		}

		headers := []string{"ID", "NAME", "LABEL", "ROUTES", "ENTITIES", "PAGES"}
		rows := make([][]string, len(suggestions))
		for i, a := range suggestions {
			rows[i] = []string{
				a.Node.ID,
				a.Node.Name,
				a.Node.Label,
				fmt.Sprintf("%d", len(a.Routes)),
				fmt.Sprintf("%d", len(a.Entities)),
				fmt.Sprintf("%d", len(a.Pages)),
			}
		}
		PrintTable(w, headers, rows)
		return nil
	}

	list, err := actions.List(graph.ListActionOpts{Limit: limit})
	if err != nil {
		return fmt.Errorf("listing actions: %w", err)
	}

	if jsonFlag {
		return PrintJSON(w, list)
	}

	headers := []string{"ID", "NAME", "LABEL", "ROUTES", "ENTITIES", "PAGES"}
	rows := make([][]string, len(list))
	for i, a := range list {
		rows[i] = []string{
			a.Node.ID,
			a.Node.Name,
			a.Node.Label,
			fmt.Sprintf("%d", len(a.Routes)),
			fmt.Sprintf("%d", len(a.Entities)),
			fmt.Sprintf("%d", len(a.Pages)),
		}
	}
	PrintTable(w, headers, rows)
	return nil
}

func actionsCreateRunE(cmd *cobra.Command, args []string) error {
	w := cmd.OutOrStdout()
	jsonFlag, _ := cmd.Flags().GetBool("json")

	name := args[0]
	label, _ := cmd.Flags().GetString("label")
	gherkin, _ := cmd.Flags().GetStringSlice("gherkin")
	routeRefs, _ := cmd.Flags().GetStringSlice("route-ref")
	entityRefs, _ := cmd.Flags().GetStringSlice("entity-ref")
	pageRefs, _ := cmd.Flags().GetStringSlice("page-ref")

	database, err := db.OpenDB(dbPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer database.Close()

	repo := graph.NewGraphRepository(database)
	actions := graph.NewActionService(repo)

	result, err := actions.Create(graph.CreateActionInput{
		Name:            name,
		Label:           label,
		GherkinPatterns: gherkin,
		RouteRefs:       routeRefs,
		EntityRefs:      entityRefs,
		PageRefs:        pageRefs,
	})
	if err != nil {
		return fmt.Errorf("creating action: %w", err)
	}

	if jsonFlag {
		return PrintJSON(w, result)
	}

	Info(w, fmt.Sprintf("Created action: %s (%s)", result.Action.Name, result.Action.ID))
	if len(result.Edges) > 0 {
		Info(w, fmt.Sprintf("  Edges created: %d", len(result.Edges)))
	}
	for _, warn := range result.Warnings {
		Warn(w, warn)
	}

	return nil
}
