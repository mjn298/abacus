package cli

import (
	"github.com/mjn/abacus/internal/db"
)

func init() {
	rootCmd.AddCommand(queryNodesCmd(db.NodeRoute, "routes", "List or search route nodes"))
}
