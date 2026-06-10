package emit

import (
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
)

func TestIRQDeclsAndAssigns(t *testing.T) {
	m := &elaborate.IRQModel{
		Signals: []elaborate.IRQSignal{
			{Name: "irqs0", Width: 8},
			{Name: "irq_0_5_a", Width: 0},
			{Name: "irq_0_5_b", Width: 0},
		},
		OrAssigns: []elaborate.IRQOrAssign{{Target: "irqs0(5)", Sources: []string{"irq_0_5_a", "irq_0_5_b"}}},
	}
	decls := renderDeclsForTest(t, irqDecls(m))
	for _, w := range []string{
		"signal irqs0 : std_logic_vector(7 downto 0) := (others => '0')",
		"signal irq_0_5_a : std_logic",
	} {
		if !strings.Contains(decls, w) {
			t.Errorf("irqDecls missing %q:\n%s", w, decls)
		}
	}
	stmts := renderPioStmts(t, irqAssigns(m))
	if got := strings.Join(strings.Fields(stmts), " "); !strings.Contains(got, "irqs0(5) <= irq_0_5_a or irq_0_5_b") {
		t.Errorf("OR-assign wrong:\n%s", stmts)
	}
}

func TestVectorNumbersAgg(t *testing.T) {
	vn := [8]int{0, 0x12, 0, 0x14, 0x15, 0, 0, 0}
	got := strings.Join(strings.Fields(renderExprStr(t, vectorNumbersAgg(vn))), " ")
	want := `(x"00", x"12", x"00", x"14", x"15", x"00", x"00", x"00")`
	if got != strings.Join(strings.Fields(want), " ") {
		t.Errorf("vector agg = %q, want %q", got, want)
	}
}
