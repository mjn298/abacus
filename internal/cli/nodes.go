package cli

import (
	"fmt"

	"github.com/mjn/abacus/internal/db"
	"github.com/mjn/abacus/internal/graph"
	"github.com/spf13/cobra"
)

// queryNodesCmd returns a pre-configured cobra.Command that queries nodes by
// kind with optional FTS5 search via --match and a --limit cap.
func queryNodesCmd(kind db.NodeKind, use, short string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			jsonFlag, _ := cmd.Flags().GetBool("json")
			matchQuery, _ := cmd.Flags().GetString("match")
			limit, _ := cmd.Flags().GetInt("limit")

			if limit <= 0 {
				limit = 1000
			}

			database, err := db.OpenDB(dbPath)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer database.Close()

			repo := graph.NewGraphRepository(database)

			var nodes []db.GraphNode

			if matchQuery != "" {
				results, err := repo.Search(matchQuery, &kind, limit)
				if err != nil {
					return fmt.Errorf("searching %s: %w", kind, err)
				}
				nodes = make([]db.GraphNode, len(results))
				for i, r := range results {
					nodes[i] = r.Node
				}
			} else {
				nodes, err = repo.GetNodesByKind(kind, limit, 0)
				if err != nil {
					return fmt.Errorf("listing %s: %w", kind, err)
				}
			}

			if jsonFlag {
				return PrintJSON(w, nodes)
			}

			headers := []string{"ID", "NAME", "LABEL", "SOURCE"}
			extraHeader := ""
			switch kind {
			case db.NodeRoute:
				extraHeader = "METHOD"
			case db.NodeEntity:
				extraHeader = "FIELDS"
			case db.NodePage:
				extraHeader = "PATH"
			}
			if extraHeader != "" {
				headers = append(headers, extraHeader)
			}

			rows := make([][]string, len(nodes))
			for i, n := range nodes {
				source := string(n.Source)
				if n.SourceFile != nil {
					source = *n.SourceFile
				}
				row := []string{n.ID, n.Name, n.Label, source}

				switch kind {
				case db.NodeRoute:
					method := ""
					if n.Properties != nil {
						if m, ok := n.Properties["method"]; ok {
							method = fmt.Sprintf("%v", m)
						}
					}
					row = append(row, method)
				case db.NodeEntity:
					fieldCount := ""
					if n.Properties != nil {
						if fields, ok := n.Properties["fields"]; ok {
							if arr, ok := fields.([]any); ok {
								fieldCount = fmt.Sprintf("%d", len(arr))
							} else {
								fieldCount = fmt.Sprintf("%v", fields)
							}
						}
					}
					row = append(row, fieldCount)
				case db.NodePage:
					path := ""
					if n.Properties != nil {
						if p, ok := n.Properties["path"]; ok {
							path = fmt.Sprintf("%v", p)
						}
					}
					row = append(row, path)
				}

				rows[i] = row
			}

			PrintTable(w, headers, rows)
			return nil
		},
	}

	cmd.Flags().StringP("match", "m", "", "FTS5 search query")
	cmd.Flags().IntP("limit", "l", 0, "Maximum number of results (0 = all)")

	return cmd
}
