package cli

import (
	"github.com/mjn/abacus/internal/db"
)

func init() {
	rootCmd.AddCommand(queryNodesCmd(db.NodeEntity, "entities", "List or search entity nodes"))
}
