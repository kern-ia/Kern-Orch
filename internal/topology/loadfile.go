package topology

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/yoann/kern-orch/internal/graph"
)

// LoadFile reads a graph YAML from path and builds it, resolving `type: subgraph` nodes
// by loading their referenced files (relative to the referring file's directory). It
// guards against recursive subgraph references.
func LoadFile(path string, reg *Registry) (*graph.Graph, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("topology: resolve %q: %w", path, err)
	}
	return loadFile(abs, reg, map[string]bool{})
}

func loadFile(abs string, reg *Registry, inProgress map[string]bool) (*graph.Graph, error) {
	if inProgress[abs] {
		return nil, fmt.Errorf("topology: recursive subgraph reference at %q", abs)
	}
	inProgress[abs] = true
	defer delete(inProgress, abs)

	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("topology: read %q: %w", abs, err)
	}
	dir := filepath.Dir(abs)
	resolve := func(nodeID, ref string) (*graph.Graph, string, error) {
		childAbs := ref
		if !filepath.IsAbs(childAbs) {
			childAbs = filepath.Join(dir, ref)
		}
		sub, err := loadFile(childAbs, reg, inProgress)
		if err != nil {
			return nil, "", fmt.Errorf("topology: subgraph node %q -> %w", nodeID, err)
		}
		// The path travels back so a caller can describe the child's shape. The engine
		// never reads it.
		return sub, childAbs, nil
	}
	return build(data, reg, resolve)
}
