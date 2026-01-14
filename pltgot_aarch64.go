// Completion: 100% - ARM64 PLT/GOT generation complete
package main

import (
	"encoding/binary"
)

// generatePLTARM64 creates PLT stubs for ARM64
func (ds *DynamicSections) generatePLTARM64(functions []string, gotBase uint64, pltBase uint64) {
	// PLT[0] - special resolver stub for ARM64
	// stp x16, x30, [sp, #-16]!  ; save x16 and lr
	ds.plt.Write([]byte{0xf0, 0x7b, 0xbf, 0xa9})
	// adrp x16, GOT[2]
	pageOffset := ((gotBase + 16) >> 12) - (pltBase >> 12)
	adrpInstr := uint32(0x90000010) | (uint32(pageOffset&0x3) << 29) | (uint32((pageOffset>>2)&0x7ffff) << 5)
	binary.Write(&ds.plt, binary.LittleEndian, adrpInstr)
	// ldr x17, [x16, #:lo12:GOT[2]]
	lo12 := (gotBase + 16) & 0xfff
	ldrInstr := uint32(0xf9400211) | (uint32(lo12>>3) << 10)
	binary.Write(&ds.plt, binary.LittleEndian, ldrInstr)
	// add x16, x16, #:lo12:GOT[2]
	addInstr := uint32(0x91000210) | (uint32(lo12) << 10)
	binary.Write(&ds.plt, binary.LittleEndian, addInstr)
	// br x17
	ds.plt.Write([]byte{0x20, 0x02, 0x1f, 0xd6})

	// PLT[1..n] - one per function
	for i := range functions {
		pltOffset := pltBase + uint64(ds.plt.Len())
		gotOffset := gotBase + uint64(24+i*8) // GOT[0,1,2] reserved, functions start at GOT[3]

		// adrp x16, GOT[n]
		pageOff := (gotOffset >> 12) - (pltOffset >> 12)
		adrpInstr := uint32(0x90000010) | (uint32(pageOff&0x3) << 29) | (uint32((pageOff>>2)&0x7ffff) << 5)
		binary.Write(&ds.plt, binary.LittleEndian, adrpInstr)

		// ldr x17, [x16, #:lo12:GOT[n]]
		lo12 := gotOffset & 0xfff
		ldrInstr := uint32(0xf9400211) | (uint32(lo12>>3) << 10)
		binary.Write(&ds.plt, binary.LittleEndian, ldrInstr)

		// add x16, x16, #:lo12:GOT[n]
		addInstr := uint32(0x91000210) | (uint32(lo12) << 10)
		binary.Write(&ds.plt, binary.LittleEndian, addInstr)

		// br x17
		ds.plt.Write([]byte{0x20, 0x02, 0x1f, 0xd6})
	}
}









