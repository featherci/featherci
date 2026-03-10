package executor

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestLogWriter_WriteAndClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	lw, err := NewLogWriter(path)
	if err != nil {
		t.Fatalf("NewLogWriter: %v", err)
	}

	data := "line one\nline two\nline three\n"
	n, err := lw.Write([]byte(data))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(data) {
		t.Errorf("wrote %d bytes, expected %d", n, len(data))
	}
	if lw.LineCount() != 3 {
		t.Errorf("line count: got %d, want 3", lw.LineCount())
	}

	if err := lw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(content) != data {
		t.Errorf("file content mismatch: got %q", string(content))
	}
}

func TestLogWriter_ConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "concurrent.log")

	lw, err := NewLogWriter(path)
	if err != nil {
		t.Fatalf("NewLogWriter: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = lw.Write([]byte("a line\n"))
		}()
	}
	wg.Wait()

	if lw.LineCount() != 100 {
		t.Errorf("line count: got %d, want 100", lw.LineCount())
	}

	if err := lw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestLogWriter_Flush(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "flush.log")

	lw, err := NewLogWriter(path)
	if err != nil {
		t.Fatalf("NewLogWriter: %v", err)
	}
	defer lw.Close()

	_, _ = lw.Write([]byte("buffered content\n"))
	if err := lw.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// Should be readable after flush.
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(content) != "buffered content\n" {
		t.Errorf("got %q after flush", string(content))
	}
}

func TestReadLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lines.log")

	content := "zero\none\ntwo\nthree\nfour\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	tests := []struct {
		name   string
		offset int
		limit  int
		want   []string
	}{
		{"from start", 0, 3, []string{"zero", "one", "two"}},
		{"with offset", 2, 2, []string{"two", "three"}},
		{"past end", 3, 10, []string{"three", "four"}},
		{"offset past end", 10, 5, nil},
		{"limit one", 0, 1, []string{"zero"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadLines(path, tt.offset, tt.limit)
			if err != nil {
				t.Fatalf("ReadLines: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d lines, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("line %d: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestReadLines_FileNotFound(t *testing.T) {
	_, err := ReadLines("/nonexistent/file.log", 0, 10)
	if err == nil {
		t.Error("expected error for missing file")
	}
}
