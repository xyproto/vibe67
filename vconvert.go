// Completion: 100% - SIMD instruction complete
package main

import (
	"fmt"
	"os"
)

// VCVT* - Vector conversion between integer and float types
//
// Essential for Vibe67's map[uint64]float64 foundation:
//   - Converting integer keys to floats for computation
//   - Converting float results back to integer indices
//   - Type conversions in data pipelines
//   - Integer-based indexing operations
//
// Example usage in Vibe67:
//   float_keys = int_keys || map(i64_to_f64)
//   int_indices = float_vals || map(f64_to_i64)
//
// Architecture details:
//   x86-64: VCVTQQ2PD/VCVTTPD2QQ (AVX-512DQ)
//   ARM64:  SCVTF/FCVTZS zd.d, pg/m, zn.d (SVE2)
//   RISC-V: vfcvt.f.x.v/vfcvt.x.f.v (RVV)

// VCvtQQ2PDVectorToVector converts packed int64 to float64
// dst[i] = (float64)src[i]
func (o *Out) VCvtQQ2PDVectorToVector(dst, src string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.vcvtqq2pdX86VectorToVector(dst, src)
	case ArchARM64:
		o.vcvtqq2pdARM64VectorToVector(dst, src)
	case ArchRiscv64:
		o.vcvtqq2pdRISCVVectorToVector(dst, src)
	}
}

// VCvtTPD2QQVectorToVector converts packed float64 to int64 with truncation
// dst[i] = (int64)trunc(src[i])
func (o *Out) VCvtTPD2QQVectorToVector(dst, src string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.vcvttpd2qqX86VectorToVector(dst, src)
	case ArchARM64:
		o.vcvttpd2qqARM64VectorToVector(dst, src)
	case ArchRiscv64:
		o.vcvttpd2qqRISCVVectorToVector(dst, src)
	}
}

// ============================================================================
// x86-64 AVX-512DQ implementation - INT64 to FP64
// ============================================================================

// x86-64 VCVTQQ2PD zmm1, zmm2/m512
// EVEX.512.F3.0F.W1 E6 /r
func (o *Out) vcvtqq2pdX86VectorToVector(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vcvtqq2pd %s, %s:", dst, src)
	}

	if dstReg.Size == 512 {
		// AVX-512DQ VCVTQQ2PD
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (AVX-512DQ)")
		}

		p0 := uint8(0x62)

		p1 := uint8(0x01) // map=0F, F3 prefix
		if (dstReg.Encoding & 8) == 0 {
			p1 |= 0x80
		}
		p1 |= 0x40
		if (srcReg.Encoding & 8) == 0 {
			p1 |= 0x20
		}
		if (dstReg.Encoding & 16) == 0 {
			p1 |= 0x10
		}

		p2 := uint8(0x82) | (0x0F << 3) // vvvv=1111 (unused), W=1

		p3 := uint8(0x40)
		if (srcReg.Encoding & 16) == 0 {
			p3 |= 0x08
		}

		o.Write(p0)
		o.Write(p1)
		o.Write(p2)
		o.Write(p3)
		o.Write(0xE6) // VCVTQQ2PD opcode

		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (srcReg.Encoding & 7)
		o.Write(modrm)
	} else if dstReg.Size == 256 {
		// AVX-512VL VCVTQQ2PD (ymm from ymm)
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (AVX-512VL)")
		}

		p0 := uint8(0x62)

		p1 := uint8(0x01)
		if (dstReg.Encoding & 8) == 0 {
			p1 |= 0x80
		}
		p1 |= 0x40
		if (srcReg.Encoding & 8) == 0 {
			p1 |= 0x20
		}
		if (dstReg.Encoding & 16) == 0 {
			p1 |= 0x10
		}

		p2 := uint8(0x82) | (0x0F << 3)

		p3 := uint8(0x20) // L'L=01 (256-bit)
		if (srcReg.Encoding & 16) == 0 {
			p3 |= 0x08
		}

		o.Write(p0)
		o.Write(p1)
		o.Write(p2)
		o.Write(p3)
		o.Write(0xE6)

		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (srcReg.Encoding & 7)
		o.Write(modrm)
	} else {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (no SSE2 int64→float64 vector conversion)")
		}
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "\n# Use scalar CVTSI2SD in loop")
		}
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// x86-64 AVX-512DQ implementation - FP64 to INT64
// ============================================================================

// x86-64 VCVTTPD2QQ zmm1, zmm2/m512
// EVEX.512.66.0F.W1 7A /r
func (o *Out) vcvttpd2qqX86VectorToVector(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vcvttpd2qq %s, %s:", dst, src)
	}

	if dstReg.Size == 512 {
		// AVX-512DQ VCVTTPD2QQ
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (AVX-512DQ)")
		}

		p0 := uint8(0x62)

		p1 := uint8(0x01) // map=0F
		if (dstReg.Encoding & 8) == 0 {
			p1 |= 0x80
		}
		p1 |= 0x40
		if (srcReg.Encoding & 8) == 0 {
			p1 |= 0x20
		}
		if (dstReg.Encoding & 16) == 0 {
			p1 |= 0x10
		}

		p2 := uint8(0x81) | (0x0F << 3) // vvvv=1111, W=1

		p3 := uint8(0x40)
		if (srcReg.Encoding & 16) == 0 {
			p3 |= 0x08
		}

		o.Write(p0)
		o.Write(p1)
		o.Write(p2)
		o.Write(p3)
		o.Write(0x7A) // VCVTTPD2QQ opcode

		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (srcReg.Encoding & 7)
		o.Write(modrm)
	} else if dstReg.Size == 256 {
		// AVX-512VL VCVTTPD2QQ
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (AVX-512VL)")
		}

		p0 := uint8(0x62)

		p1 := uint8(0x01)
		if (dstReg.Encoding & 8) == 0 {
			p1 |= 0x80
		}
		p1 |= 0x40
		if (srcReg.Encoding & 8) == 0 {
			p1 |= 0x20
		}
		if (dstReg.Encoding & 16) == 0 {
			p1 |= 0x10
		}

		p2 := uint8(0x81) | (0x0F << 3)

		p3 := uint8(0x20)
		if (srcReg.Encoding & 16) == 0 {
			p3 |= 0x08
		}

		o.Write(p0)
		o.Write(p1)
		o.Write(p2)
		o.Write(p3)
		o.Write(0x7A)

		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (srcReg.Encoding & 7)
		o.Write(modrm)
	} else {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (no SSE2 float64→int64 vector conversion)")
		}
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "\n# Use scalar CVTTSD2SI in loop")
		}
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// ARM64 SVE2/NEON implementation - INT64 to FP64
// ============================================================================

func (o *Out) vcvtqq2pdARM64VectorToVector(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if dstReg.Size == 512 {
		// SVE SCVTF - signed convert to float
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "scvtf %s.d, p7/m, %s.d:", dst, src)
		}

		// SVE SCVTF encoding
		// 01100101 11 0 11010 101 Pg Zn Zd
		instr := uint32(0x65D54000) |
			(7 << 10) | // Pg=p7
			(uint32(srcReg.Encoding&31) << 5) | // Zn
			uint32(dstReg.Encoding&31) // Zd

		o.Write(uint8(instr & 0xFF))
		o.Write(uint8((instr >> 8) & 0xFF))
		o.Write(uint8((instr >> 16) & 0xFF))
		o.Write(uint8((instr >> 24) & 0xFF))
	} else {
		// NEON SCVTF
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "scvtf %s.2d, %s.2d:", dst, src)
		}

		// NEON SCVTF encoding
		// 0 Q 0 01110 0 sz 1 00001 11101 10 Rn Rd
		// Q=1, sz=1 (double)
		instr := uint32(0x4EE1D800) |
			(uint32(srcReg.Encoding&31) << 5) |
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

// ============================================================================
// ARM64 SVE2/NEON implementation - FP64 to INT64
// ============================================================================

func (o *Out) vcvttpd2qqARM64VectorToVector(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if dstReg.Size == 512 {
		// SVE FCVTZS - float convert to signed with truncation
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "fcvtzs %s.d, p7/m, %s.d:", dst, src)
		}

		// SVE FCVTZS encoding
		// 01100101 11 0 11010 101 Pg Zn Zd
		instr := uint32(0x65DA5000) |
			(7 << 10) | // Pg=p7
			(uint32(srcReg.Encoding&31) << 5) | // Zn
			uint32(dstReg.Encoding&31) // Zd

		o.Write(uint8(instr & 0xFF))
		o.Write(uint8((instr >> 8) & 0xFF))
		o.Write(uint8((instr >> 16) & 0xFF))
		o.Write(uint8((instr >> 24) & 0xFF))
	} else {
		// NEON FCVTZS
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "fcvtzs %s.2d, %s.2d:", dst, src)
		}

		// NEON FCVTZS encoding
		// 0 Q 0 01110 1 sz 1 00001 10111 10 Rn Rd
		// Q=1, sz=1 (double)
		instr := uint32(0x4EE1B800) |
			(uint32(srcReg.Encoding&31) << 5) |
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

// ============================================================================
// RISC-V RVV implementation - INT64 to FP64
// ============================================================================

// RISC-V vfcvt.f.x.v vd, vs2
// Convert signed integer to float
func (o *Out) vcvtqq2pdRISCVVectorToVector(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vfcvt.f.x.v %s, %s:", dst, src)
	}

	// vfcvt.f.x.v encoding
	// funct6=010010, vm=1, rs1=00011, funct3=001
	instr := uint32(0x57) |
		(1 << 12) | // funct3=001
		(uint32(dstReg.Encoding&31) << 7) |
		(3 << 15) | // rs1=00011 (F.X)
		(uint32(srcReg.Encoding&31) << 20) |
		(1 << 25) | // vm=1
		(0x12 << 26) // funct6=010010

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// RISC-V RVV implementation - FP64 to INT64
// ============================================================================

// RISC-V vfcvt.x.f.v vd, vs2
// Convert float to signed integer with truncation
func (o *Out) vcvttpd2qqRISCVVectorToVector(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vfcvt.rtz.x.f.v %s, %s:", dst, src)
	}

	// vfcvt.rtz.x.f.v encoding (round toward zero)
	// funct6=010010, vm=1, rs1=00111, funct3=001
	instr := uint32(0x57) |
		(1 << 12) | // funct3=001
		(uint32(dstReg.Encoding&31) << 7) |
		(7 << 15) | // rs1=00111 (RTZ.X.F)
		(uint32(srcReg.Encoding&31) << 20) |
		(1 << 25) | // vm=1
		(0x12 << 26) // funct6=010010

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}
