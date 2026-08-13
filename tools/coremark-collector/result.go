package main

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	Magic = 0x4B4D434A

	// ExpectedCRC is the known-good CoreMark crcfinal for the exact
	// firmware build parameters used by targets/boards/icesugar/rom/coremark:
	// vendor CoreMark's default TOTAL_DATA_SIZE (2*1000 = 2000, so
	// PERFORMANCE_RUN: seed1=0, seed2=0, seed3=0x66) and the board's fixed
	// ITERATIONS=1000 (see coremark/Makefile: -DITERATIONS=1000, and
	// coremark/core_portme.c: seed4_volatile = ITERATIONS).
	//
	// This is NOT the commonly-quoted "CoreMark 1.0 crclist" constant
	// 0xe714 (targets/boards/icesugar/rom/coremark/vendor/core_main.c:39,
	// list_known_crc[3] for the size-666-per-algorithm run) -- that value
	// is only the crcfinal snapshot after the FIRST iteration
	// (core_main.c:69-70, `if (i == 0) res->crclist = res->crc;`).
	// results[0].crc (crcfinal), which is what portme_finish()/the board
	// actually transmits (core_portme.h: portme_finish(ee_u16 crc, ...)),
	// keeps accumulating via crcu16() across ALL `iterations` (core_main.c
	// iterate(), lines 63-71) and is therefore iteration-count dependent.
	//
	// Confirmed by natively compiling the vendored CoreMark sources
	// (core_main.c/core_list_join.c/core_matrix.c/core_state.c/core_util.c)
	// against a minimal Linux core_portme with ITERATIONS=1000 and default
	// TOTAL_DATA_SIZE=2000 (matching the board build exactly): the run
	// printed seedcrc=0xe9f5 (known_id=3, the "2K performance run", size
	// 666-per-algorithm after the /3 split), crclist=0xe714 (matches
	// list_known_crc[3], confirming the vendored copy is bit-identical to
	// upstream CoreMark 1.0), but crcfinal=0xd340 for the full 1000
	// iterations, deterministic and reproducible across repeated runs.
	ExpectedCRC uint16 = 0xd340
)

type Result struct {
	Magic      uint32
	GitRev     uint32
	CRC        uint16
	Iterations uint32
	Cycles     uint32
	ClkHz      uint32
}

// ParseRecord scans a set of lines (as accumulated from the board's serial
// output between "CMK READY" and "CMK DONE") for the "CMK key=value" fields
// that make up a complete CoreMark record. Non-"CMK " lines (console noise)
// and unrecognized "CMK KEY=..." fields (forward compatibility with future
// firmware) are ignored. All six known fields are required; Cycles == 0 is
// rejected as an invalid (in-progress or corrupted) result.
func ParseRecord(lines []string) (Result, error) {
	fields := map[string]string{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		rest, ok := strings.CutPrefix(line, "CMK ")
		if !ok {
			continue // console noise
		}
		key, value, ok := strings.Cut(rest, "=")
		if !ok {
			continue // e.g. "CMK READY" / "CMK DONE", not a key=value field
		}
		fields[key] = value
	}

	magic, err := parseHex32(fields, "MAGIC")
	if err != nil {
		return Result{}, err
	}
	gitrev, err := parseHex32(fields, "GITREV")
	if err != nil {
		return Result{}, err
	}
	crc, err := parseHex32(fields, "CRC")
	if err != nil {
		return Result{}, err
	}
	iterations, err := parseDecimal(fields, "ITERATIONS")
	if err != nil {
		return Result{}, err
	}
	cycles, err := parseDecimal(fields, "CYCLES")
	if err != nil {
		return Result{}, err
	}
	clkhz, err := parseDecimal(fields, "CLKHZ")
	if err != nil {
		return Result{}, err
	}
	if cycles == 0 {
		return Result{}, fmt.Errorf("CMK CYCLES=0: invalid result")
	}

	return Result{
		Magic:      magic,
		GitRev:     gitrev,
		CRC:        uint16(crc),
		Iterations: iterations,
		Cycles:     cycles,
		ClkHz:      clkhz,
	}, nil
}

func parseHex32(fields map[string]string, key string) (uint32, error) {
	v, ok := fields[key]
	if !ok {
		return 0, fmt.Errorf("missing CMK %s field", key)
	}
	n, err := strconv.ParseUint(v, 0, 32)
	if err != nil {
		return 0, fmt.Errorf("CMK %s=%q: %w", key, v, err)
	}
	return uint32(n), nil
}

func parseDecimal(fields map[string]string, key string) (uint32, error) {
	v, ok := fields[key]
	if !ok {
		return 0, fmt.Errorf("missing CMK %s field", key)
	}
	n, err := strconv.ParseUint(v, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("CMK %s=%q: %w", key, v, err)
	}
	return uint32(n), nil
}

// validate checks a parsed, non-zero-cycle Result against the known-good
// CoreMark CRC. A mismatch means the candidate gcc miscompiled CoreMark
// (or the wrong firmware/build parameters were flashed) -- this is the
// board's primary purpose, so it must be a hard failure, not a skip.
func validate(r Result, expectedCRC uint16) error {
	if r.CRC != expectedCRC {
		return fmt.Errorf("CRC MISMATCH: got %#04x want %#04x — candidate gcc miscompiled", r.CRC, expectedCRC)
	}
	return nil
}
