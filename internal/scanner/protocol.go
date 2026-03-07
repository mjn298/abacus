package scanner

// ScanNodeRef is a lightweight node reference sent to link-phase scanners.
// It intentionally excludes Properties to reduce payload size and avoid leaking scanner-extracted metadata.
type ScanNodeRef struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	SourceFile string `json:"sourceFile,omitempty"`
}

// ScanInput is the JSON payload sent to a scanner's stdin.
type ScanInput struct {
	Version       int            `json:"version"`
	ProjectRoot   string         `json:"projectRoot"`
	Options       map[string]any `json:"options"`
	IgnorePaths   []string       `json:"ignorePaths,omitempty"`
	ExistingNodes []ScanNodeRef  `json:"existingNodes,omitempty"`
}

// ScanOutput is the JSON payload a scanner writes to stdout.
type ScanOutput struct {
	Version  int           `json:"version"`
	Scanner  ScannerInfo   `json:"scanner"`
	Nodes    []ScanNode    `json:"nodes"`
	Edges    []ScanEdge    `json:"edges"`
	Warnings []ScanWarning `json:"warnings"`
	Stats    ScanStats     `json:"stats"`
}

// ScannerInfo identifies the scanner that produced the output.
type ScannerInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ScanNode represents a discovered node in the application graph.
type ScanNode struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	Name       string         `json:"name"`
	Label      string         `json:"label"`
	Properties map[string]any `json:"properties"`
	Source     string         `json:"source"`
	SourceFile string         `json:"sourceFile,omitempty"`
}

// ScanEdge represents a discovered edge between two nodes.
type ScanEdge struct {
	ID         string         `json:"id"`
	SrcID      string         `json:"srcId"`
	DstID      string         `json:"dstId"`
	Kind       string         `json:"kind"`
	Properties map[string]any `json:"properties,omitempty"`
}

// ScanWarning represents a non-fatal issue encountered during scanning.
type ScanWarning struct {
	File     string `json:"file"`
	Line     *int   `json:"line,omitempty"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
}

// ScanStats contains aggregate statistics from a scan run.
type ScanStats struct {
	FilesScanned int   `json:"filesScanned"`
	NodesFound   int   `json:"nodesFound"`
	EdgesFound   int   `json:"edgesFound"`
	Errors       int   `json:"errors"`
	DurationMs   int64 `json:"durationMs"`
}

// MergedScanOutput aggregates results from multiple scanner runs.
type MergedScanOutput struct {
	Nodes    []ScanNode
	Edges    []ScanEdge
	Warnings []ScanWarning
	Stats    MergedStats
	Errors   []ScannerError
}

// ScannerError captures a failure from a specific scanner.
type ScannerError struct {
	ScannerID string
	Error     string
}

// MergedStats aggregates statistics across multiple scanners.
type MergedStats struct {
	TotalFilesScanned int
	TotalNodesFound   int
	TotalEdgesFound   int
	TotalErrors       int
	ScannerCount      int
}
