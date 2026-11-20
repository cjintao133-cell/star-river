package store

import "testing"

// FuzzOpen asserts the field cipher never panics on arbitrary ciphertext/AAD.
func FuzzOpen(f *testing.F) {
	c, err := NewFieldCipher(make([]byte, 32))
	if err != nil {
		f.Fatalf("NewFieldCipher: %v", err)
	}
	sealed, err := c.Seal([]byte("hello"), []byte("aad"))
	if err != nil {
		f.Fatalf("Seal: %v", err)
	}
	f.Add(sealed, []byte("aad"))
	f.Add([]byte{}, []byte{})
	f.Add([]byte{1, 2, 3}, []byte("x"))
	f.Fuzz(func(t *testing.T, sealed, aad []byte) {
		_, _ = c.Open(sealed, aad)
	})
}
