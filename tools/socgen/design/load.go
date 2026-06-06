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
	var errs []error
	if d.Pins != nil && d.Pins.File != "" {
		// The .pins file path is relative to the soc_gen working directory
		// (targets/soc_gen), matching the original Clojure soc_gen which is run
		// as `cd targets/soc_gen; lein run`. design.yaml lives at
		// targets/boards/<name>/, so the soc_gen dir is ../../soc_gen from there.
		pinPath := filepath.Join(filepath.Dir(path), "..", "..", "soc_gen", d.Pins.File)
		data, err := os.ReadFile(pinPath)
		if err != nil {
			errs = append(errs, fmt.Errorf("read pins %s: %w", pinPath, err))
		} else {
			var perrs []error
			if d.Pins.Part != "" || d.Pins.Type == "pin-list" {
				d.Pins.Pins, perrs = parsePinList(data, d.Pins.Part)
			} else {
				d.Pins.Pins, perrs = parsePinNames(data)
			}
			errs = append(errs, perrs...)
		}
	}
	return d, errs
}
