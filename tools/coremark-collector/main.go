package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	dev := flag.String("dev", "/dev/ttyACM0", "serial device")
	timeout := flag.Duration("timeout", 60*time.Second, "overall deadline for a complete record")
	expectCRC := flag.Uint("expect-crc", uint(ExpectedCRC), "expected CoreMark crcfinal (known-good, catches gcc miscompiles)")
	trigger := flag.String("trigger", "g", "byte sent on seeing CMK READY")
	asJSON := flag.Bool("json", false, "emit the result as one JSON object instead of the text line")
	flag.Parse()

	if len(*trigger) != 1 {
		fmt.Fprintln(os.Stderr, "-trigger must be exactly one byte")
		os.Exit(1)
	}

	f, err := openSerial(*dev)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()

	deadline := time.Now().Add(*timeout)

	var (
		armed    bool // saw "CMK READY" and sent the trigger
		sawReady bool
		record   []string
	)

	lines := newLineReader(f)
	// The serial fd's VTIME makes each raw read return within ~100ms even
	// with no data, so lines.next() never blocks past the deadline check.
	for time.Now().Before(deadline) {
		line, err := lines.next()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if line == "" {
			continue // read timed out with no complete line; keep polling
		}

		if strings.Contains(line, "CMK READY") {
			sawReady = true
			record = record[:0]
			if !armed {
				if _, err := f.Write([]byte(*trigger)); err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
				armed = true
			}
			continue
		}

		if !strings.HasPrefix(line, "CMK ") {
			continue // console noise
		}

		if strings.Contains(line, "CMK DONE") {
			r, perr := ParseRecord(record)
			if perr != nil {
				fmt.Fprintln(os.Stderr, "malformed record:", perr)
				armed = false // re-arm for the board's next attempt
				continue
			}
			if verr := validate(r, uint16(*expectCRC)); verr != nil {
				// A well-formed result with the wrong CRC is a definite
				// miscompile signal, not noise -- fail hard.
				fmt.Fprintln(os.Stderr, verr)
				os.Exit(3)
			}
			printResult(r, *asJSON)
			os.Exit(0)
		}

		record = append(record, line)
	}

	switch {
	case len(record) > 0:
		fmt.Fprintln(os.Stderr, "timeout: saw a partial CMK record but no CMK DONE")
	case sawReady:
		fmt.Fprintln(os.Stderr, "timeout: saw CMK READY but no record")
	default:
		fmt.Fprintln(os.Stderr, "timeout: no output at all from the board")
	}
	os.Exit(2)
}

func printResult(r Result, asJSON bool) {
	ips := float64(r.ClkHz) / float64(r.Cycles) * float64(r.Iterations)
	if asJSON {
		fmt.Printf("{\"git\":%d,\"crc\":%d,\"iterations\":%d,\"cycles\":%d,\"clkhz\":%d,\"iters_per_sec\":%.2f}\n",
			r.GitRev, r.CRC, r.Iterations, r.Cycles, r.ClkHz, ips)
		return
	}
	fmt.Printf("coremark git=%#x crc=%#x iterations=%d cycles=%d iters_per_sec=%.2f\n",
		r.GitRev, r.CRC, r.Iterations, r.Cycles, ips)
}

// lineReader accumulates raw serial reads into CRLF-terminated lines. It is
// hand-rolled rather than bufio.Scanner because the serial fd is configured
// with VMIN=0/VTIME>0 (see serial.go): a read with no data available
// returns (0, nil), and bufio.Scanner retries internally on that case
// without ever returning to the caller -- which would defeat polling the
// overall deadline in main's loop.
type lineReader struct {
	f   *os.File
	buf []byte
}

func newLineReader(f *os.File) *lineReader {
	return &lineReader{f: f}
}

// next returns the next complete line (without its line terminator) once
// one is available. It returns ("", nil) if the current read produced no
// complete line yet -- the caller is expected to call next again.
func (l *lineReader) next() (string, error) {
	if i := indexNewline(l.buf); i >= 0 {
		line := string(l.buf[:i])
		l.buf = l.buf[i+1:]
		return strings.TrimRight(line, "\r"), nil
	}

	chunk := make([]byte, 256)
	n, err := l.f.Read(chunk)
	if err != nil {
		return "", err
	}
	if n > 0 {
		l.buf = append(l.buf, chunk[:n]...)
	}
	if i := indexNewline(l.buf); i >= 0 {
		line := string(l.buf[:i])
		l.buf = l.buf[i+1:]
		return strings.TrimRight(line, "\r"), nil
	}
	return "", nil
}

func indexNewline(b []byte) int {
	for i, c := range b {
		if c == '\n' {
			return i
		}
	}
	return -1
}
