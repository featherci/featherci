package crypto

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func testKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return key
}

func TestRoundTrip(t *testing.T) {
	enc, err := NewEncryptor(testKey(t))
	if err != nil {
		t.Fatal(err)
	}

	plaintext := []byte("super secret value")
	ct, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatal(err)
	}

	got, err := enc.Decrypt(ct)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(got, plaintext) {
		t.Fatalf("got %q, want %q", got, plaintext)
	}
}

func TestRoundTripEmpty(t *testing.T) {
	enc, err := NewEncryptor(testKey(t))
	if err != nil {
		t.Fatal(err)
	}

	ct, err := enc.Encrypt([]byte(""))
	if err != nil {
		t.Fatal(err)
	}

	got, err := enc.Decrypt(ct)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 0 {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestInvalidKeyLength(t *testing.T) {
	_, err := NewEncryptor([]byte("too short"))
	if err == nil {
		t.Fatal("expected error for short key")
	}
}

func TestTamperedCiphertext(t *testing.T) {
	enc, err := NewEncryptor(testKey(t))
	if err != nil {
		t.Fatal(err)
	}

	ct, err := enc.Encrypt([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}

	// Flip a byte in the ciphertext (after the nonce)
	ct[len(ct)-1] ^= 0xff

	_, err = enc.Decrypt(ct)
	if err == nil {
		t.Fatal("expected error for tampered ciphertext")
	}
}

func TestCiphertextTooShort(t *testing.T) {
	enc, err := NewEncryptor(testKey(t))
	if err != nil {
		t.Fatal(err)
	}

	_, err = enc.Decrypt([]byte("short"))
	if err == nil {
		t.Fatal("expected error for short ciphertext")
	}
}

func TestDifferentCiphertextEachTime(t *testing.T) {
	enc, err := NewEncryptor(testKey(t))
	if err != nil {
		t.Fatal(err)
	}

	plaintext := []byte("same input")
	ct1, _ := enc.Encrypt(plaintext)
	ct2, _ := enc.Encrypt(plaintext)

	if bytes.Equal(ct1, ct2) {
		t.Fatal("two encryptions of same plaintext should differ (random nonce)")
	}
}
