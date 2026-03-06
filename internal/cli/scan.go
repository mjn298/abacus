package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/mjn/abacus/internal/config"
	"github.com/mjn/abacus/internal/db"
	"github.com/mjn/abacus/internal/graph"
	"github.com/mjn/abacus/internal/scanner"
	"github.com/spf13/cobra"
)

var scanCmd = &cobra.Command{
	Use:   "scan [type]",
	Short: "Run scanners and ingest results into the graph",
	Long:  "Runs all configured scanners (or a specific one by ID) and ingests discovered nodes and edges into the graph database.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runScan,
}

func init() {
	rootCmd.AddCommand(scanCmd)
}

type scanResult struct {
	NodesIngested int                    `json:"nodes_ingested"`
	EdgesCreated  int                    `json:"edges_created"`
	Warnings      []string               `json:"warnings"`
	Errors        []scanner.ScannerError `json:"errors,omitempty"`
	Stats         scanner.MergedStats    `json:"stats"`
}

func runScan(cmd *cobra.Command, args []string) error {
	w := cmd.OutOrStdout()

	// Derive config path from dbPath
	configPath := resolveConfigPath(dbPath)
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Build scanner configs, optionally filtering by type
	var filterType string
	if len(args) > 0 {
		filterType = args[0]
	}

	configs := make([]config.ScannerConfig, 0, len(cfg.Scanners))
	for id, sc := range cfg.Scanners {
		if filterType != "" && id != filterType {
			continue
		}
		configs = append(configs, sc)
	}

	if filterType != "" && len(configs) == 0 {
		return fmt.Errorf("no scanner found with ID %q", filterType)
	}

	if len(configs) == 0 {
		return fmt.Errorf("no scanners configured; check %s", configPath)
	}

	// Run scanners
	runner := scanner.NewRunner(60 * time.Second)
	ctx := context.Background()
	merged, err := runner.RunAll(ctx, cfg.Project.Root, configs)
	if err != nil {
		return fmt.Errorf("running scanners: %w", err)
	}

	// Open DB and ingest
	database, err := db.OpenDB(dbPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer database.Close()

	repo := graph.NewGraphRepository(database)

	// Ingest nodes
	graphNodes := make([]db.GraphNode, len(merged.Nodes))
	for i, sn := range merged.Nodes {
		var sf *string
		if sn.SourceFile != "" {
			sf = &sn.SourceFile
		}
		graphNodes[i] = db.GraphNode{
			ID:         sn.ID,
			Kind:       db.NodeKind(sn.Kind),
			Name:       sn.Name,
			Label:      sn.Label,
			Properties: sn.Properties,
			Source:     db.NodeSource(sn.Source),
			SourceFile: sf,
		}
	}

	nodesIngested, err := repo.BulkUpsertNodes(graphNodes)
	if err != nil {
		return fmt.Errorf("ingesting nodes: %w", err)
	}

	// Ingest edges
	edgesCreated := 0
	var warnings []string
	for _, se := range merged.Edges {
		edge := &db.GraphEdge{
			ID:         se.ID,
			SrcID:      se.SrcID,
			DstID:      se.DstID,
			Kind:       db.EdgeKind(se.Kind),
			Properties: se.Properties,
		}
		if err := repo.InsertEdge(edge); err != nil {
			warnings = append(warnings, fmt.Sprintf("edge %s: %v", se.ID, err))
		} else {
			edgesCreated++
		}
	}

	// Collect warning messages from scanner output
	for _, sw := range merged.Warnings {
		warnings = append(warnings, sw.Message)
	}

	result := scanResult{
		NodesIngested: nodesIngested,
		EdgesCreated:  edgesCreated,
		Warnings:      warnings,
		Errors:        merged.Errors,
		Stats:         merged.Stats,
	}

	jsonFlag, _ := cmd.Flags().GetBool("json")
	if jsonFlag {
		return PrintJSON(w, result)
	}

	Info(w, "Scan complete.")
	Info(w, fmt.Sprintf("  Nodes ingested: %d", result.NodesIngested))
	Info(w, fmt.Sprintf("  Edges created: %d", result.EdgesCreated))
	Info(w, fmt.Sprintf("  Warnings: %d", len(result.Warnings)))
	Info(w, fmt.Sprintf("  Errors: %d", len(result.Errors)))

	if verbose && len(result.Warnings) > 0 {
		Info(w, "\nWarnings:")
		for _, warning := range result.Warnings {
			Warn(w, warning)
		}
	}

	if verbose && len(result.Errors) > 0 {
		Info(w, "\nErrors:")
		for _, e := range result.Errors {
			Warn(w, fmt.Sprintf("[%s] %s", e.ScannerID, e.Error))
		}
	}

	return nil
}

// resolveConfigPath derives the config.yaml path from the database path.
// If dbPath ends with "abacus.db", replaces it with "config.yaml".
// Otherwise falls back to the default config path.
func resolveConfigPath(dbFilePath string) string {
	if strings.HasSuffix(dbFilePath, "abacus.db") {
		return filepath.Join(filepath.Dir(dbFilePath), "config.yaml")
	}
	return config.DefaultPath()
}
