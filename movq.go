// Completion: 100% - Instruction implementation complete
package main

// movq.go - MOVQ instruction encoding for x86-64
//
// This file contains helper functions for the MOVQ instruction which moves
// quadword (64-bit) values between XMM registers and general-purpose registers.

// MovqXmmToReg generates a MOVQ instruction to move from XMM to GPR
// Format: MOVQ r64, xmm
// Encoding: 66 REX.W 0F 7E /r
//
// Example: MovqXmmToReg("r10", "xmm0") generates: movq r10, xmm0
func (out *Out) MovqXmmToReg(destReg string, srcXmm string) {
	// Determine REX prefix
	var rex byte = 0x48 // REX.W (64-bit operand)
	var regBits byte

	// Parse destination register
	switch destReg {
	case "rax":
		regBits = 0
	case "rcx":
		regBits = 1
	case "rdx":
		regBits = 2
	case "rbx":
		regBits = 3
	case "rsp":
		regBits = 4
	case "rbp":
		regBits = 5
	case "rsi":
		regBits = 6
	case "rdi":
		regBits = 7
	case "r8":
		rex |= 0x01 // REX.B
		regBits = 0
	case "r9":
		rex |= 0x01
		regBits = 1
	case "r10":
		rex |= 0x01
		regBits = 2
	case "r11":
		rex |= 0x01
		regBits = 3
	case "r12":
		rex |= 0x01
		regBits = 4
	case "r13":
		rex |= 0x01
		regBits = 5
	case "r14":
		rex |= 0x01
		regBits = 6
	case "r15":
		rex |= 0x01
		regBits = 7
	}

	// Parse source XMM register
	var xmmBits byte
	switch srcXmm {
	case "xmm0":
		xmmBits = 0
	case "xmm1":
		xmmBits = 1
	case "xmm2":
		xmmBits = 2
	case "xmm3":
		xmmBits = 3
	case "xmm4":
		xmmBits = 4
	case "xmm5":
		xmmBits = 5
	case "xmm6":
		xmmBits = 6
	case "xmm7":
		xmmBits = 7
	case "xmm8":
		rex |= 0x04 // REX.R
		xmmBits = 0
	case "xmm9":
		rex |= 0x04
		xmmBits = 1
	case "xmm10":
		rex |= 0x04
		xmmBits = 2
	case "xmm11":
		rex |= 0x04
		xmmBits = 3
	case "xmm12":
		rex |= 0x04
		xmmBits = 4
	case "xmm13":
		rex |= 0x04
		xmmBits = 5
	case "xmm14":
		rex |= 0x04
		xmmBits = 6
	case "xmm15":
		rex |= 0x04
		xmmBits = 7
	}

	// Build ModR/M byte
	// Mod = 11 (register-register)
	// Reg = xmm register (source)
	// R/M = GPR (destination)
	modRM := byte(0xC0) | (xmmBits << 3) | regBits

	// Emit: 66 REX.W 0F 7E ModR/M
	out.Write(0x66)
	out.Write(rex)
	out.Write(0x0F)
	out.Write(0x7E)
	out.Write(modRM)
}

// MovqRegToXmm generates a MOVQ instruction to move from GPR to XMM
// Format: MOVQ xmm, r/m64
// Encoding: 66 REX.W 0F 6E /r
//
// Example: MovqRegToXmm("xmm0", "r10") generates: movq xmm0, r10
func (out *Out) MovqRegToXmm(destXmm string, srcReg string) {
	// Determine REX prefix
	var rex byte = 0x48 // REX.W (64-bit operand)
	var regBits byte

	// Parse source register
	switch srcReg {
	case "rax":
		regBits = 0
	case "rcx":
		regBits = 1
	case "rdx":
		regBits = 2
	case "rbx":
		regBits = 3
	case "rsp":
		regBits = 4
	case "rbp":
		regBits = 5
	case "rsi":
		regBits = 6
	case "rdi":
		regBits = 7
	case "r8":
		rex |= 0x01 // REX.B
		regBits = 0
	case "r9":
		rex |= 0x01
		regBits = 1
	case "r10":
		rex |= 0x01
		regBits = 2
	case "r11":
		rex |= 0x01
		regBits = 3
	case "r12":
		rex |= 0x01
		regBits = 4
	case "r13":
		rex |= 0x01
		regBits = 5
	case "r14":
		rex |= 0x01
		regBits = 6
	case "r15":
		rex |= 0x01
		regBits = 7
	}

	// Parse destination XMM register
	var xmmBits byte
	switch destXmm {
	case "xmm0":
		xmmBits = 0
	case "xmm1":
		xmmBits = 1
	case "xmm2":
		xmmBits = 2
	case "xmm3":
		xmmBits = 3
	case "xmm4":
		xmmBits = 4
	case "xmm5":
		xmmBits = 5
	case "xmm6":
		xmmBits = 6
	case "xmm7":
		xmmBits = 7
	case "xmm8":
		rex |= 0x04 // REX.R
		xmmBits = 0
	case "xmm9":
		rex |= 0x04
		xmmBits = 1
	case "xmm10":
		rex |= 0x04
		xmmBits = 2
	case "xmm11":
		rex |= 0x04
		xmmBits = 3
	case "xmm12":
		rex |= 0x04
		xmmBits = 4
	case "xmm13":
		rex |= 0x04
		xmmBits = 5
	case "xmm14":
		rex |= 0x04
		xmmBits = 6
	case "xmm15":
		rex |= 0x04
		xmmBits = 7
	}

	// Build ModR/M byte
	// Mod = 11 (register-register)
	// Reg = xmm register (destination)
	// R/M = GPR (source)
	modRM := byte(0xC0) | (xmmBits << 3) | regBits

	// Emit: 66 REX.W 0F 6E ModR/M
	out.Write(0x66)
	out.Write(rex)
	out.Write(0x0F)
	out.Write(0x6E)
	out.Write(modRM)
}









