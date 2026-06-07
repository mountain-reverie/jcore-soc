package elaborate

import (
	"fmt"
	"strconv"

	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// evalInt evaluates a vhdl integer expression with generic substitution.
// ok==false means the expression cannot be reduced to a concrete int (symbolic).
// Supports integer +,-,* and unary +,-; generic names resolve via env (lower-cased).
func evalInt(e vhdl.Expr, env map[string]int64) (int64, bool) {
	switch n := e.(type) {
	case *vhdl.BasicLit:
		if n.Kind == vhdl.INT {
			v, err := strconv.ParseInt(n.Value, 10, 64)
			if err != nil {
				return 0, false
			}
			return v, true
		}
		return 0, false
	case *vhdl.Ident:
		v, ok := env[lc(n.Name)]
		return v, ok
	case *vhdl.ParenExpr:
		return evalInt(n.X, env)
	case *vhdl.UnaryExpr:
		v, ok := evalInt(n.X, env)
		if !ok {
			return 0, false
		}
		switch n.Op {
		case vhdl.MINUS:
			return -v, true
		case vhdl.PLUS:
			return v, true
		}
		return 0, false
	case *vhdl.BinaryExpr:
		l, lo := evalInt(n.X, env)
		r, ro := evalInt(n.Y, env)
		if !lo || !ro {
			return 0, false
		}
		switch n.Op {
		case vhdl.PLUS:
			return l + r, true
		case vhdl.MINUS:
			return l - r, true
		case vhdl.STAR:
			return l * r, true
		}
		return 0, false // /, **, mod, etc. -> symbolic
	}
	return 0, false
}

// ResolvedType is a port/signal type with generic-resolved bounds where possible.
type ResolvedType struct {
	Mark        string
	Constraint  vhdl.Expr // as-written, kept when bounds are symbolic
	Left, Right *int      // concrete bounds when the evaluator reduced them
	Dir         string    // "to"|"downto" when Left/Right are set
}

func (t *ResolvedType) String() string {
	if t == nil {
		return ""
	}
	if t.Left != nil && t.Right != nil {
		return fmt.Sprintf("%s(%d %s %d)", t.Mark, *t.Left, t.Dir, *t.Right)
	}
	if t.Constraint != nil {
		return vhdl.SubtypeString(t.Mark, t.Constraint)
	}
	return t.Mark
}

// resolveType resolves a type mark + optional constraint expr using env.
// Handles the common vector case: ParenExpr wrapping a Range whose bounds
// evaluate to integers. Otherwise keeps the constraint as-written (symbolic).
func resolveType(mark string, constraint vhdl.Expr, env map[string]int64) *ResolvedType {
	rt := &ResolvedType{Mark: mark, Constraint: constraint}
	if constraint == nil {
		return rt
	}
	pe, ok := constraint.(*vhdl.ParenExpr)
	if !ok {
		return rt
	}
	rng, ok := pe.X.(*vhdl.Range)
	if !ok {
		return rt
	}
	l, lo := evalInt(rng.Left, env)
	r, ro := evalInt(rng.Right, env)
	if lo && ro {
		li, ri := int(l), int(r)
		var dir string
		switch rng.Dir {
		case vhdl.DOWNTO:
			dir = dirDownto
		case vhdl.TO:
			dir = "to"
		default:
			return rt // unrecognized direction; keep symbolic
		}
		return &ResolvedType{Mark: mark, Left: &li, Right: &ri, Dir: dir}
	}
	return rt
}
