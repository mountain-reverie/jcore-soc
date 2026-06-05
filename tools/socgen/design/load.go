package design

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Load reads and decodes a YAML board spec at path into a Design. (Includes and
// merge are layered in Task 2; this version decodes a single self-contained file.)
func Load(path string) (*Design, []error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, []error{fmt.Errorf("read %s: %w", path, err)}
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, []error{fmt.Errorf("%s: %w", path, err)}
	}
	// doc is a DocumentNode; its single Content child is the root mapping.
	root := &doc
	if doc.Kind == yaml.DocumentNode && len(doc.Content) == 1 {
		root = doc.Content[0]
	}
	if err := resolveTree(root, filepath.Dir(path), nil); err != nil {
		return nil, []error{fmt.Errorf("%s: %w", path, err)}
	}
	d := &Design{}
	if err := root.Decode(d); err != nil {
		return nil, []error{fmt.Errorf("%s: %w", path, err)}
	}
	return d, nil
}
