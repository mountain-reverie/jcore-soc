package generate

import (
	"errors"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/cheader"
	"github.com/j-core/jcore-soc/tools/socgen/devicetree"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/emit"
)

// File is one generated artifact. InBuildMK marks the core VHDL files listed in
// build.mk (the Clojure :buildmk? true files); extra plugin files are false.
type File struct {
	Name      string // base name, e.g. "devices.vhd"
	Content   string
	InBuildMK bool
}

// knownPlugins is the recognized plugin set (faithful to the Clojure
// create-plugin case). aic1 is recognized but selects no file (IRQ wiring is
// unconditional in emit.Devices).
var knownPlugins = map[string]bool{"device_tree": true, "board.h": true, "aic1": true}

// Build assembles the full generated file set for a board. It validates the
// plugin list (duplicates and unknown names are errors), always emits the three
// core VHDL files, emits board.dts/board.h when their plugins are listed (in
// plugin-list order), and appends build.mk. Build is pure (no filesystem).
//
// On a plugin-validation error Build returns (nil, err) and emits nothing. On an
// emitter error Build is best-effort: it joins the error and continues, returning
// the files it could produce alongside a non-nil error.
func Build(b *board.Board, res *elaborate.Resolution) ([]File, error) {
	if err := validatePlugins(b.Design.Plugins); err != nil {
		return nil, err
	}
	var errs []error
	var files []File

	core := []struct {
		name string
		emit func() (string, error)
	}{
		{"devices.vhd", func() (string, error) { return emit.Devices(res) }},
		{"soc.vhd", func() (string, error) { return emit.SoC(res) }},
		{"pad_ring.vhd", func() (string, error) { return emit.PadRing(res) }},
	}
	for _, c := range core {
		content, err := c.emit()
		if err != nil {
			errs = append(errs, &GenerateError{Kind: ErrEmit, Name: c.name, Detail: err.Error()})
		} else if content == "" {
			// Empty content without an error is the "Failed to generate" warning
			// case (faithful to the Clojure add-warning); an emitter error already
			// covers the error case above.
			errs = append(errs, &GenerateError{Kind: ErrEmptyContent, Name: c.name})
		}
		files = append(files, File{Name: c.name, Content: content, InBuildMK: true})
	}

	for _, p := range b.Design.Plugins {
		switch p {
		case "device_tree":
			content, err := devicetree.BoardDTS(b, res)
			if err != nil {
				errs = append(errs, &GenerateError{Kind: ErrEmit, Name: "board.dts", Detail: err.Error()})
			}
			files = append(files, File{Name: "board.dts", Content: content})
		case "board.h":
			content, err := cheader.BoardH(res)
			if err != nil {
				errs = append(errs, &GenerateError{Kind: ErrEmit, Name: "board.h", Detail: err.Error()})
			}
			files = append(files, File{Name: "board.h", Content: content})
		}
	}

	files = append(files, File{Name: "build.mk", Content: buildMK(files)})
	return files, errors.Join(errs...)
}

// validatePlugins reports duplicate or unknown plugin names, joined. Each bad
// name is reported once, in input order (deterministic) — a name that is both
// unknown and duplicated yields one of each.
func validatePlugins(plugins []string) error {
	var errs []error
	seen := make(map[string]int, len(plugins))
	for _, p := range plugins {
		seen[p]++
	}
	reported := make(map[string]bool, len(plugins))
	for _, p := range plugins {
		if reported[p] {
			continue
		}
		reported[p] = true
		if !knownPlugins[p] {
			errs = append(errs, &GenerateError{Kind: ErrUnknownPlugin, Name: p})
		}
		if seen[p] > 1 {
			errs = append(errs, &GenerateError{Kind: ErrDuplicatePlugin, Name: p})
		}
	}
	return errors.Join(errs...)
}
