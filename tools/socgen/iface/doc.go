// Package iface extracts a shallow interface model from parsed VHDL design
// files: per-entity port/generic interfaces, per-package constants/types/
// subtypes/components, architectures (name->entity), and configurations
// (name->entity+architecture), plus a cross-unit name index. It performs NO
// type resolution, value evaluation, or use-clause scoping — those are deferred
// to later soc_gen phases. It depends one-way on the vhdl package.
package iface
