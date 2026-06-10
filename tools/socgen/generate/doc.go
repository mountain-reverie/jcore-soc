// Package generate orchestrates the soc_gen emit phase: it turns a loaded board
// and its elaborated Resolution into the full generated file set (devices.vhd,
// soc.vhd, pad_ring.vhd, optional board.dts/board.h, and build.mk) and writes it
// to an output directory.
//
// It uses a selection-only plugin model: the board's design.Plugins list decides
// which optional files are produced (device_tree -> board.dts, board.h ->
// board.h). The Clojure SocGenPlugin protocol is intentionally not reproduced;
// the plugin bodies already live in the emit/devicetree/cheader packages.
package generate
