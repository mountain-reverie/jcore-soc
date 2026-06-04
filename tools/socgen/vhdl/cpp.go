package vhdl

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// runCPP streams src through the external preprocessor exe and returns its
// stdout. Flags mirror jcore's tools/common.mk. Include search paths are
// rooted at the source file's directory so `#include "x"` and config/ resolve.
// No temp file: src is piped to stdin, stdout buffered from a pipe.
func runCPP(exe, filename string, src []byte) ([]byte, error) {
	dir := filepath.Dir(filename)
	args := []string{
		"-x", "c-header", "-E", "-P", "-w", "-nostdinc",
		"-I", dir, "-I", filepath.Join(dir, "config"),
		"-",
	}
	cmd := exec.Command(exe, args...)
	cmd.Stdin = bytes.NewReader(src)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}
