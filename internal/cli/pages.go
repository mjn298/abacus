package cli

import (
	"github.com/mjn/abacus/internal/db"
)

func init() {
	rootCmd.AddCommand(queryNodesCmd(db.NodePage, "pages", "List or search page nodes"))
}
