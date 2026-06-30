package emit

import (
	"fmt"
	"sort"
	"strings"

	"github.com/j-core/jcore-soc/tools/socgen/design"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
)

// CPUsConfigName is the stable name of the generated cpus configuration. The
// hand-written board top-level references this single name regardless of variant.
// It is an alias for elaborate.CPUsConfigName (defined there to avoid import cycles).
const CPUsConfigName = elaborate.CPUsConfigName

// CPUsConfig generates the `configuration soc_cpus_config of cpus` declaration
// from a declarative cpu: block. It returns the stable name, the VHDL source,
// and the extra synth filelist sources the chosen binding needs.
func CPUsConfig(cpu *design.CPU) (string, string, []string, error) {
	if cpu == nil {
		return "", "", nil, fmt.Errorf("cpu block is nil")
	}
	if cpu.Architecture == "" {
		return "", "", nil, fmt.Errorf("cpu.architecture is required")
	}
	synth, generics, files, err := elaborate.CPUSynthConfig(cpu.Model, cpu.Decode, cpu.Mult)
	if err != nil {
		return "", "", nil, err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "configuration %s of cpus is\n", CPUsConfigName)
	fmt.Fprintf(&b, "  for %s\n", cpu.Architecture)
	b.WriteString("    for all : cpu_core\n")
	b.WriteString("      use entity work.cpu_core(arch);\n")
	b.WriteString("      for arch\n")
	b.WriteString("        for u_cpu : cpu\n")
	if len(generics) == 0 {
		fmt.Fprintf(&b, "          use configuration work.%s;\n", synth)
	} else {
		fmt.Fprintf(&b, "          use configuration work.%s\n", synth)
		b.WriteString("            generic map (\n")
		keys := make([]string, 0, len(generics))
		for k := range generics {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			sep := ","
			if i == len(keys)-1 {
				sep = ""
			}
			fmt.Fprintf(&b, "              %s => %s%s\n", k, generics[k], sep)
		}
		b.WriteString("            );\n")
	}
	b.WriteString("        end for;\n")
	b.WriteString("      end for;\n")
	b.WriteString("    end for;\n")
	b.WriteString("  end for;\n")
	b.WriteString("end configuration;\n")
	return CPUsConfigName, b.String(), files, nil
}
