package executor

import (
	"bufio"
	"fmt"
	"os"
	"sync"
)

// LogWriter provides thread-safe, buffered log writing to a file.
type LogWriter struct {
	file      *os.File
	writer    *bufio.Writer
	mu        sync.Mutex
	lineCount int
}

// NewLogWriter creates a new log writer at the given path.
// Parent directories must already exist.
func NewLogWriter(path string) (*LogWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("creating log file: %w", err)
	}
	return &LogWriter{
		file:   f,
		writer: bufio.NewWriter(f),
	}, nil
}

// Write implements io.Writer. It is safe for concurrent use.
func (lw *LogWriter) Write(p []byte) (int, error) {
	lw.mu.Lock()
	defer lw.mu.Unlock()

	n, err := lw.writer.Write(p)
	if err != nil {
		return n, err
	}

	// Count newlines for line tracking.
	for _, b := range p[:n] {
		if b == '\n' {
			lw.lineCount++
		}
	}
	return n, nil
}

// LineCount returns the number of complete lines written so far.
func (lw *LogWriter) LineCount() int {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	return lw.lineCount
}

// Flush writes any buffered data to the underlying file.
func (lw *LogWriter) Flush() error {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	return lw.writer.Flush()
}

// Close flushes and closes the log file.
func (lw *LogWriter) Close() error {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	if err := lw.writer.Flush(); err != nil {
		lw.file.Close()
		return err
	}
	return lw.file.Close()
}

// ReadLines reads a range of lines from the log file on disk.
// offset is 0-based, limit is the max number of lines to return.
// This opens the file independently, so it can be called while writing.
func ReadLines(path string, offset, limit int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening log file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var lines []string
	lineNum := 0
	for scanner.Scan() {
		if lineNum >= offset {
			lines = append(lines, scanner.Text())
			if len(lines) >= limit {
				break
			}
		}
		lineNum++
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning log file: %w", err)
	}
	return lines, nil
}
