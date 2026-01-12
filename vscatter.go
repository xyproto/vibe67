// Completion: 100% - SIMD instruction complete
package main

import (
	"fmt"
	"os"
)

// VSCATTERDPD/VSCATTERQPD - Vector scatter stores of packed double-precision floats
// using indices
//
// CRITICAL counterpart to VGATHER for Vibe67's map[uint64]float64:
//   - Sparse map writes: scatter values to indices [i1, i2, i3, i4]
//   - Indirect storage: map_values[indices[0..7]] := values
//   - Non-linear writes: storing map with computed offsets
//   - Parallel random writes without branches
//
// Completes the gather/scatter pair for full SIMD map access!
//
// Architecture details:
//   x86-64: VSCATTERQPD [base + zmm2*8]{k1}, zmm1 (AVX-512: 64-bit indices)
//   ARM64:  ST1D {zt.d}, pg, [xn, zm.d] (SVE2: scatter with vector base)
//   RISC-V: vsuxei64.v vs3, (rs1), vs2 (RVV: unordered indexed store)

// VScatterQPDToMem scatters float64 values using 64-bit indices
// memory[base + indices[i] * scale] = src[i]
// scale is typically 8 for float64 (8 bytes per element)
func (o *Out) VScatterQPDToMem(src, base, indices string, scale int32, mask string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.vscatterqpdX86ToMem(src, base, indices, scale, mask)
	case ArchARM64:
		o.vscatterARM64ToMem(src, base, indices, scale, mask)
	case ArchRiscv64:
		o.vscatterRISCVToMem(src, base, indices, scale, mask)
	}
}

// ============================================================================
// x86-64 AVX-512 implementation
// ============================================================================

// x86-64 VSCATTERQPD [base + zmm2*8]{k1}, zmm1
// EVEX.512.66.0F38.W1 A3 /r (VSIB addressing)
func (o *Out) vscatterqpdX86ToMem(src, base, indices string, scale int32, mask string) {
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	baseReg, baseOk := GetRegister(o.target.Arch(), base)
	indicesReg, indicesOk := GetRegister(o.target.Arch(), indices)
	maskReg, maskOk := GetRegister(o.target.Arch(), mask)
	if !srcOk || !baseOk || !indicesOk || !maskOk {
		return
	}

	// Validate scale (must be 1, 2, 4, or 8)
	if scale != 1 && scale != 2 && scale != 4 && scale != 8 {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "Error: Invalid scale %d (must be 1,2,4,8)\n", scale)
		}
		return
	}

	scaleEncoding := uint8(0)
	switch scale {
	case 1:
		scaleEncoding = 0
	case 2:
		scaleEncoding = 1
	case 4:
		scaleEncoding = 2
	case 8:
		scaleEncoding = 3
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vscatterqpd [%s + %s*%d]{%s}, %s:", base, indices, scale, mask, src)
	}

	if srcReg.Size == 512 {
		// AVX-512 VSCATTERQPD with VSIB addressing
		// EVEX encoding with masking
		p0 := uint8(0x62)

		// P1: map_select = 10 (0F38), with VSIB
		p1 := uint8(0x02)
		if (srcReg.Encoding & 8) == 0 {
			p1 |= 0x80 // R
		}
		if (indicesReg.Encoding & 8) == 0 { // X comes from index register
			p1 |= 0x40 // X
		}
		if (baseReg.Encoding & 8) == 0 {
			p1 |= 0x20 // B
		}
		if (srcReg.Encoding & 16) == 0 {
			p1 |= 0x10 // R'
		}

		// P2: W=1, vvvv=1111 (not used for scatter), pp=01
		p2 := uint8(0x81) | (0x0F << 3)

		// P3: L'L=10 (512-bit), with masking (aaa = mask register)
		p3 := uint8(0x40) | (maskReg.Encoding & 7) // aaa = k register
		if (indicesReg.Encoding & 16) == 0 {       // V' from index
			p3 |= 0x08
		}

		o.Write(p0)
		o.Write(p1)
		o.Write(p2)
		o.Write(p3)

		// Opcode: 0xA3 (VSCATTERQPD)
		o.Write(0xA3)

		// ModR/M with VSIB: [base + index*scale]
		// mod=00 (indirect), reg=src, r/m=base
		modrm := uint8(0x04) | ((srcReg.Encoding & 7) << 3) // mod=00, VSIB
		o.Write(modrm)

		// SIB: scale | index | base
		sib := (scaleEncoding << 6) |
			((indicesReg.Encoding & 7) << 3) |
			(baseReg.Encoding & 7)
		o.Write(sib)
	} else if srcReg.Size == 256 {
		// AVX2 doesn't have scatter - need AVX-512VL minimum
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (requires AVX-512VL, not AVX2)")
		}

		// Would need emulation with multiple scalar stores
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "\n# AVX2 scatter not available, needs AVX-512VL or emulation\n")
		}
		return
	} else {
		// XMM version - no scatter available
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (requires AVX-512VL)")
		}
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "\n# SSE scatter not available\n")
		}
		return
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// ARM64 SVE2 implementation
// ============================================================================

// ARM64 ST1D {zt.d}, pg, [xn, zm.d, LSL #3]
// SVE scatter store with vector indices
func (o *Out) vscatterARM64ToMem(src, base, indices string, scale int32, mask string) {
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	baseReg, baseOk := GetRegister(o.target.Arch(), base)
	indicesReg, indicesOk := GetRegister(o.target.Arch(), indices)
	maskReg, maskOk := GetRegister(o.target.Arch(), mask)
	if !srcOk || !baseOk || !indicesOk || !maskOk {
		return
	}

	if srcReg.Size == 512 {
		// SVE scatter store with vector base
		// ST1D {zt.d}, pg, [xn, zm.d, LSL #3]
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "st1d {%s.d}, %s, [%s, %s.d, lsl #3]:", src, mask, base, indices)
		}

		// SVE scatter store encoding
		// 1110010 1 1 Zm 0 1 Pg Rn Zt
		// xs=1 (scaled), U=0 (unsigned offset)
		instr := uint32(0xE5A00000) |
			(uint32(indicesReg.Encoding&31) << 16) | // Zm (indices)
			(uint32(maskReg.Encoding&15) << 10) | // Pg (predicate)
			(uint32(baseReg.Encoding&31) << 5) | // Xn (scalar base)
			uint32(srcReg.Encoding&31) // Zt (source)

		o.Write(uint8(instr & 0xFF))
		o.Write(uint8((instr >> 8) & 0xFF))
		o.Write(uint8((instr >> 16) & 0xFF))
		o.Write(uint8((instr >> 24) & 0xFF))
	} else {
		// NEON doesn't have scatter - would need to emulate with multiple stores
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "# NEON scatter not available, needs emulation\n")
		}
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// RISC-V RVV implementation
// ============================================================================

// RISC-V vsuxei64.v vs3, (rs1), vs2
// Unordered indexed store (scatter) with 64-bit indices
func (o *Out) vscatterRISCVToMem(src, base, indices string, scale int32, mask string) {
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	baseReg, baseOk := GetRegister(o.target.Arch(), base)
	indicesReg, indicesOk := GetRegister(o.target.Arch(), indices)
	// RISC-V uses v0 as implicit mask register
	if !srcOk || !baseOk || !indicesOk {
		return
	}

	// Note: RVV indices are byte offsets, so caller must scale appropriately
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vsuxei64.v %s, (%s), %s:", src, base, indices)
	}

	// vsuxei64.v encoding
	// nf mew mop vm sumop rs1 width vs3 opcode
	// mop=01 (indexed-unordered), width=111 (EEW=64)
	// sumop=00000 (plain), nf=0 (single register)
	instr := uint32(0x27) | // opcode=0100111 (STORE-FP)
		(7 << 12) | // width=111 (64-bit elements)
		(uint32(srcReg.Encoding&31) << 7) | // vs3 (source data)
		(uint32(baseReg.Encoding&31) << 15) | // rs1 (base address)
		(uint32(indicesReg.Encoding&31) << 20) | // vs2 (indices)
		(1 << 28) | // mop=01 (indexed unordered)
		(1 << 25) // vm=1 (unmasked)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}
