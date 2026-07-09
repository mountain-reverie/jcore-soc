package main

import (
	"encoding/binary"
	"errors"
)

const (
	Magic         = 0x4B4D434A
	CollectorPort = 47000
	resultSize    = 24
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
