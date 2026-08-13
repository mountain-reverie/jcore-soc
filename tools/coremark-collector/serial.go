package main

import (
	"os"
	"syscall"
	"unsafe"
)

// openSerial opens dev and configures it as a raw 115200 8N1 serial line,
// matching the iCELink UART the board's CoreMark firmware talks over.
func openSerial(dev string) (*os.File, error) {
	f, err := os.OpenFile(dev, os.O_RDWR|syscall.O_NOCTTY|syscall.O_SYNC, 0)
	if err != nil {
		return nil, err
	}

	var t syscall.Termios
	// c_iflag: don't map/translate input, don't do XON/XOFF flow control --
	// this is a binary/text framed line, not a terminal session.
	t.Iflag &^= syscall.IXON | syscall.ICRNL
	// c_oflag: no output post-processing (we never write more than the
	// single trigger byte, but keep the line raw regardless).
	t.Oflag &^= syscall.OPOST
	// c_lflag: raw mode -- no line buffering (ICANON), no local echo, no
	// signal-generating control characters (ISIG).
	t.Lflag &^= syscall.ICANON | syscall.ECHO | syscall.ISIG
	// c_cflag: 8 data bits, ignore modem control lines, enable the
	// receiver.
	t.Cflag = (t.Cflag &^ syscall.CSIZE) | syscall.CS8 | syscall.CLOCAL | syscall.CREAD
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
