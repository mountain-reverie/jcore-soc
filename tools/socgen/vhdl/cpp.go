package vhdl

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// runCPP streams src through the external preprocessor and returns its stdout.
// Flags mirror jcore's tools/common.mk. Include search paths are rooted at the
// source file's directory so `#include "x"` and config/ resolve. Extra defines
// and include dirs from cfg are appended before the terminal "-" argument.
// No temp file: src is piped to stdin, stdout buffered from a pipe.
func runCPP(cfg parseConfig, filename string, src []byte) ([]byte, error) {
	dir := filepath.Dir(filename)
	args := []string{
		"-x", "c-header", "-E", "-P", "-w", "-nostdinc",
		"-I", dir, "-I", filepath.Join(dir, "config"),
	}
	for _, inc := range cfg.cppIncludes {
		args = append(args, "-I", filepath.Join(dir, inc))
	}
	for _, d := range cfg.cppDefines {
		if d.value == "" {
			args = append(args, "-D"+d.key)
		} else {
			args = append(args, "-D"+d.key+"="+d.value)
		}
	}
	args = append(args, "-")
	cmd := exec.Command(cfg.cpp, args...)
	cmd.Stdin = bytes.NewReader(src)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}
