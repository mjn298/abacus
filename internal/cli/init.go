package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mjn/abacus/internal/config"
	"github.com/mjn/abacus/internal/db"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var initDir string

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize Abacus for the current project",
	Long:  "Creates the .abacus/ directory, generates a config file with auto-detected project settings, and initializes the SQLite database.",
	RunE:  runInit,
}

var initForce bool

func init() {
	initCmd.Flags().StringVar(&initDir, "dir", ".", "Project directory to initialize")
	initCmd.Flags().BoolVar(&initForce, "force", false, "Re-detect scanners and overwrite config")
	rootCmd.AddCommand(initCmd)
}

// initResult holds the result of initialization for JSON output.
type initResult struct {
	Project     string   `json:"project"`
	ProjectType string   `json:"project_type"`
	ConfigPath  string   `json:"config_path"`
	DBPath      string   `json:"db_path"`
	Scanners    []string `json:"scanners"`
}

func runInit(cmd *cobra.Command, args []string) error {
	dir := initDir
	abacusDir := filepath.Join(dir, ".abacus")
	configPath := filepath.Join(abacusDir, "config.yaml")
	dbFilePath := filepath.Join(abacusDir, "abacus.db")

	w := cmd.OutOrStdout()

	// Check if already initialized
	if _, err := os.Stat(configPath); err == nil && !initForce {
		Warn(w, "Abacus is already initialized in this directory (use --force to re-detect)")
		return nil
	}

	// Create .abacus/ directory
	if err := os.MkdirAll(abacusDir, 0755); err != nil {
		return fmt.Errorf("creating .abacus directory: %w", err)
	}

	// Auto-detect project type
	projectName, projectType, scanners := detectProject(dir)

	// Generate config
	cfg := config.Config{
		Version: 1,
		Project: config.ProjectConfig{
			Name: projectName,
			Root: ".",
			IgnorePaths: []string{
				"node_modules",
				"dist",
				"build",
				".git",
			},
		},
		Scanners: scanners,
	}

	cfgData, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(configPath, cfgData, 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	// Initialize database
	database, err := db.OpenDB(dbFilePath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer database.Close()

	if err := db.InitSchema(database); err != nil {
		return fmt.Errorf("initializing schema: %w", err)
	}

	// Update .gitignore
	if err := updateGitignore(dir); err != nil {
		return fmt.Errorf("updating .gitignore: %w", err)
	}

	// Output
	result := initResult{
		Project:     projectName,
		ProjectType: projectType,
		ConfigPath:  configPath,
		DBPath:      dbFilePath,
		Scanners:    scannerNames(scanners),
	}

	jsonFlag, _ := cmd.Flags().GetBool("json")
	if jsonFlag {
		return PrintJSON(w, result)
	}

	Info(w, fmt.Sprintf("Initialized Abacus for project %q (%s)", result.Project, result.ProjectType))
	Info(w, fmt.Sprintf("  Config: %s", result.ConfigPath))
	Info(w, fmt.Sprintf("  Database: %s", result.DBPath))
	if len(result.Scanners) > 0 {
		Info(w, fmt.Sprintf("  Scanners: %s", strings.Join(result.Scanners, ", ")))
	}

	return nil
}

// resolveScannersPath finds the scanners directory.
// Checks ABACUS_SCANNERS_PATH env var, then tries to find scanners/
// relative to the running binary.
func resolveScannersPath() string {
	// 1. Env var override
	if p := os.Getenv("ABACUS_SCANNERS_PATH"); p != "" {
		return p
	}

	// 2. Relative to executable
	if exe, err := os.Executable(); err == nil {
		exe, _ = filepath.EvalSymlinks(exe)
		// Binary might be in repo root or ~/go/bin
		// Check: binary_dir/scanners/ and binary_dir/../scanners/
		for _, candidate := range []string{
			filepath.Join(filepath.Dir(exe), "scanners"),
			filepath.Join(filepath.Dir(exe), "..", "scanners"),
		} {
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				abs, _ := filepath.Abs(candidate)
				return abs
			}
		}
	}

	return ""
}

// scannerCommand builds the command string for a scanner.
func scannerCommand(scannersBase, scannerName string) string {
	if scannersBase != "" {
		return fmt.Sprintf("node %s", filepath.Join(scannersBase, scannerName, "dist", "index.js"))
	}
	return fmt.Sprintf("node scanners/%s/dist/index.js", scannerName)
}

// extractDeps collects all dependency names from package.json.
func extractDeps(pkg map[string]interface{}) map[string]bool {
	deps := make(map[string]bool)
	for _, key := range []string{"dependencies", "devDependencies", "peerDependencies"} {
		if d, ok := pkg[key].(map[string]interface{}); ok {
			for name := range d {
				deps[name] = true
			}
		}
	}
	return deps
}

// detectProject inspects the directory for common project files and returns
// a project name, type string, and any auto-detected scanner configs.
func detectProject(dir string) (name string, projectType string, scanners map[string]config.ScannerConfig) {
	scanners = make(map[string]config.ScannerConfig)
	name = filepath.Base(dir)
	if name == "." || name == "/" {
		if wd, err := os.Getwd(); err == nil {
			name = filepath.Base(wd)
		} else {
			name = "project"
		}
	}
	projectType = "unknown"

	// Resolve scanners base path for generating commands
	scannersBase := resolveScannersPath()

	// Check for Go project
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		projectType = "go"
		// Try to extract module name
		if data, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "module ") {
					parts := strings.Fields(line)
					if len(parts) >= 2 {
						// Use last path segment as project name
						modParts := strings.Split(parts[1], "/")
						name = modParts[len(modParts)-1]
					}
					break
				}
			}
		}
	}

	// Read package.json dependencies from root and immediate subdirectories
	// (monorepo support: backend/, frontend/, etc.)
	deps := make(map[string]bool)
	if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
		projectType = "node"
		if data, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil {
			var pkg map[string]interface{}
			if err := json.Unmarshal(data, &pkg); err == nil {
				if n, ok := pkg["name"].(string); ok && n != "" {
					name = n
				}
				for k, v := range extractDeps(pkg) {
					deps[k] = v
				}
			}
		}
	}
	// Scan immediate subdirectories for additional package.json files
	if entries, err := os.ReadDir(dir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || entry.Name() == "node_modules" {
				continue
			}
			subPkg := filepath.Join(dir, entry.Name(), "package.json")
			if data, err := os.ReadFile(subPkg); err == nil {
				projectType = "node"
				var pkg map[string]interface{}
				if err := json.Unmarshal(data, &pkg); err == nil {
					for k, v := range extractDeps(pkg) {
						deps[k] = v
					}
				}
			}
		}
	}

	// Detect Express
	if deps["express"] {
		scanners["express"] = config.ScannerConfig{
			Command: scannerCommand(scannersBase, "express"),
			Options: map[string]interface{}{
				"routeGlobs": []string{"**/*.ts", "**/*.js"},
			},
		}
	}

	// Detect Prisma schema (check root and immediate subdirectories)
	prismaFound := false
	prismaLocations := []string{
		filepath.Join(dir, "prisma", "schema.prisma"),
		filepath.Join(dir, "schema.prisma"),
	}
	// Also check subdirs like backend/prisma/schema.prisma
	if entries, err := os.ReadDir(dir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") && entry.Name() != "node_modules" {
				prismaLocations = append(prismaLocations,
					filepath.Join(dir, entry.Name(), "prisma", "schema.prisma"),
					filepath.Join(dir, entry.Name(), "schema.prisma"),
				)
			}
		}
	}
	for _, loc := range prismaLocations {
		if _, err := os.Stat(loc); err == nil {
			opts := map[string]interface{}{}
			// Store schema path if it's not the default location
			relPath, relErr := filepath.Rel(dir, loc)
			if relErr == nil && relPath != filepath.Join("prisma", "schema.prisma") {
				opts["schemaPath"] = relPath
			}
			scanners["prisma"] = config.ScannerConfig{
				Command: scannerCommand(scannersBase, "prisma"),
				Options: opts,
			}
			prismaFound = true
			_ = prismaFound
			break
		}
	}

	// Detect React Router
	if deps["react-router"] || deps["react-router-dom"] {
		scanners["react-router"] = config.ScannerConfig{
			Command: scannerCommand(scannersBase, "react-router"),
		}
	}

	// Detect oRPC (any @orpc/ package indicates oRPC usage)
	orpcDetected := false
	for dep := range deps {
		if strings.HasPrefix(dep, "@orpc/") {
			orpcDetected = true
			break
		}
	}
	if orpcDetected {
		scanners["orpc"] = config.ScannerConfig{
			Command: scannerCommand(scannersBase, "orpc"),
			Options: map[string]interface{}{
				"contractGlobs": []string{"**/*.ts"},
			},
		}
	}

	return name, projectType, scanners
}

// updateGitignore adds .abacus/abacus.db to .gitignore if not already present.
func updateGitignore(dir string) error {
	gitignorePath := filepath.Join(dir, ".gitignore")
	entry := ".abacus/abacus.db"

	var existing []byte
	if data, err := os.ReadFile(gitignorePath); err == nil {
		existing = data
		// Check if already present
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) == entry {
				return nil // already present
			}
		}
	}

	// Append the entry
	var content string
	if len(existing) > 0 {
		s := string(existing)
		if !strings.HasSuffix(s, "\n") {
			s += "\n"
		}
		content = s + entry + "\n"
	} else {
		content = "# Abacus database (regenerated, not versioned)\n" + entry + "\n"
	}

	return os.WriteFile(gitignorePath, []byte(content), 0644)
}

// scannerNames returns the keys of the scanner config map.
func scannerNames(scanners map[string]config.ScannerConfig) []string {
	names := make([]string, 0, len(scanners))
	for k := range scanners {
		names = append(names, k)
	}
	return names
}
