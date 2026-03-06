package cli

import (
	"fmt"

	"github.com/mjn/abacus/internal/db"
	"github.com/mjn/abacus/internal/graph"
	"github.com/spf13/cobra"
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show graph statistics",
	RunE:  statsRunE,
}

func init() {
	rootCmd.AddCommand(statsCmd)
}

func statsRunE(cmd *cobra.Command, args []string) error {
	w := cmd.OutOrStdout()
	jsonFlag, _ := cmd.Flags().GetBool("json")

	database, err := db.OpenDB(dbPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer database.Close()

	repo := graph.NewGraphRepository(database)

	kinds := []db.NodeKind{db.NodeRoute, db.NodeEntity, db.NodePage, db.NodeAction, db.NodePermission}
	stats := make(map[string]int)
	total := 0

	for _, kind := range kinds {
		nodes, err := repo.GetNodesByKind(kind, 100000, 0)
		if err != nil {
			return fmt.Errorf("counting %s: %w", kind, err)
		}
		count := len(nodes)
		stats[string(kind)] = count
		total += count
	}

	if jsonFlag {
		stats["total"] = total
		return PrintJSON(w, stats)
	}

	headers := []string{"KIND", "COUNT"}
	rows := make([][]string, 0, len(kinds)+1)
	for _, kind := range kinds {
		rows = append(rows, []string{string(kind), fmt.Sprintf("%d", stats[string(kind)])})
	}
	rows = append(rows, []string{"Total", fmt.Sprintf("%d", total)})
	PrintTable(w, headers, rows)

	return nil
}
