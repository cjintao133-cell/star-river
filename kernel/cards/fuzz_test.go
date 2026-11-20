package cards

import "testing"

// FuzzParse asserts that card parsing never panics on arbitrary/untrusted input.
func FuzzParse(f *testing.F) {
	f.Add([]byte(`{"name":"a","description":"b"}`))
	f.Add([]byte(`{"spec":"chara_card_v2","spec_version":"2.0","data":{"name":"x"}}`))
	f.Add([]byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}) // PNG magic
	f.Add([]byte("PK\x03\x04"))                                   // zip magic
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = Parse(b)
		_, _ = ParseJSON(b)
		_, _ = ParsePNG(b)
		_, _ = ParseCharx(b)
	})
}
