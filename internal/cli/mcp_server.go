package cli

import (
	"github.com/mjn/abacus/mcp"
	"github.com/spf13/cobra"
)

var mcpConfigPath string

var mcpServerCmd = &cobra.Command{
	Use:   "mcp-server",
	Short: "Start MCP server for Claude Code integration",
	Long: `Starts an MCP (Model Context Protocol) server on stdio, exposing abacus tools for scanning, querying, matching, and managing the application graph.

Designed to be launched by Claude Code or other MCP-compatible clients. The
server communicates over stdin/stdout using the MCP JSON-RPC protocol.`,
	Example: `  $ abacus mcp-server
  $ abacus mcp-server --config .abacus/config.yaml
  $ abacus mcp-server --db ./custom/path/abacus.db`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath := mcpConfigPath
		if cfgPath == "" {
			cfgPath = ""
		}

		srv, err := mcp.NewAbacusServer(dbPath, cfgPath)
		if err != nil {
			return err
		}

		return srv.Run(cmd.Context())
	},
}

func init() {
	mcpServerCmd.Flags().StringVar(&mcpConfigPath, "config", "", "Path to abacus config file (default: .abacus/config.yaml)")
	rootCmd.AddCommand(mcpServerCmd)
}
