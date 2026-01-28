// Completion: 100% - Instruction implementation complete
package main

import (
	"fmt"
	"os"
)

// ROR - Rotate Right
// Rotates bits to the right (bits shifted out come back on the left)

// RorClReg - Rotate Right by CL register
// ror reg, cl
func (o *Out) RorClReg(dst, cl string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.rorClX86(dst)
	}
}

func (o *Out) rorClX86(dst string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "ror %s, cl: ", dst)
	}

	// REX prefix for 64-bit operation
	rex := uint8(0x48)
	if (dstReg.Encoding & 8) != 0 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)

	// D3 /1 - ROR r/m64, CL
	o.Write(0xD3)

	// ModR/M: 11 (register direct) | 001 (opcode extension /1) | r/m (dst)
	modrm := uint8(0xC8) | (dstReg.Encoding & 7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}
