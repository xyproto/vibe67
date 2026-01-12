// Completion: 100% - SIMD instruction complete
package main

import (
	"fmt"
	"os"
)

// VSUBPD - Vector subtraction for packed double-precision (float64) values
// Essential for Vibe67's differential operations:
//   - Vector differences: deltas |> map((a, b) -> a - b)
//   - Offset removal: values |> map(x -> x - baseline)
//   - Comparisons and differences in pipelines
//
// Architecture details:
//   x86-64: VSUBPD zmm1, zmm2, zmm3 (AVX-512: 8x float64)
//   ARM64:  FSUB z0.d, p0/m, z1.d, z2.d (SVE2: scalable float64)
//   RISC-V: vfsub.vv v1, v2, v3 (RVV: scalable float64)

// VSubPDVectorToVector performs vector subtraction: dst = src1 - src2
func (o *Out) VSubPDVectorToVector(dst, src1, src2 string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.vsubpdX86VectorToVector(dst, src1, src2)
	case ArchARM64:
		o.vsubpdARM64VectorToVector(dst, src1, src2)
	case ArchRiscv64:
		o.vsubpdRISCVVectorToVector(dst, src1, src2)
	}
}

// x86-64 VSUBPD (opcode 0x5C)
func (o *Out) vsubpdX86VectorToVector(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vsubpd %s, %s, %s:", dst, src1, src2)
	}

	if dstReg.Size == 512 {
		// EVEX encoding
		p0 := uint8(0x62)
		p1 := uint8(0x01)
		if (dstReg.Encoding & 8) == 0 {
			p1 |= 0x80
		}
		p1 |= 0x40
		if (src2Reg.Encoding & 8) == 0 {
			p1 |= 0x20
		}
		if (dstReg.Encoding & 16) == 0 {
			p1 |= 0x10
		}

		p2 := uint8(0x81)
		p2 |= uint8((^src1Reg.Encoding & 0x0F) << 3)

		p3 := uint8(0x40)
		if (src1Reg.Encoding & 16) == 0 {
			p3 |= 0x08
		}

		o.Write(p0)
		o.Write(p1)
		o.Write(p2)
		o.Write(p3)
		o.Write(0x5C) // VSUBPD opcode

		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (src2Reg.Encoding & 7)
		o.Write(modrm)
	} else if dstReg.Size == 256 {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (AVX2)")
		}
		o.Write(0xC4)
		vex1 := uint8(0x01)
		if (dstReg.Encoding & 8) == 0 {
			vex1 |= 0x80
		}
		vex1 |= 0x40
		if (src2Reg.Encoding & 8) == 0 {
			vex1 |= 0x20
		}
		o.Write(vex1)

		vex2 := uint8(0x45)
		vex2 |= uint8((^src1Reg.Encoding & 0x0F) << 3)
		o.Write(vex2)
		o.Write(0x5C)

		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (src2Reg.Encoding & 7)
		o.Write(modrm)
	} else {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (SSE2)")
		}
		o.Write(0x66)
		if (dstReg.Encoding&8) != 0 || (src2Reg.Encoding&8) != 0 {
			rex := uint8(0x40)
			if (dstReg.Encoding & 8) != 0 {
				rex |= 0x04
			}
			if (src2Reg.Encoding & 8) != 0 {
				rex |= 0x01
			}
			o.Write(rex)
		}
		o.Write(0x0F)
		o.Write(0x5C)
		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (src2Reg.Encoding & 7)
		o.Write(modrm)
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ARM64 FSUB
func (o *Out) vsubpdARM64VectorToVector(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if dstReg.Size == 512 {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "fsub %s.d, p7/m, %s.d, %s.d:", dst, src1, src2)
		}
		// SVE FSUB: opc=001 for SUB
		instr := uint32(0x65000000) |
			(3 << 22) | // size=11
			(1 << 18) | // opc[0]=1 for SUB
			(uint32(src2Reg.Encoding&31) << 16) |
			(7 << 10) |
			(uint32(src1Reg.Encoding&31) << 5) |
			uint32(dstReg.Encoding&31)

		o.Write(uint8(instr & 0xFF))
		o.Write(uint8((instr >> 8) & 0xFF))
		o.Write(uint8((instr >> 16) & 0xFF))
		o.Write(uint8((instr >> 24) & 0xFF))
	} else {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "fsub %s.2d, %s.2d, %s.2d:", dst, src1, src2)
		}
		// NEON FSUB
		instr := uint32(0x4EE01C00) |
			(uint32(src2Reg.Encoding&31) << 16) |
			(uint32(src1Reg.Encoding&31) << 5) |
			uint32(dstReg.Encoding&31)

		o.Write(uint8(instr & 0xFF))
		o.Write(uint8((instr >> 8) & 0xFF))
		o.Write(uint8((instr >> 16) & 0xFF))
		o.Write(uint8((instr >> 24) & 0xFF))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// RISC-V vfsub.vv
func (o *Out) vsubpdRISCVVectorToVector(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vfsub.vv %s, %s, %s:", dst, src2, src1)
	}

	// vfsub.vv: funct6=000010
	instr := uint32(0x57) |
		(1 << 12) | // OPFVV
		(uint32(dstReg.Encoding&31) << 7) |
		(uint32(src1Reg.Encoding&31) << 15) |
		(uint32(src2Reg.Encoding&31) << 20) |
		(1 << 25) | // vm=1
		(0x02 << 26) // funct6=000010

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}
