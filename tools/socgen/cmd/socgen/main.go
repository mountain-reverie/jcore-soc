// Command socgen generates a board's SoC file set (devices.vhd, soc.vhd,
// pad_ring.vhd, optional board.dts/board.h, build.mk).
//
// Usage: socgen [-root DIR] [-o OUTDIR] <board>
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

	b, lerr := board.Load(*root, name)
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

	dst := *outDir
	if dst == "" {
		dst = filepath.Join(*root, "targets/boards", name)
	}
	return generate.Write(files, dst)
}
