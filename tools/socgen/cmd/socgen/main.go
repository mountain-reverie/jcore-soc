// Command socgen generates a board's SoC file set (devices.vhd, soc.vhd,
// pad_ring.vhd, optional board.dts/board.h, build.mk).
//
// Usage: socgen [-root DIR] [-o OUTDIR] [-watch] <board>
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/generate"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "socgen:", err)
		os.Exit(1)
	}
}

// run parses args and generates the board's file set. Separated from main for
// testability.
func run(args []string) error {
	fs := flag.NewFlagSet("socgen", flag.ContinueOnError)
	root := fs.String("root", ".", "repository root (contains targets/boards)")
	outDir := fs.String("o", "", "output directory (default: the board's dir)")
	watch := fs.Bool("watch", false, "watch the board's yaml inputs and regenerate on change (Ctrl-C to stop)")
	variant := fs.String("variant", "", "board design variant (selects design.<variant>.yaml)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil // usage already printed by flag; exit 0
		}
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("expected exactly one board name")
	}
	name := fs.Arg(0)
	if *watch {
		return runWatch(*root, name, *outDir, *variant)
	}
	return generateBoard(*root, name, *outDir, *variant)
}

// generateBoard loads, elaborates, builds and writes one board's file set.
// Best-effort: load/elaborate/generate warnings print to stderr; only hard
// errors (unknown/duplicate plugin) and write failures are returned.
func generateBoard(root, name, outDir, variant string) error {
	b, lerr := board.Load(root, name, variant)
	if b == nil || b.Design == nil {
		return fmt.Errorf("load board %q: %w", name, lerr)
	}
	if lerr != nil {
		fmt.Fprintln(os.Stderr, "socgen: load notes:", lerr)
	}
	res, rerr := elaborate.Elaborate(b)
	if rerr != nil {
		fmt.Fprintln(os.Stderr, "socgen: elaborate notes:", rerr)
	}

	files, berr := generate.Build(b, res)
	if errors.Is(berr, generate.ErrUnknownPlugin) || errors.Is(berr, generate.ErrDuplicatePlugin) {
		return berr // hard error: abort before writing
	}
	if berr != nil {
		fmt.Fprintln(os.Stderr, "socgen: generate warnings:", berr)
	}

	dst := outDir
	if dst == "" {
		dst = filepath.Join(root, "targets/boards", name)
	}
	return generate.Write(files, dst)
}
