package iface

import (
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/vhdl"
)

func TestExtractSocPortNames(t *testing.T) {
	src := `entity e is
  port (p_o : out std_logic; spi_clk : out std_logic; reset : in std_logic; dr : out std_logic; plain : in std_logic);
  attribute soc_port_global_name of p_o : signal is "PO";
  attribute soc_port_local_name of spi_clk : signal is "clk";
  group sigs : global_ports(reset, dr);
end entity;`
	df, perr := vhdl.ParseFile(vhdl.NewFileSet(), "e.vhd", []byte(src))
	if perr != nil {
		t.Fatalf("parse: %v", perr)
	}
	lib, err := Extract([]*vhdl.DesignFile{df})
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	e, ok := lib.Entity("e")
	if !ok {
		t.Fatal("entity e not found")
	}
	g := map[string]string{}
	l := map[string]string{}
	for _, p := range e.Ports {
		if p.GlobalName != "" {
			g[p.Name] = p.GlobalName
		}
		if p.LocalName != "" {
			l[p.Name] = p.LocalName
		}
	}
	// attribute: value lowercased, quotes stripped
	if g["p_o"] != "po" {
		t.Errorf("p_o GlobalName = %q, want po", g["p_o"])
	}
	// local-name attribute
	if l["spi_clk"] != "clk" {
		t.Errorf("spi_clk LocalName = %q, want clk", l["spi_clk"])
	}
	// global_ports group members -> bare own id
	if g["reset"] != "reset" || g["dr"] != "dr" {
		t.Errorf("group GlobalNames = reset:%q dr:%q, want bare ids", g["reset"], g["dr"])
	}
	// untouched port
	if g["plain"] != "" || l["plain"] != "" {
		t.Errorf("plain should have no global/local name")
	}
}

func TestSocPortAttrOverridesGroup(t *testing.T) {
	// a port in a global_ports group AND with a soc_port_global_name attr: attr wins.
	src := `entity e is
  port (reset : in std_logic);
  group sigs : global_ports(reset);
  attribute soc_port_global_name of reset : signal is "sys_rst_n";
end entity;`
	df, perr := vhdl.ParseFile(vhdl.NewFileSet(), "e.vhd", []byte(src))
	if perr != nil {
		t.Fatalf("parse: %v", perr)
	}
	lib, err := Extract([]*vhdl.DesignFile{df})
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	e, _ := lib.Entity("e")
	for _, p := range e.Ports {
		if p.Name == "reset" && p.GlobalName != "sys_rst_n" {
			t.Errorf("reset GlobalName = %q, want sys_rst_n (attr overrides group)", p.GlobalName)
		}
	}
}
