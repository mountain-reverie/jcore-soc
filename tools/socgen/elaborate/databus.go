package elaborate

import "errors"

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
