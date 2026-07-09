package main

import (
	"errors"
	"fmt"
	"os"
)

const flashBase = 0x00100000

func assemble(bitstream, payload []byte) ([]byte, error) {
	if len(bitstream) > flashBase {
		return nil, fmt.Errorf("bitstream %d bytes overruns FLASH_BASE %#x", len(bitstream), flashBase)
	}
	out := make([]byte, flashBase+len(payload))
	for i := range out { out[i] = 0xFF }        // erased-flash fill
	copy(out, bitstream)
	copy(out[flashBase:], payload)
	return out, nil
}

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: mkflash <bitstream.bit> <coremark.bin> <out.img>")
		os.Exit(2)
	}
	bit, err := os.ReadFile(os.Args[1]); check(err)
	pay, err := os.ReadFile(os.Args[2]); check(err)
	img, err := assemble(bit, pay); check(err)
	check(os.WriteFile(os.Args[3], img, 0644))
}

func check(err error) { if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) } }
var _ = errors.New
