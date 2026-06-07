package elaborate

import (
	"errors"
	"sort"

	"github.com/j-core/jcore-soc/tools/socgen/iface"
)

// Data-bus port types (parse.clj): a device OUT port of type cpu_data_i_t carries
// read data to the CPU; a device IN port of type cpu_data_o_t receives write data.
const (
	dataBusInType  = "cpu_data_i_t"
	dataBusOutType = "cpu_data_o_t"
)

// classifyDataBus tags each device's data-bus ports by VHDL type+direction
// (parse.clj:448-461). A device is a participant iff it has exactly one
// cpu_data_i_t/out and one cpu_data_o_t/in port; both become KindDataBus. A
// partial/duplicate pair is a malformed-bus error.
func classifyDataBus(devices []*ResolvedDevice) error {
	var errs []error
	for _, dev := range devices {
		var readOut, writeIn []*ResolvedPort
		for _, p := range dev.Ports {
			if p.Type == nil {
				continue
			}
			switch {
			case lc(p.Type.Mark) == dataBusInType && p.Dir == dirOut:
				readOut = append(readOut, p)
			case lc(p.Type.Mark) == dataBusOutType && p.Dir == dirIn:
				writeIn = append(writeIn, p)
			}
		}
		switch {
		case len(readOut) == 0 && len(writeIn) == 0:
			// not a data-bus device
		case len(readOut) == 1 && len(writeIn) == 1:
			readOut[0].Kind = KindDataBus
			writeIn[0].Kind = KindDataBus
			dev.DataBus = true
		default:
			errs = append(errs, &ResolveError{
				Kind: ErrDataBusPorts,
				Ctx:  "device " + dev.Name,
				Name: dev.Name,
			})
		}
	}
	return errors.Join(errs...)
}

// resolvePeripheralBuses builds the master topology. Each gathered bus defaults
// connected; the override map (design peripheral-buses) flips listed entries.
// Single connected master -> that bus, no muxes. cpu0+cpu1 connected -> a
// multi_master_bus_mux producing "cpu01"; +dmac -> a multi_master_bus_muxff
// producing "cpudm" (generate.clj:423-453).
func resolvePeripheralBuses(gathered []string, override map[string]bool, decodeMode string) (*PeripheralBusModel, error) {
	connSet := map[string]bool{}
	for _, b := range gathered {
		connSet[b] = true
	}
	// Only keys present in override flip a gathered bus; an override key that
	// names no gathered bus is silently ignored (faithful to devices.clj).
	for b, v := range override { // only present keys override; absent stays default-true
		connSet[b] = v
	}
	var connected, disconnected []string
	for _, b := range gathered {
		if connSet[b] {
			connected = append(connected, b)
		} else {
			disconnected = append(disconnected, b)
		}
	}
	sort.Strings(connected)
	sort.Strings(disconnected)

	m := &PeripheralBusModel{Connected: connected, Disconnected: disconnected, DecodeMode: decodeMode}
	// cpu0 is the implicit baseline master (devices.clj hard-codes the cpu0 ->
	// cpu01 -> cpudm chain names).
	master := "cpu0"
	if connSet["cpu0"] && connSet["cpu1"] {
		m.MuxStages = append(m.MuxStages, &MuxStage{Label: "cpus_mux", Entity: "multi_master_bus_mux", In1: master, In2: "cpu1", Out: "cpu01"})
		master = "cpu01"
	}
	if connSet["dmac"] {
		m.MuxStages = append(m.MuxStages, &MuxStage{Label: "dmac_mux", Entity: "multi_master_bus_muxff", In1: master, In2: "dmac", Out: "cpudm"})
		master = "cpudm"
	}
	m.MasterBus = master
	return m, nil
}

// gatherPeripheralBuses collects bus names from every used entity's
// PeripheralBuses (devices.clj:860-902): bound class entities, top entities, and
// padring entities. Returns sorted unique names.
func gatherPeripheralBuses(res *Resolution) []string {
	seen := map[string]bool{}
	add := func(e *iface.Entity) {
		if e == nil {
			return
		}
		for _, pb := range e.PeripheralBuses {
			seen[pb.Name] = true
		}
	}
	for _, dev := range res.Devices {
		if rc := res.Classes[lc(dev.Class)]; rc != nil {
			add(rc.Entity)
		}
	}
	for _, re := range res.TopEntities {
		add(re.Entity)
	}
	for _, re := range res.PadringEntities {
		add(re.Entity)
	}
	out := make([]string, 0, len(seen))
	for b := range seen {
		out = append(out, b)
	}
	sort.Strings(out)
	return out
}
