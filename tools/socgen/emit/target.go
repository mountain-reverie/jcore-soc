package emit

// externalConstraints reports whether the target emits its pin constraints to a
// separate file (LPF/PCF) and uses plain top-level ports with no inline VHDL pad
// attributes and no unisim library — true for the Lattice flows (ecp5, ice40),
// false for Xilinx (spartan6), which uses inline attrs + unisim.
func externalConstraints(target string) bool {
	return target == "ecp5" || target == "ice40"
}
