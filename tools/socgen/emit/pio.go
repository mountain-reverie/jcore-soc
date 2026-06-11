package emit

import (
	"strconv"

	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// pioStatements builds the system.pio loopback assignments: pi(idx) <= po(idx)
// for a loopback bit, pi(idx) <= '<const>' for a constant bit. Faithful to
// generate.clj pio-stmts.
func pioStatements(res *elaborate.Resolution) []vhdl.Stmt {
	stmts := make([]vhdl.Stmt, 0, len(res.Pio))
	for _, b := range res.Pio {
		idx := strconv.Itoa(b.Idx)
		pi := &vhdl.Ident{Name: "pi(" + idx + ")"}
		var rhs vhdl.Expr
		if b.Const != nil {
			rhs = &vhdl.BasicLit{Kind: vhdl.CHARLIT, Value: "'" + strconv.Itoa(*b.Const) + "'"}
		} else {
			rhs = &vhdl.Ident{Name: "po(" + idx + ")"}
		}
		if b.Name != "" {
			stmts = append(stmts, &vhdl.Comment{Text: b.Name})
		}
		stmts = append(stmts, concAssign(pi, rhs))
	}
	return stmts
}
