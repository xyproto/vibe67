package main

import (
	"fmt"
	"os"
)

func (fc *C67Compiler) detectBadAddresses(data []byte) {
	badPatterns := []struct {
		pattern []byte
		name    string
	}{
		{[]byte{0xef, 0xbe, 0xad, 0xde}, "0xdeadbeef"},
		{[]byte{0x78, 0x56, 0x34, 0x12}, "0x12345678"},
	}

	var found []string
	for _, bp := range badPatterns {
		idx := 0
		for idx < len(data) {
			pos := -1
			for i := idx; i <= len(data)-len(bp.pattern); i++ {
				match := true
				for j := 0; j < len(bp.pattern); j++ {
					if data[i+j] != bp.pattern[j] {
						match = false
						break
					}
				}
				if match {
					pos = i
					break
				}
			}
			if pos == -1 {
				break
			}
			found = append(found, fmt.Sprintf("%s at offset 0x%x", bp.name, pos))
			idx = pos + 1
		}
	}

	if len(found) > 0 && VerboseMode {
		fmt.Fprintf(os.Stderr, "\nWARNING: Unpatched relocations detected:\n")
		for _, f := range found {
			fmt.Fprintf(os.Stderr, "  %s\n", f)
		}
	}
}
