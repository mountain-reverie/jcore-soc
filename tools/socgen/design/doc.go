// Package design loads YAML SoC board specifications (with file includes and
// recursive merge) into a typed Design model, and validates a Design against an
// iface.Library (entity/generic/port references). It replaces the Clojure
// design.edn mechanism. Layering: design -> iface -> vhdl (one-way).
package design
