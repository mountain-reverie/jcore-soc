package cheader

import (
	"sort"
	"strconv"
	"strings"

	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
)

type cReg struct {
	name           string
	lo, hi         int
	width          int
	mode           string
	ignore         bool
	expanded       bool
	origLo, origHi int
}

// classRegStruct renders `struct <name>_regs { … };` for a class's registers,
// faithful to c_header.clj class-reg-struct: sub-4-byte regs are expanded to
// aligned words where there is room, gaps become ignoreN fields, and an
// unalignable class returns ("", false). Operates on a copy; never mutates regs.
func classRegStruct(name string, regs []*elaborate.ResolvedReg) (string, bool) {
	if len(regs) == 0 {
		return "", false
	}
	ex := make([]cReg, len(regs))
	for i, rg := range regs {
		c := cReg{name: rg.Name, lo: rg.ByteRange[0], hi: rg.ByteRange[1], width: rg.Width, mode: rg.Mode, origLo: rg.ByteRange[0], origHi: rg.ByteRange[1]}
		if rg.Width < 4 {
			leftBoundary := -1
			if i > 0 {
				leftBoundary = regs[i-1].ByteRange[1]
			}
			rightBoundary := 0xffffffff
			if i < len(regs)-1 {
				rightBoundary = regs[i+1].ByteRange[0]
			}
			left, right := rg.ByteRange[0], rg.ByteRange[1]
			leftSpace := left % 4
			rightSpace := 3 - (right % 4)
			if leftSpace > 0 && leftSpace <= left-leftBoundary-1 {
				c.lo = left - leftSpace
				c.width += leftSpace
				c.expanded = true
			}
			// Keep the left-expanded c.lo (cumulative). NOTE: the Clojure reference
			// rewrites lo back to the original `left` here, so a both-ways expansion
			// fails alignment there; we keep it, which is correct. No mimas register
			// expands on both sides (all sub-word regs sit at byte 3 → rightSpace 0).
			if c.width < 4 && rightSpace > 0 && rightSpace <= rightBoundary-right-1 {
				c.hi = right + rightSpace
				c.width += rightSpace
				c.expanded = true
			}
		}
		ex[i] = c
	}
	var all []cReg
	if ex[0].lo > 0 {
		all = append(all, cReg{lo: 0, hi: ex[0].lo - 1, width: ex[0].lo, ignore: true})
	}
	all = append(all, ex...)
	for i := 0; i+1 < len(ex); i++ {
		a, b := ex[i].hi+1, ex[i+1].lo-1
		if a <= b {
			all = append(all, cReg{lo: a, hi: b, width: b - a + 1, ignore: true})
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].lo < all[j].lo })
	for _, c := range all {
		if c.lo%4 != 0 || c.hi%4 != 3 {
			return "", false
		}
	}
	var b strings.Builder
	b.WriteString("struct " + name + "_regs {\n")
	ign := 0
	for _, c := range all {
		field := c.name
		if c.ignore {
			field = "ignore" + strconv.Itoa(ign)
			ign++
		}
		b.WriteString("  uint32_t " + field)
		if n := c.width / 4; n > 1 {
			b.WriteString("[" + strconv.Itoa(n) + "]")
		}
		b.WriteString(";")
		if cmt := regComment(c); cmt != "" {
			b.WriteString(" //" + cmt)
		}
		b.WriteString("\n")
	}
	b.WriteString("};")
	return b.String(), true
}

func regComment(c cReg) string {
	s := ""
	if c.mode == "read" {
		s += " read-only"
	}
	if c.expanded {
		loOff, hiOff := c.origLo-c.lo, c.origHi-c.lo
		if loOff == hiOff {
			s += " only byte " + strconv.Itoa(loOff)
		} else {
			s += " only bytes " + strconv.Itoa(loOff) + "-" + strconv.Itoa(hiOff)
		}
	}
	return s
}
