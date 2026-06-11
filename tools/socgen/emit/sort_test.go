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
	// A list with a positional (formal-less) element is left unchanged.
	pos := []*vhdl.AssocElement{
		{Formal: "", Actual: &vhdl.Ident{Name: "1"}},
		{Formal: "a", Actual: &vhdl.Ident{Name: "2"}},
	}
	sortAssoc(pos)
	if pos[0].Formal != "" || pos[1].Formal != "a" {
		t.Errorf("sortAssoc reordered a positional list: %+v", pos)
	}
}
