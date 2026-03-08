package cli

import (
	"github.com/mjn/abacus/internal/db"
)

func init() {
	rootCmd.AddCommand(queryNodesCmd(db.NodeModule, "modules", "List or search module nodes"))
}
