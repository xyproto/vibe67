// Completion: 100% - x86_64 PLT/GOT generation complete
package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

// generatePLTx86_64 creates PLT stubs for x86_64
func (ds *DynamicSections) generatePLTx86_64(functions []string, gotBase uint64, pltBase uint64) {
	// PLT[0] - special resolver stub
	// pushq GOT[1]
	ds.plt.Write([]byte{0xff, 0x35})
	offset1 := uint32(gotBase + 8 - pltBase - 6) // offset to GOT[1]
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "PLT[0] push offset: gotBase=0x%x, pltBase=0x%x, offset=0x%x\n", gotBase, pltBase, offset1)
	}
	binary.Write(&ds.plt, binary.LittleEndian, offset1)

	// jmpq *GOT[2]
	ds.plt.Write([]byte{0xff, 0x25})
	offset2 := uint32(gotBase + 16 - pltBase - 12) // offset to GOT[2]
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "PLT[0] jmp offset: offset=0x%x\n", offset2)
	}
	binary.Write(&ds.plt, binary.LittleEndian, offset2)

	// padding
	ds.plt.Write([]byte{0x0f, 0x1f, 0x40, 0x00})

	// PLT[1..n] - one per function
	for i := range functions {
		pltOffset := pltBase + uint64(ds.plt.Len())
		gotOffset := gotBase + uint64(24+i*8) // GOT[0,1,2] reserved, functions start at GOT[3]

		// jmpq *GOT[n]
		ds.plt.Write([]byte{0xff, 0x25})
		relOffset := int32(gotOffset - pltOffset - 6)
		binary.Write(&ds.plt, binary.LittleEndian, relOffset)

		// pushq $index
		ds.plt.Write([]byte{0x68})
		binary.Write(&ds.plt, binary.LittleEndian, uint32(i))

		// jmpq PLT[0]
		ds.plt.Write([]byte{0xe9})
		jumpBack := int32(pltBase - pltOffset - 16)
		binary.Write(&ds.plt, binary.LittleEndian, jumpBack)
	}
}
