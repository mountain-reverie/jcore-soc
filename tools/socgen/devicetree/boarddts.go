package devicetree

import (
	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/dts"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
)

// BoardDTS renders the board.dts Device Tree Source for a board.
func BoardDTS(b *board.Board, res *elaborate.Resolution) (string, error) {
	root, err := DeviceTree(b, res)
	if err != nil {
		return "", err
	}
	return dts.Print(root), nil
}
