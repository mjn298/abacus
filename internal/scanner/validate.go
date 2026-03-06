package scanner

import "fmt"

// validNodeKinds is the set of allowed node kinds.
var validNodeKinds = map[string]bool{
	"route":      true,
	"entity":     true,
	"page":       true,
	"action":     true,
	"permission": true,
}

// validEdgeKinds is the set of allowed edge kinds.
var validEdgeKinds = map[string]bool{
	"uses_route":          true,
	"touches_entity":      true,
	"on_page":             true,
	"requires_permission": true,
	"relates_to":          true,
	"field_relation":      true,
}

// ValidateOutput checks a ScanOutput for protocol compliance and returns
// a list of human-readable validation error strings. An empty slice means
// the output is valid.
func ValidateOutput(output *ScanOutput) []string {
	var errs []string

	if output.Version != 1 {
		errs = append(errs, fmt.Sprintf("version must be 1, got %d", output.Version))
	}

	if output.Scanner.ID == "" {
		errs = append(errs, "scanner.id must be non-empty")
	}

	nodeIDs := make(map[string]bool, len(output.Nodes))

	for i, node := range output.Nodes {
		if node.ID == "" {
			errs = append(errs, fmt.Sprintf("node[%d]: id must be non-empty", i))
		} else {
			if nodeIDs[node.ID] {
				errs = append(errs, fmt.Sprintf("node[%d]: duplicate node id %q", i, node.ID))
			}
			nodeIDs[node.ID] = true
		}

		if node.Kind == "" {
			errs = append(errs, fmt.Sprintf("node[%d]: kind must be non-empty", i))
		} else if !validNodeKinds[node.Kind] {
			errs = append(errs, fmt.Sprintf("node[%d]: invalid kind %q", i, node.Kind))
		}

		if node.Name == "" {
			errs = append(errs, fmt.Sprintf("node[%d]: name must be non-empty", i))
		}

		if node.Label == "" {
			errs = append(errs, fmt.Sprintf("node[%d]: label must be non-empty", i))
		}
	}

	for i, edge := range output.Edges {
		if !validEdgeKinds[edge.Kind] {
			errs = append(errs, fmt.Sprintf("edge[%d]: invalid kind %q", i, edge.Kind))
		}

		if !nodeIDs[edge.SrcID] {
			errs = append(errs, fmt.Sprintf("edge[%d]: srcId %q does not reference a known node", i, edge.SrcID))
		}

		if !nodeIDs[edge.DstID] {
			errs = append(errs, fmt.Sprintf("edge[%d]: dstId %q does not reference a known node", i, edge.DstID))
		}
	}

	return errs
}
