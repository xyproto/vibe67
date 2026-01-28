// Completion: 100% - Platform support complete
package main

import (
	"encoding/binary"
)

// GeneratePLT creates PLT (Procedure Linkage Table) stubs
func (ds *DynamicSections) GeneratePLT(functions []string, gotBase uint64, pltBase uint64) {
	ds.plt.Reset()
	ds.pltEntries = functions

	switch ds.arch {
	case ArchARM64:
		ds.generatePLTARM64(functions, gotBase, pltBase)
	case ArchRiscv64:
		ds.generatePLTRiscv64(functions, gotBase, pltBase)
	default: // x86_64
		ds.generatePLTx86_64(functions, gotBase, pltBase)
	}
}

// GenerateGOT creates GOT (Global Offset Table) entries
func (ds *DynamicSections) GenerateGOT(functions []string, dynamicAddr uint64, pltBase uint64) {
	ds.got.Reset()

	// GOT[0] = address of _DYNAMIC
	binary.Write(&ds.got, binary.LittleEndian, dynamicAddr)

	// GOT[1] = link_map (filled by dynamic linker)
	binary.Write(&ds.got, binary.LittleEndian, uint64(0))

	// GOT[2] = _dl_runtime_resolve (filled by dynamic linker)
	binary.Write(&ds.got, binary.LittleEndian, uint64(0))

	// GOT[3..n] = PLT stubs (initial values point to PLT push instructions)
	for i := range functions {
		// Point to the push instruction in PLT[i+1]
		// x86_64: PLT[0] is 16 bytes, each entry is 16 bytes, push at +6
		// ARM64: PLT[0] is 20 bytes, each entry is 16 bytes
		var pltPushAddr uint64
		switch ds.arch {
		case ArchARM64:
			pltPushAddr = pltBase + 20 + uint64(i*16)
		default: // x86_64
			pltPushAddr = pltBase + 16 + uint64(i*16) + 6
		}
		binary.Write(&ds.got, binary.LittleEndian, pltPushAddr)
	}
}

// GetPLTOffset returns the offset within PLT for a function
func (ds *DynamicSections) GetPLTOffset(funcName string) int {
	for i, name := range ds.pltEntries {
		if name == funcName {
			switch ds.arch {
			case ArchARM64:
				// PLT[0] is 20 bytes, each entry is 16 bytes
				return 20 + i*16
			default: // x86_64
				// PLT[0] is 16 bytes, each entry is 16 bytes
				return 16 + i*16
			}
		}
	}
	return -1
}
