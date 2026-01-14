// Completion: 100% - Instruction implementation complete
package main

import (
	"fmt"
	"os"
)

// SHL - Shift Left
// Shifts bits to the left, filling with zeros

// ShlClReg - Shift Left by CL register
// shl reg, cl
func (o *Out) ShlClReg(dst, cl string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.shlClX86(dst)
	}
}

func (o *Out) shlClX86(dst string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "shl %s, cl: ", dst)
	}

	// REX prefix for 64-bit operation
	rex := uint8(0x48)
	if (dstReg.Encoding & 8) != 0 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)

	// D3 /4 - SHL r/m64, CL
	o.Write(0xD3)

	// ModR/M: 11 (register direct) | 100 (opcode extension /4) | r/m (dst)
	modrm := uint8(0xE0) | (dstReg.Encoding & 7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}









