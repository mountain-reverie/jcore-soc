package design

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// socGenRelPath is the path from a board's design.yaml dir (targets/boards/<name>)
// to the soc_gen working directory (targets/soc_gen). A pins `file:` value is
// relative to soc_gen, matching the original Clojure tool run as `cd targets/soc_gen`.
const socGenRelPath = "../../soc_gen"

// Load reads and decodes a YAML board spec at path into a Design, resolving
// !include/merge directives relative to the spec's directory. If the spec has a
// pins: block with a non-empty file:, Load also parses the referenced .pins file
// (resolved relative to soc_gen, see socGenRelPath); any .pins read/parse errors
// are returned alongside the (still non-nil) Design.
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
		pinPath := filepath.Join(filepath.Dir(path), socGenRelPath, d.Pins.File)
		data, err := os.ReadFile(pinPath)
		if err != nil {
			errs = append(errs, fmt.Errorf("read pins %s: %w", pinPath, err))
		} else {
			// type defaults to pin-list (EAGLE); only "pin-names" selects the
			// simple NAME-PAD parser — faithful to the Clojure :or {type :pin-list}.
			var perrs []error
			if d.Pins.Type == "pin-names" {
				d.Pins.Pins, perrs = parsePinNames(data)
			} else {
				d.Pins.Pins, perrs = parsePinList(data, d.Pins.Part)
			}
			errs = append(errs, perrs...)
		}
	}
	return d, errs
}
