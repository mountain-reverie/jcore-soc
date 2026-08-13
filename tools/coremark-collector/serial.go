package main

import (
	"os"
	"syscall"
	"unsafe"
)

// CBAUD is the c_cflag baud-rate mask. It is not exposed by the standard
// syscall package (only golang.org/x/sys/unix has it, and this tool stays
// dependency-free), but its value is part of the stable Linux termios ABI
// (asm-generic/termbits.h): 0010017 octal.
const cbaud = 0010017

// openSerial opens dev and configures it as a raw 115200 8N1 serial line,
// matching the iCELink UART the board's CoreMark firmware talks over.
func openSerial(dev string) (*os.File, error) {
	f, err := os.OpenFile(dev, os.O_RDWR|syscall.O_NOCTTY|syscall.O_SYNC, 0)
	if err != nil {
		return nil, err
	}

	var t syscall.Termios
	// Populate t from the actual line first (TCGETS) -- without this, t is
	// zero-valued and the &^= clears below are no-ops on already-zero bits,
	// and c_cflag's CBAUD field starts at 0 (B0, i.e. "hang up the line"),
	// so the port would never actually run at 115200.
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TCGETS), uintptr(unsafe.Pointer(&t))); errno != 0 {
		f.Close()
		return nil, errno
	}
	// c_iflag: don't map/translate input, don't do XON/XOFF flow control --
	// this is a binary/text framed line, not a terminal session.
	t.Iflag &^= syscall.IXON | syscall.ICRNL
	// c_oflag: no output post-processing (we never write more than the
	// single trigger byte, but keep the line raw regardless).
	t.Oflag &^= syscall.OPOST
	// c_lflag: raw mode -- no line buffering (ICANON), no local echo, no
	// signal-generating control characters (ISIG).
	t.Lflag &^= syscall.ICANON | syscall.ECHO | syscall.ISIG
	// c_cflag: 8 data bits, 115200 baud (Linux carries line speed in the
	// CBAUD bits of c_cflag; TCSETS ignores Ispeed/Ospeed -- those are only
	// honoured via TCSETS2/BOTHER), ignore modem control lines, enable the
	// receiver.
	t.Cflag = (t.Cflag &^ (syscall.CSIZE | cbaud)) | syscall.CS8 | syscall.B115200 | syscall.CLOCAL | syscall.CREAD
	t.Ispeed = syscall.B115200
	t.Ospeed = syscall.B115200
	// VMIN=0, VTIME=1 (100ms): non-blocking-ish reads that return whatever
	// is available after a short wait, rather than blocking forever for a
	// full buffer -- callers poll this in a loop against their own deadline.
	t.Cc[syscall.VMIN] = 0
	t.Cc[syscall.VTIME] = 1

	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TCSETS), uintptr(unsafe.Pointer(&t))); errno != 0 {
		f.Close()
		return nil, errno
	}
	return f, nil
}
