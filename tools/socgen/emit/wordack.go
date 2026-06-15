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
	// Sensitivity list: ack_thru_in_* signals, three per line. The trailing comma on
	// the last item (before the word_bus_en, adr line) is deliberate — awk-faithful
	// and VHDL-legal — not an off-by-one.
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

// phase2Devices applies the turtle phase-2 transforms (awk_p02_02_devices) to the
// rendered devices.vhd as a faithful text post-pass: (1) insert the byte-bus
// banner line after the "running soc_gen." line; (2) convert the no-dma cpus_mux
// from multi_master_bus_mux to multi_master_bus_muxff(a); (3) splice the
// word_ack_gen instance over the master read-back assignment. Only called for a
// board with #bus_word devices (res.BusWord non-empty).
func phase2Devices(s string, res *elaborate.Resolution) string {
	if res.DataBus == nil {
		// A word-ack board with no data-bus devices has no master read-back to splice
		// (and no mux). Not produced by any board; guard mirrors WordAckGen so a
		// future mis-spec degrades to a no-op rather than a nil dereference.
		return s
	}
	astLine, _, _ := strings.Cut(fileBanner, "\n") // "-- ****…****"
	socGenLine := "-- the tool is run. See soc_top/README for information on running soc_gen.\n"
	// (1) banner: after the soc_gen line, insert an asterisk line + the byte-bus note.
	s = strings.Replace(s, socGenLine,
		socGenLine+astLine+"\n-- byte bus post-processing script (script Apr/2017) --\n", 1)
	// (2) mux -> muxff(a) (no-dma). Match the no-arch line (trailing newline guards
	// against matching an already-"muxff" name).
	s = strings.Replace(s, "work.multi_master_bus_mux\n", "work.multi_master_bus_muxff(a)\n", 1)
	// (3) word_ack splice over `<master>_periph_dbus_i <= devs_bus_i(active_dev);`.
	master := res.DataBus.MasterBus
	target := "    " + master + "_periph_dbus_i <= devs_bus_i(active_dev);\n"
	var b strings.Builder
	fmt.Fprintf(&b, "    %s_periph_dbus_i.d <= devs_bus_i(active_dev).d;\n", master)
	b.WriteString("\n")
	b.WriteString("    word_ack_gen : entity work.word_ack_gen ( impl )\n")
	b.WriteString("      port map (\n")
	fmt.Fprintf(&b, "        %-20s => %s_periph_dbus_o.a,\n", "adr", master)
	fmt.Fprintf(&b, "        %-20s => %s_periph_dbus_o.en,\n", "word_bus_en", master)
	for _, name := range res.BusWord {
		fmt.Fprintf(&b, "        %-20s => devs_bus_i(%s).ack,\n", "ack_thru_in_"+name, devLit(name))
	}
	fmt.Fprintf(&b, "        %-20s => %s_periph_dbus_i.ack\n", "word_ack", master)
	b.WriteString("    );\n")
	return strings.Replace(s, target, b.String(), 1)
}
