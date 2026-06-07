package board

import (
	"fmt"
	"os"

	"github.com/j-core/jcore-soc/tools/socgen/iface"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// Library parses every file in files and extracts an iface.Library. The build
// already cpp-preprocesses cpp sources into .vhh, so every listed file is plain
// VHDL parsed with vhdl.ParseFile (no WithCPP). Best-effort: a file that fails
// to read or parse is recorded in the returned []error (each prefixed "read "/
// "parse ") and skipped; the rest still extract. iface.Extract's own dedup
// notes (duplicate symbols across a real multi-file board are expected) are NOT
// returned here — P4a's contract is parse-strictness. Never panics.
func Library(files []string) (*iface.Library, []error) {
	var dfs []*vhdl.DesignFile
	var errs []error
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			errs = append(errs, fmt.Errorf("read %s: %w", f, err))
			continue
		}
		df, perr := vhdl.ParseFile(vhdl.NewFileSet(), f, src)
		if perr != nil {
			errs = append(errs, fmt.Errorf("parse %s: %v", f, perr))
			continue
		}
		dfs = append(dfs, df)
	}
	lib, _ := iface.Extract(dfs) // dedup notes intentionally dropped (see doc)
	return lib, errs
}
