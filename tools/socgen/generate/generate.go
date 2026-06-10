package generate

// File is one generated artifact. InBuildMK marks the core VHDL files listed in
// build.mk (the Clojure :buildmk? true files); extra plugin files are false.
type File struct {
	Name      string // base name, e.g. "devices.vhd"
	Content   string
	InBuildMK bool
}
