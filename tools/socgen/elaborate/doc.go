// Package elaborate is the soc_gen elaborate phase: it resolves a loaded board
// (design spec + interface library) into a concrete, semantic SoC structure that
// the emit phase renders to VHDL. P4b resolves devices (entity/architecture,
// registers, names, generics); later sub-milestones add the signal net-list,
// pins/rings/irq, and final validation. Layering: elaborate -> board + design +
// iface + vhdl (one-way).
package elaborate
