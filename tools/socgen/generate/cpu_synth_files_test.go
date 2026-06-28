package generate

import (
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/j-core/jcore-soc/tools/socgen/board"
	"github.com/j-core/jcore-soc/tools/socgen/elaborate"
)

// TestBuildEmitsCPUSynthFilesList builds the ulx3s default variant and asserts
// the generated cpu_synth_files.list, as a SET, equals the expected j2-direct
// synth set. This proves the default analyzed set is preserved (the three files
// removed from the static filelist.sh array now come from this generated list).
func TestBuildEmitsCPUSynthFilesList(t *testing.T) {
	root := os.Getenv("JCORE_SOC_ROOT")
	if root == "" {
		t.Skip("JCORE_SOC_ROOT not set")
	}
	b, _ := board.Load(root, "ulx3s", "")
	if b == nil || b.Design == nil || b.Design.CPU == nil {
		t.Skip("ulx3s not yet migrated to cpu: block")
	}
	res, _ := elaborate.Elaborate(b)
	files, err := Build(b, res)
	if err != nil {
		t.Logf("Build notes (tolerated in hg-less env): %v", err)
	}
	var got *File
	for i := range files {
		if files[i].Name == "cpu_synth_files.list" {
			got = &files[i]
		}
	}
	if got == nil {
		t.Fatal("cpu_synth_files.list not emitted")
	}
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(got.Content), "\n") {
		if l != "" {
			lines = append(lines, l)
		}
	}
	sort.Strings(lines)
	want := []string{
		"decode/decode_table_direct.vhd",
		"decode/decode_table_direct_config.vhd",
		"synth/cpu_synth_config.vhd",
	}
	sort.Strings(want)
	if strings.Join(lines, ",") != strings.Join(want, ",") {
		t.Errorf("j2-direct synth set mismatch:\n got %v\nwant %v", lines, want)
	}
}
