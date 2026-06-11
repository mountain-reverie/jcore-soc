package emit

import (
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

func TestSortAssoc(t *testing.T) {
	named := []*vhdl.AssocElement{
		{Formal: "z", Actual: &vhdl.Ident{Name: "1"}},
		{Formal: "a", Actual: &vhdl.Ident{Name: "2"}},
		{Formal: "m", Actual: &vhdl.Ident{Name: "3"}},
	}
	sortAssoc(named)
	if named[0].Formal != "a" || named[1].Formal != "m" || named[2].Formal != "z" {
		t.Errorf("sortAssoc = [%s %s %s], want [a m z]", named[0].Formal, named[1].Formal, named[2].Formal)
	}
	// A list with a positional (formal-less) element is left unchanged — whether
	// the positional is at the start or in the middle (bail on the first
	// positional found, regardless of index).
	pos := []*vhdl.AssocElement{
		{Formal: "", Actual: &vhdl.Ident{Name: "1"}},
		{Formal: "a", Actual: &vhdl.Ident{Name: "2"}},
	}
	sortAssoc(pos)
	if pos[0].Formal != "" || pos[1].Formal != "a" {
		t.Errorf("sortAssoc reordered a positional-at-start list: %+v", pos)
	}
	mid := []*vhdl.AssocElement{
		{Formal: "b", Actual: &vhdl.Ident{Name: "1"}},
		{Formal: "", Actual: &vhdl.Ident{Name: "2"}},
		{Formal: "a", Actual: &vhdl.Ident{Name: "3"}},
	}
	sortAssoc(mid)
	if mid[0].Formal != "b" || mid[1].Formal != "" || mid[2].Formal != "a" {
		t.Errorf("sortAssoc reordered a positional-in-middle list: %+v", mid)
	}
}
