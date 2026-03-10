package executor

import (
	"bytes"
	"io"
)

// MaskingWriter wraps an io.Writer and replaces occurrences of secret values
// with "***" before writing to the underlying writer. This prevents secrets
// injected as environment variables from leaking into build logs.
type MaskingWriter struct {
	inner   io.Writer
	secrets [][]byte
	mask    []byte
}

// NewMaskingWriter creates a writer that masks the given secret values.
// If secrets is empty, writes pass through unchanged.
func NewMaskingWriter(w io.Writer, secrets []string) io.Writer {
	if len(secrets) == 0 {
		return w
	}

	var byteSecrets [][]byte
	for _, s := range secrets {
		if len(s) > 0 {
			byteSecrets = append(byteSecrets, []byte(s))
		}
	}
	if len(byteSecrets) == 0 {
		return w
	}

	return &MaskingWriter{
		inner:   w,
		secrets: byteSecrets,
		mask:    []byte("***"),
	}
}

// Write replaces any secret values in p with "***" before writing.
func (m *MaskingWriter) Write(p []byte) (int, error) {
	masked := p
	for _, secret := range m.secrets {
		masked = bytes.ReplaceAll(masked, secret, m.mask)
	}
	_, err := m.inner.Write(masked)
	// Return original length to satisfy io.Writer contract —
	// callers expect n == len(p) on success.
	if err != nil {
		return 0, err
	}
	return len(p), nil
}
