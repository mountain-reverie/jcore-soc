package emit

import (
	"fmt"
	"maps"
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

	if cpu.FpgaOptCore0 {
		// Asymmetric dual: core0 binds the FPGA-optimised config (register file
		// in ECP5 block RAM); core1 keeps the standard config (portable /
		// ASIC-representative). Requires cores==2 (two core labels to bind).
		if cpu.Cores != 2 {
			return "", "", nil, fmt.Errorf("fpga-opt-core0 requires cores: 2, got %d", cpu.Cores)
		}
		optSynth, optGenerics, optFiles, err := elaborate.CPUSynthConfigFPGAOpt(cpu.Model, cpu.Decode, cpu.Mult)
		if err != nil {
			return "", "", nil, err
		}
		// CORE_ID is set here (in the config binding) rather than at the cpu_core
		// instantiation because a binding indication's generic map REPLACES the
		// instantiation's -- so the instantiation's CORE_ID would be dropped. It
		// gives the two cpu instances distinct ghdl->yosys module names despite
		// identical generics, which they need because their nested register-file
		// architecture is bound differently (ebr vs two_bank).
		writeCoreBinding(&b, "core0", optSynth, withCoreID(optGenerics, 0))
		writeCoreBinding(&b, "core1", synth, withCoreID(generics, 1))
		// The FPGA-optimised core adds the ebr regfile + its synth config; the
		// shared decode tables come from the standard config's files.
		files = append(files, optFiles...)
	} else {
		writeCoreBinding(&b, "all", synth, generics)
	}

	b.WriteString("  end for;\n")
	b.WriteString("end configuration;\n")
	return CPUsConfigName, b.String(), files, nil
}

// withCoreID returns a copy of generics with CORE_ID set (the cpu-level
// elaboration tag for the asymmetric dual). generics may be nil.
func withCoreID(generics map[string]string, id int) map[string]string {
	out := make(map[string]string, len(generics)+1)
	maps.Copy(out, generics)
	out["CORE_ID"] = fmt.Sprintf("%d", id)
	return out
}

// writeCoreBinding emits one `for <label> : cpu_core ... use configuration
// work.<synth> [generic map (...)]` block. label is "all" (symmetric) or a
// specific instance label ("core0"/"core1") for the asymmetric dual.
func writeCoreBinding(b *strings.Builder, label, synth string, generics map[string]string) {
	fmt.Fprintf(b, "    for %s : cpu_core\n", label)
	b.WriteString("      use entity work.cpu_core(arch);\n")
	b.WriteString("      for arch\n")
	b.WriteString("        for u_cpu : cpu\n")
	if len(generics) == 0 {
		fmt.Fprintf(b, "          use configuration work.%s;\n", synth)
	} else {
		fmt.Fprintf(b, "          use configuration work.%s\n", synth)
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
			fmt.Fprintf(b, "              %s => %s%s\n", k, generics[k], sep)
		}
		b.WriteString("            );\n")
	}
	b.WriteString("        end for;\n")
	b.WriteString("      end for;\n")
	b.WriteString("    end for;\n")
}
