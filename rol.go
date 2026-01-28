// Completion: 100% - Instruction implementation complete
package main

import (
	"fmt"
	"os"
)

// ROL - Rotate Left
// Rotates bits to the left (bits shifted out come back on the right)

// RolClReg - Rotate Left by CL register
// rol reg, cl
func (o *Out) RolClReg(dst, cl string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.rolClX86(dst)
	}
}

func (o *Out) rolClX86(dst string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "rol %s, cl: ", dst)
	}

	// REX prefix for 64-bit operation
	rex := uint8(0x48)
	if (dstReg.Encoding & 8) != 0 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)

	// D3 /0 - ROL r/m64, CL
	o.Write(0xD3)

	// ModR/M: 11 (register direct) | 000 (opcode extension /0) | r/m (dst)
	modrm := uint8(0xC0) | (dstReg.Encoding & 7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}
