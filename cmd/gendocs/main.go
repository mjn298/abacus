package main

import (
	"fmt"
	"log"
	"os"

	"github.com/mjn/abacus/internal/cli"
	"github.com/spf13/cobra/doc"
)

func main() {
	rootCmd := cli.RootCmd()

	// Disable the auto-generated completion command in docs.
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	manDir := "docs/man"
	mdDir := "docs/md"

	if err := os.MkdirAll(manDir, 0o755); err != nil {
		log.Fatalf("creating %s: %v", manDir, err)
	}
	if err := os.MkdirAll(mdDir, 0o755); err != nil {
		log.Fatalf("creating %s: %v", mdDir, err)
	}

	header := &doc.GenManHeader{
		Title:   "ABACUS",
		Section: "1",
	}
	if err := doc.GenManTree(rootCmd, header, manDir); err != nil {
		log.Fatalf("generating man pages: %v", err)
	}
	fmt.Println("Generated man pages in", manDir)

	if err := doc.GenMarkdownTree(rootCmd, mdDir); err != nil {
		log.Fatalf("generating markdown docs: %v", err)
	}
	fmt.Println("Generated markdown docs in", mdDir)
}
