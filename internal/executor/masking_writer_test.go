package executor

import (
	"bytes"
	"testing"
)

func TestMaskingWriter_MasksSecrets(t *testing.T) {
	var buf bytes.Buffer
	w := NewMaskingWriter(&buf, []string{"s3cr3t", "pa$$word"})

	w.Write([]byte("connecting with s3cr3t to server\n"))
	w.Write([]byte("using pa$$word for auth\n"))

	got := buf.String()
	if want := "connecting with *** to server\nusing *** for auth\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMaskingWriter_NoSecrets(t *testing.T) {
	var buf bytes.Buffer
	w := NewMaskingWriter(&buf, nil)

	w.Write([]byte("hello world\n"))

	if got := buf.String(); got != "hello world\n" {
		t.Errorf("got %q, want %q", got, "hello world\n")
	}
}

func TestMaskingWriter_EmptySecrets(t *testing.T) {
	var buf bytes.Buffer
	w := NewMaskingWriter(&buf, []string{"", ""})

	w.Write([]byte("hello world\n"))

	if got := buf.String(); got != "hello world\n" {
		t.Errorf("got %q, want %q", got, "hello world\n")
	}
}

func TestMaskingWriter_ReturnsOriginalLength(t *testing.T) {
	var buf bytes.Buffer
	w := NewMaskingWriter(&buf, []string{"secret"})

	input := []byte("my secret value")
	n, err := w.Write(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len(input) {
		t.Errorf("got n=%d, want %d", n, len(input))
	}
}

func TestMaskingWriter_MultipleOccurrences(t *testing.T) {
	var buf bytes.Buffer
	w := NewMaskingWriter(&buf, []string{"tok"})

	w.Write([]byte("tok and tok again"))

	if got, want := buf.String(), "*** and *** again"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
