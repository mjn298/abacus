package cli

import (
	"fmt"

	"github.com/mjn/abacus/internal/db"
	"github.com/mjn/abacus/internal/graph"
	"github.com/spf13/cobra"
)

var graphCmd = &cobra.Command{
	Use:   "graph <node-id>",
	Short: "Show the connected subgraph around a node",
	Long: `Perform a bidirectional BFS traversal from a node and display the connected subgraph.

Returns all nodes and edges reachable within the specified depth. Use --depth to
control how far the traversal extends from the starting node.`,
	Example: `  $ abacus graph "route:GET-/api/users"
  $ abacus graph "entity:User" --depth 3
  $ abacus graph "action:create-user" --depth 1 --json`,
	Args: cobra.ExactArgs(1),
	RunE: graphRunE,
}

func init() {
	graphCmd.Flags().IntP("depth", "d", 2, "Maximum traversal depth")
	graphCmd.Flags().StringSlice("edge-kinds", nil, "Filter traversal to specific edge kinds (comma-separated)")
	rootCmd.AddCommand(graphCmd)
}

func graphRunE(cmd *cobra.Command, args []string) error {
	w := cmd.OutOrStdout()
	jsonFlag, _ := cmd.Flags().GetBool("json")
	depth, _ := cmd.Flags().GetInt("depth")

	nodeID := args[0]

	if depth <= 0 {
		depth = 2
	}

	database, err := db.OpenDB(dbPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer database.Close()

	repo := graph.NewGraphRepository(database)

	edgeKindStrs, _ := cmd.Flags().GetStringSlice("edge-kinds")
	var edgeKinds []db.EdgeKind
	for _, k := range edgeKindStrs {
		edgeKinds = append(edgeKinds, db.EdgeKind(k))
	}

	subgraph, err := repo.GetConnected(nodeID, depth, edgeKinds)
	if err != nil {
		return fmt.Errorf("getting connected subgraph: %w", err)
	}

	if jsonFlag {
		return PrintJSON(w, subgraph)
	}

	fmt.Fprintf(w, "NODES (%d):\n", len(subgraph.Nodes))
	headers := []string{"  ID", "KIND", "NAME"}
	rows := make([][]string, len(subgraph.Nodes))
	for i, n := range subgraph.Nodes {
		rows[i] = []string{"  " + n.ID, string(n.Kind), n.Name}
	}
	PrintTable(w, headers, rows)

	fmt.Fprintf(w, "\nEDGES (%d):\n", len(subgraph.Edges))
	edgeHeaders := []string{"  SRC", "DST", "KIND"}
	edgeRows := make([][]string, len(subgraph.Edges))
	for i, e := range subgraph.Edges {
		edgeRows[i] = []string{"  " + e.SrcID, e.DstID, string(e.Kind)}
	}
	PrintTable(w, edgeHeaders, edgeRows)

	return nil
}
