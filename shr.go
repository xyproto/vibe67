// Completion: 100% - Instruction implementation complete
package main

import (
	"fmt"
	"os"
)

// SHR - Shift Right
// Shifts bits to the right, filling with zeros (logical shift)

// ShrClReg - Shift Right by CL register
// shr reg, cl
func (o *Out) ShrClReg(dst, cl string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.shrClX86(dst)
	}
}

func (o *Out) shrClX86(dst string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "shr %s, cl: ", dst)
	}

	// REX prefix for 64-bit operation
	rex := uint8(0x48)
	if (dstReg.Encoding & 8) != 0 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)

	// D3 /5 - SHR r/m64, CL
	o.Write(0xD3)

	// ModR/M: 11 (register direct) | 101 (opcode extension /5) | r/m (dst)
	modrm := uint8(0xE8) | (dstReg.Encoding & 7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}
