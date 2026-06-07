// Package board is the integration layer of the Go soc_gen: it turns a board
// name into its full VHDL file set (via the build's file-list target), parses
// that set into an iface.Library, and validates the board's design spec against
// it. Layering: board -> design + iface + vhdl (one-way).
package board
