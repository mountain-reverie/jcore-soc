// Package elaborate is the soc_gen elaborate phase: it resolves a loaded board
// (design spec + interface library) into a concrete, semantic SoC structure that
// the emit phase renders to VHDL. It resolves devices (entity/architecture,
// registers, names, generics; P4b), builds the global-signal net-list (P4c), and
// resolves top/padring entities and joins their ports into that net-list (P4d).
// Pins/rings/irq wiring and final cross-validation are added by later
// sub-milestones. Layering: elaborate -> board + design + iface + vhdl (one-way).
package elaborate
