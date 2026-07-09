package main

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	Magic         = 0x4B4D434A
	CollectorPort = 47000
	resultSize    = 24

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

func ParseResult(b []byte) (Result, error) {
	if len(b) < resultSize {
		return Result{}, errors.New("short packet")
	}
	le := binary.LittleEndian
	r := Result{
		Magic:      le.Uint32(b[0:]),
		GitRev:     le.Uint32(b[4:]),
		CRC:        le.Uint16(b[8:]),
		Iterations: le.Uint32(b[12:]),
		Cycles:     le.Uint32(b[16:]),
		ClkHz:      le.Uint32(b[20:]),
	}
	if r.Magic != Magic {
		return Result{}, errors.New("bad magic")
	}
	return r, nil
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
