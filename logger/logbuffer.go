package logger

import (
	"io"
	"log"
	"os"
	"strings"
	"sync"
)

var (
	LogBuf = new(logBuffer)
	Logger = log.New(io.MultiWriter(os.Stderr, LogBuf), "", log.Lmicroseconds|log.Lshortfile)
)

const systemLogSize = 64 << 10

// logBuffer is a ring buffer.
type logBuffer struct {
	mu   sync.Mutex
	i    int
	full bool
	buf  [systemLogSize]byte
}

func (b *logBuffer) Write(p []byte) (ntot int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for len(p) > 0 {
		n := copy(b.buf[b.i:], p)
		p = p[n:]
		ntot += n
		b.i += n
		if b.i == len(b.buf) {
			b.i = 0
			b.full = true
		}
	}
	return
}

func (b *logBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.full {
		return string(b.buf[:b.i])
	}
	s := string(b.buf[b.i:]) + string(b.buf[:b.i])
	if nl := strings.Index(s, "\n"); nl != -1 {
		// Remove first line, since it's probably truncated
		s = s[nl+1:]
	}
	return "...\n" + s
}
