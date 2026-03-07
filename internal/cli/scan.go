package cli

import (
	"context"
	"fmt"
	"os"
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

type scannerEntry struct {
	id  string
	cfg config.ScannerConfig
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

	// Determine output mode early so we can gate progress output
	jsonFlag, _ := cmd.Flags().GetBool("json")
	showProgress := !quiet && !jsonFlag

	// Build scanner configs, optionally filtering by type
	var filterType string
	if len(args) > 0 {
		filterType = args[0]
	}

	// Separate scanners into scan-phase and link-phase lists
	var scanPhaseConfigs []scannerEntry
	var linkPhaseConfigs []scannerEntry
	for id, sc := range cfg.Scanners {
		if filterType != "" && id != filterType {
			continue
		}
		phase := sc.Phase
		if phase == "" {
			phase = "scan"
		}
		entry := scannerEntry{id: id, cfg: sc}
		if phase == "link" {
			linkPhaseConfigs = append(linkPhaseConfigs, entry)
		} else {
			scanPhaseConfigs = append(scanPhaseConfigs, entry)
		}
	}

	if filterType != "" && len(scanPhaseConfigs)+len(linkPhaseConfigs) == 0 {
		return fmt.Errorf("no scanner found with ID %q", filterType)
	}

	if len(scanPhaseConfigs)+len(linkPhaseConfigs) == 0 {
		return fmt.Errorf("no scanners configured; check %s", configPath)
	}

	// Run scan-phase scanners
	runner := scanner.NewRunner(60 * time.Second)
	ctx := context.Background()
	merged := &scanner.MergedScanOutput{}

	for _, entry := range scanPhaseConfigs {
		id, sc := entry.id, entry.cfg

		if showProgress {
			fmt.Fprintf(os.Stderr, "Scanning with %s...", id)
		}

		opts := sc.Options
		if opts == nil {
			opts = map[string]interface{}{}
		}
		input := scanner.ScanInput{
			Version:     1,
			ProjectRoot: cfg.Project.Root,
			Options:     opts,
			IgnorePaths: cfg.Project.IgnorePaths,
		}

		out, err := runner.RunScanner(ctx, sc.Command, input, nil)
		if err != nil {
			if showProgress {
				fmt.Fprintf(os.Stderr, " error\n")
			}
			merged.Errors = append(merged.Errors, scanner.ScannerError{
				ScannerID: sc.Command,
				Error:     err.Error(),
			})
			merged.Stats.TotalErrors++
			continue
		}

		if showProgress {
			fmt.Fprintf(os.Stderr, " done (%d nodes, %d edges, %dms)\n",
				out.Stats.NodesFound, out.Stats.EdgesFound, out.Stats.DurationMs)
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

	// Open DB and ingest
	database, err := db.OpenDB(dbPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer database.Close()

	repo := graph.NewGraphRepository(database)

	// Ingest scan-phase nodes
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

	if showProgress {
		fmt.Fprintf(os.Stderr, "Ingesting %d nodes...", len(graphNodes))
	}
	nodesIngested, err := repo.BulkUpsertNodes(graphNodes)
	if err != nil {
		return fmt.Errorf("ingesting nodes: %w", err)
	}
	if showProgress {
		fmt.Fprintf(os.Stderr, " done\n")
	}

	// Ingest scan-phase edges
	if showProgress {
		fmt.Fprintf(os.Stderr, "Ingesting %d edges...", len(merged.Edges))
	}
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
	if showProgress {
		fmt.Fprintf(os.Stderr, " done (%d created, %d warnings)\n", edgesCreated, len(warnings))
	}

	// Run link-phase scanners
	if len(linkPhaseConfigs) > 0 {
		var existingNodes []scanner.ScanNodeRef

		if len(scanPhaseConfigs) > 0 {
			for _, sn := range merged.Nodes {
				if sn.Kind == "route" || sn.Kind == "entity" {
					existingNodes = append(existingNodes, scanner.ScanNodeRef{
						ID:         sn.ID,
						Kind:       sn.Kind,
						Name:       sn.Name,
						SourceFile: sn.SourceFile,
					})
				}
			}
		} else {
			existingNodes, err = repo.GetNodeRefsByKinds([]db.NodeKind{db.NodeRoute, db.NodeEntity})
			if err != nil {
				return fmt.Errorf("querying existing nodes for linker: %w", err)
			}
		}

		knownNodeIDs := make(map[string]bool, len(existingNodes))
		for _, ref := range existingNodes {
			knownNodeIDs[ref.ID] = true
		}

		linkRunner := scanner.NewRunner(5 * time.Minute)

		for _, entry := range linkPhaseConfigs {
			id, sc := entry.id, entry.cfg

			if showProgress {
				fmt.Fprintf(os.Stderr, "Linking with %s...", id)
			}

			opts := sc.Options
			if opts == nil {
				opts = map[string]interface{}{}
			}
			input := scanner.ScanInput{
				Version:       1,
				ProjectRoot:   cfg.Project.Root,
				Options:       opts,
				IgnorePaths:   cfg.Project.IgnorePaths,
				ExistingNodes: existingNodes,
			}

			out, err := linkRunner.RunScanner(ctx, sc.Command, input, knownNodeIDs)
			if err != nil {
				if showProgress {
					fmt.Fprintf(os.Stderr, " error\n")
				}
				merged.Errors = append(merged.Errors, scanner.ScannerError{
					ScannerID: sc.Command,
					Error:     err.Error(),
				})
				merged.Stats.TotalErrors++
				continue
			}

			if showProgress {
				fmt.Fprintf(os.Stderr, " done (%d edges, %dms)\n",
					out.Stats.EdgesFound, out.Stats.DurationMs)
			}

			merged.Warnings = append(merged.Warnings, out.Warnings...)
			merged.Stats.TotalEdgesFound += out.Stats.EdgesFound
			merged.Stats.TotalErrors += out.Stats.Errors
			merged.Stats.ScannerCount++

			if _, err := repo.DeleteEdgesBySourceScanner(id); err != nil {
				return fmt.Errorf("deleting stale edges for scanner %s: %w", id, err)
			}

			linkEdges := make([]db.GraphEdge, len(out.Edges))
			for i, se := range out.Edges {
				scannerID := id
				linkEdges[i] = db.GraphEdge{
					ID:            se.ID,
					SrcID:         se.SrcID,
					DstID:         se.DstID,
					Kind:          db.EdgeKind(se.Kind),
					Properties:    se.Properties,
					SourceScanner: &scannerID,
				}
			}

			if showProgress {
				fmt.Fprintf(os.Stderr, "Ingesting %d linker edges...", len(linkEdges))
			}
			linkerEdgesCreated, err := repo.BulkUpsertEdges(linkEdges)
			if err != nil {
				return fmt.Errorf("ingesting linker edges for %s: %w", id, err)
			}
			edgesCreated += linkerEdgesCreated
			if showProgress {
				fmt.Fprintf(os.Stderr, " done\n")
			}
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
