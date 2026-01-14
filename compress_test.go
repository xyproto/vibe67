package main

import (
	"bytes"
	"testing"
)

func TestCompressionBasic(t *testing.T) {
	c := NewCompressor()

	tests := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"simple", []byte("hello world")},
		{"repeated", []byte("aaaaaaaaaa")},
		{"pattern", []byte("abcabcabcabc")},
		{"mixed", []byte("hello hello world world")},
		{"with_0xFF", []byte{0xFF, 0xFF, 0xFF, 0x00, 0x01}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compressed := c.Compress(tt.data)
			decompressed, err := c.Decompress(compressed)
			if err != nil {
				t.Fatalf("Decompress failed: %v", err)
			}
			if !bytes.Equal(decompressed, tt.data) {
				t.Fatalf("Data mismatch:\noriginal:     %v\ndecompressed: %v", tt.data, decompressed)
			}
		})
	}
}

func TestCompressionRatio(t *testing.T) {
	c := NewCompressor()

	data := make([]byte, 1000)
	for i := range data {
		data[i] = byte(i % 10)
	}

	compressed := c.Compress(data)
	ratio := float64(len(compressed)) / float64(len(data))

	t.Logf("Original: %d bytes, Compressed: %d bytes, Ratio: %.2f",
		len(data), len(compressed), ratio)

	if ratio > 0.8 {
		t.Logf("Warning: compression ratio not very good (%.2f)", ratio)
	}
}









