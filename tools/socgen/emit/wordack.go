package emit

import (
	"fmt"
	"strings"

	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

// renderArchDecls prints declarations at architecture-declaration indent by
// wrapping them in a throwaway architecture and slicing out the decl region
// (between the `is` line and `begin`). Used to reuse the exact device_t enum and
// decode_address body text in the templated word_ack_gen.vhd.
func renderArchDecls(decls []vhdl.Decl) string {
	df := &vhdl.DesignFile{Units: []vhdl.DesignUnit{
		&vhdl.ArchitectureBody{Name: "impl", Entity: "x", Decls: decls},
	}}
	out := vhdl.Print(df)
	// out is: "architecture impl of x is\n<decls>\nbegin\nend;\n"; return just the
	// <decls> region. The markers are always present for a non-empty decl list, but
	// guard defensively so a printer change degrades gracefully rather than panics.
	_, rest, ok := strings.Cut(out, "architecture impl of x is\n")
	if !ok {
		return ""
	}
	decl, _, ok := strings.Cut(rest, "\nbegin\n")
	if !ok {
		return ""
	}
	return decl // the decl lines, no trailing newline
}

// WordAckGen renders word_ack_gen.vhd for a board with #bus_word devices, a
// faithful reproduction of phase-2 awk_p02_01_wordack. Returns "" when the board
// has no bus-word devices. Templated (not AST-printed): the awk hand-formatting
// (port padding, the wrapped decode signature, the 3-per-line sensitivity list)
// is not the AST printer's style. The device_t enum and decode_address body are
// reused from the data-bus emit so they stay byte-identical to devices.vhd.
func WordAckGen(res *elaborate.Resolution) (string, error) {
	if res == nil || len(res.BusWord) == 0 || res.DataBus == nil {
		return "", nil
	}
	devs := busDevicesOf(res)
	lits := busLits(res)

	enumLits := []string{"NONE"}
	for _, d := range devs {
		enumLits = append(enumLits, devLit(d.Name))
	}
	deviceT := renderArchDecls([]vhdl.Decl{
		&vhdl.TypeDecl{Name: "device_t", Def: &vhdl.EnumDef{Lits: enumLits}},
	})

	// Reuse only the decode function's body (begin..end;); the awk renames the
	// function and wraps its signature, so the signature line is templated below.
	decodeFull := renderArchDecls([]vhdl.Decl{decodeFunction(devs, lits, res.DataBus.DecodeMode == "simple")})
	_, body, ok := strings.Cut(decodeFull, "    begin\n")
	if !ok {
		return "", nil
	}
	body = "    begin\n" + body // "    begin\n …\n    end;"

	var b strings.Builder
	b.WriteString(fileBanner)
	b.WriteString("library ieee;\n")
	b.WriteString("use ieee.std_logic_1164.all;\n\n")
	b.WriteString("entity word_ack_gen is\n")
	b.WriteString("  port (\n")
	b.WriteString("    adr : in std_logic_vector( 31 downto 0);\n")
	b.WriteString("    word_bus_en : in std_logic ;\n")
	for _, name := range res.BusWord {
		fmt.Fprintf(&b, "    %-20s : in std_logic ;\n", "ack_thru_in_"+name)
	}
	b.WriteString("    word_ack : out std_logic\n")
	b.WriteString("  );\n")
	b.WriteString("end entity;\n\n")
	b.WriteString("architecture impl of word_ack_gen is\n")
	b.WriteString(deviceT)
	b.WriteString("\n")
	b.WriteString("    function decode_address_wag (addr : \n")
	b.WriteString("      std_logic_vector(31 downto 0)) return device_t is\n")
	b.WriteString(body)
	b.WriteString("\n")
	b.WriteString("begin\n")
	b.WriteString("    ack_gen : process (\n")
	for i, name := range res.BusWord {
		if i%3 == 0 {
			b.WriteString("      ")
		}
		b.WriteString("ack_thru_in_")
		b.WriteString(name)
		if i == len(res.BusWord)-1 || i%3 == 2 {
			b.WriteString(",\n")
		} else {
			b.WriteString(", ")
		}
	}
	b.WriteString("      word_bus_en, adr\n")
	b.WriteString("      )\n")
	b.WriteString("      variable active_dev : device_t;\n")
	b.WriteString("    begin\n")
	b.WriteString("      active_dev := decode_address_wag ( adr );\n")
	b.WriteString("      case active_dev is\n")
	for _, name := range res.BusWord {
		fmt.Fprintf(&b, "        when %-12s => word_ack <= ack_thru_in_%s;\n", devLit(name), name)
	}
	b.WriteString("        when others       => word_ack <= word_bus_en;\n")
	b.WriteString("      end case;\n")
	b.WriteString("    end process;\n")
	b.WriteString("end impl;\n")
	return b.String(), nil
}
