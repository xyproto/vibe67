// Completion: 95% - Comprehensive instruction set, ready for testing
// 66 instruction methods: arithmetic, logical, shifts, multiply/divide, FP, loads/stores, branches
package main

import (
	"encoding/binary"
	"fmt"
)

// RISC-V64 instruction encoding
// RISC-V uses fixed 32-bit little-endian instructions

// RISC-V Register mapping
var riscvGPRegs = map[string]uint32{
	"zero": 0, "x0": 0,
	"ra": 1, "x1": 1,
	"sp": 2, "x2": 2,
	"gp": 3, "x3": 3,
	"tp": 4, "x4": 4,
	"t0": 5, "x5": 5,
	"t1": 6, "x6": 6,
	"t2": 7, "x7": 7,
	"s0": 8, "fp": 8, "x8": 8,
	"s1": 9, "x9": 9,
	"a0": 10, "x10": 10,
	"a1": 11, "x11": 11,
	"a2": 12, "x12": 12,
	"a3": 13, "x13": 13,
	"a4": 14, "x14": 14,
	"a5": 15, "x15": 15,
	"a6": 16, "x16": 16,
	"a7": 17, "x17": 17,
	"s2": 18, "x18": 18,
	"s3": 19, "x19": 19,
	"s4": 20, "x20": 20,
	"s5": 21, "x21": 21,
	"s6": 22, "x22": 22,
	"s7": 23, "x23": 23,
	"s8": 24, "x24": 24,
	"s9": 25, "x25": 25,
	"s10": 26, "x26": 26,
	"s11": 27, "x27": 27,
	"t3": 28, "x28": 28,
	"t4": 29, "x29": 29,
	"t5": 30, "x30": 30,
	"t6": 31, "x31": 31,
}

var riscvFPRegs = map[string]uint32{
	"ft0": 0, "f0": 0,
	"ft1": 1, "f1": 1,
	"ft2": 2, "f2": 2,
	"ft3": 3, "f3": 3,
	"ft4": 4, "f4": 4,
	"ft5": 5, "f5": 5,
	"ft6": 6, "f6": 6,
	"ft7": 7, "f7": 7,
	"fs0": 8, "f8": 8,
	"fs1": 9, "f9": 9,
	"fa0": 10, "f10": 10,
	"fa1": 11, "f11": 11,
	"fa2": 12, "f12": 12,
	"fa3": 13, "f13": 13,
	"fa4": 14, "f14": 14,
	"fa5": 15, "f15": 15,
	"fa6": 16, "f16": 16,
	"fa7": 17, "f17": 17,
	"fs2": 18, "f18": 18,
	"fs3": 19, "f19": 19,
	"fs4": 20, "f20": 20,
	"fs5": 21, "f21": 21,
	"fs6": 22, "f22": 22,
	"fs7": 23, "f23": 23,
	"fs8": 24, "f24": 24,
	"fs9": 25, "f25": 25,
	"fs10": 26, "f26": 26,
	"fs11": 27, "f27": 27,
	"ft8": 28, "f28": 28,
	"ft9": 29, "f29": 29,
	"ft10": 30, "f30": 30,
	"ft11": 31, "f31": 31,
}

// RiscvOut wraps Out for RISC-V-specific instructions
type RiscvOut struct {
	out *Out
}

// encodeInstr writes a 32-bit RISC-V instruction in little-endian format
func (r *RiscvOut) encodeInstr(instr uint32) {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], instr)
	r.out.writer.WriteBytes(buf[:])
}

// RISC-V Instruction encodings

// R-type: opcode[6:0] | rd[11:7] | funct3[14:12] | rs1[19:15] | rs2[24:20] | funct7[31:25]
func (r *RiscvOut) encodeRType(opcode, funct3, funct7 uint32, rd, rs1, rs2 uint32) uint32 {
	return opcode | (rd << 7) | (funct3 << 12) | (rs1 << 15) | (rs2 << 20) | (funct7 << 25)
}

// I-type: opcode[6:0] | rd[11:7] | funct3[14:12] | rs1[19:15] | imm[31:20]
func (r *RiscvOut) encodeIType(opcode, funct3 uint32, rd, rs1 uint32, imm int32) uint32 {
	return opcode | (rd << 7) | (funct3 << 12) | (rs1 << 15) | (uint32(imm&0xfff) << 20)
}

// S-type: opcode[6:0] | imm[11:7] | funct3[14:12] | rs1[19:15] | rs2[24:20] | imm[31:25]
func (r *RiscvOut) encodeSType(opcode, funct3 uint32, rs1, rs2 uint32, imm int32) uint32 {
	imm_4_0 := uint32(imm & 0x1f)
	imm_11_5 := uint32((imm >> 5) & 0x7f)
	return opcode | (imm_4_0 << 7) | (funct3 << 12) | (rs1 << 15) | (rs2 << 20) | (imm_11_5 << 25)
}

// B-type: opcode[6:0] | imm[11|4:1] | funct3[14:12] | rs1[19:15] | rs2[24:20] | imm[12|10:5]
func (r *RiscvOut) encodeBType(opcode, funct3 uint32, rs1, rs2 uint32, imm int32) uint32 {
	imm_11 := uint32((imm >> 11) & 0x1)
	imm_4_1 := uint32((imm >> 1) & 0xf)
	imm_10_5 := uint32((imm >> 5) & 0x3f)
	imm_12 := uint32((imm >> 12) & 0x1)
	return opcode | (imm_11 << 7) | (imm_4_1 << 8) | (funct3 << 12) | (rs1 << 15) | (rs2 << 20) | (imm_10_5 << 25) | (imm_12 << 31)
}

// U-type: opcode[6:0] | rd[11:7] | imm[31:12]
func (r *RiscvOut) encodeUType(opcode uint32, rd uint32, imm uint32) uint32 {
	return opcode | (rd << 7) | (imm & 0xfffff000)
}

// J-type: opcode[6:0] | rd[11:7] | imm[19:12|11|10:1|20]
func (r *RiscvOut) encodeJType(opcode uint32, rd uint32, imm int32) uint32 {
	imm_19_12 := uint32((imm >> 12) & 0xff)
	imm_11 := uint32((imm >> 11) & 0x1)
	imm_10_1 := uint32((imm >> 1) & 0x3ff)
	imm_20 := uint32((imm >> 20) & 0x1)
	return opcode | (rd << 7) | (imm_19_12 << 12) | (imm_11 << 20) | (imm_10_1 << 21) | (imm_20 << 31)
}

// ADD: add rd, rs1, rs2
func (r *RiscvOut) Add(dest, src1, src2 string) error {
	rd, ok := riscvGPRegs[dest]
	if !ok {
		return fmt.Errorf("invalid RISC-V register: %s", dest)
	}
	rs1, ok := riscvGPRegs[src1]
	if !ok {
		return fmt.Errorf("invalid RISC-V register: %s", src1)
	}
	rs2, ok := riscvGPRegs[src2]
	if !ok {
		return fmt.Errorf("invalid RISC-V register: %s", src2)
	}

	// ADD: opcode=0110011, funct3=000, funct7=0000000
	instr := r.encodeRType(0x33, 0x0, 0x00, rd, rs1, rs2)
	r.encodeInstr(instr)
	return nil
}

// ADDI: addi rd, rs1, imm
func (r *RiscvOut) AddImm(dest, src string, imm int32) error {
	rd, ok := riscvGPRegs[dest]
	if !ok {
		return fmt.Errorf("invalid RISC-V register: %s", dest)
	}
	rs1, ok := riscvGPRegs[src]
	if !ok {
		return fmt.Errorf("invalid RISC-V register: %s", src)
	}

	// Check immediate range (-2048 to 2047)
	if imm < -2048 || imm > 2047 {
		return fmt.Errorf("immediate value out of range for ADDI: %d", imm)
	}

	// ADDI: opcode=0010011, funct3=000
	instr := r.encodeIType(0x13, 0x0, rd, rs1, imm)
	r.encodeInstr(instr)
	return nil
}

// SUB: sub rd, rs1, rs2
func (r *RiscvOut) Sub(dest, src1, src2 string) error {
	rd, ok := riscvGPRegs[dest]
	if !ok {
		return fmt.Errorf("invalid RISC-V register: %s", dest)
	}
	rs1, ok := riscvGPRegs[src1]
	if !ok {
		return fmt.Errorf("invalid RISC-V register: %s", src1)
	}
	rs2, ok := riscvGPRegs[src2]
	if !ok {
		return fmt.Errorf("invalid RISC-V register: %s", src2)
	}

	// SUB: opcode=0110011, funct3=000, funct7=0100000
	instr := r.encodeRType(0x33, 0x0, 0x20, rd, rs1, rs2)
	r.encodeInstr(instr)
	return nil
}

// MV (pseudo-instruction): addi rd, rs, 0
func (r *RiscvOut) Move(dest, src string) error {
	return r.AddImm(dest, src, 0)
}

// LI (load immediate, pseudo-instruction)
func (r *RiscvOut) LoadImm(dest string, imm int64) error {
	rd, ok := riscvGPRegs[dest]
	if !ok {
		return fmt.Errorf("invalid RISC-V register: %s", dest)
	}

	// For small immediates, use ADDI
	if imm >= -2048 && imm <= 2047 {
		return r.AddImm(dest, "zero", int32(imm))
	}

	// For larger immediates, use LUI + ADDI
	// LUI loads upper 20 bits, ADDI adds lower 12 bits
	upper := uint32((imm + 0x800) >> 12) // Add 0x800 for sign extension compensation
	lower := int32(imm & 0xfff)

	// LUI: opcode=0110111
	instr := r.encodeUType(0x37, rd, upper<<12)
	r.encodeInstr(instr)

	// ADDI to add lower bits (if non-zero)
	if lower != 0 {
		instr = r.encodeIType(0x13, 0x0, rd, rd, lower)
		r.encodeInstr(instr)
	}

	return nil
}

// LD: ld rd, offset(rs1)
func (r *RiscvOut) Load64(dest, base string, offset int32) error {
	rd, ok := riscvGPRegs[dest]
	if !ok {
		return fmt.Errorf("invalid RISC-V register: %s", dest)
	}
	rs1, ok := riscvGPRegs[base]
	if !ok {
		return fmt.Errorf("invalid RISC-V register: %s", base)
	}

	if offset < -2048 || offset > 2047 {
		return fmt.Errorf("load offset out of range: %d", offset)
	}

	// LD: opcode=0000011, funct3=011
	instr := r.encodeIType(0x03, 0x3, rd, rs1, offset)
	r.encodeInstr(instr)
	return nil
}

// SD: sd rs2, offset(rs1)
func (r *RiscvOut) Store64(src, base string, offset int32) error {
	rs2, ok := riscvGPRegs[src]
	if !ok {
		return fmt.Errorf("invalid RISC-V register: %s", src)
	}
	rs1, ok := riscvGPRegs[base]
	if !ok {
		return fmt.Errorf("invalid RISC-V register: %s", base)
	}

	if offset < -2048 || offset > 2047 {
		return fmt.Errorf("store offset out of range: %d", offset)
	}

	// SD: opcode=0100011, funct3=011
	instr := r.encodeSType(0x23, 0x3, rs1, rs2, offset)
	r.encodeInstr(instr)
	return nil
}

// JAL: jal rd, offset
func (r *RiscvOut) JumpAndLink(dest string, offset int32) error {
	rd, ok := riscvGPRegs[dest]
	if !ok {
		return fmt.Errorf("invalid RISC-V register: %s", dest)
	}

	// Offset must be even and within ±1MB
	if offset%2 != 0 {
		return fmt.Errorf("JAL offset must be even: %d", offset)
	}
	if offset < -(1<<20) || offset >= (1<<20) {
		return fmt.Errorf("JAL offset out of range: %d", offset)
	}

	// JAL: opcode=1101111
	instr := r.encodeJType(0x6f, rd, offset)
	r.encodeInstr(instr)
	return nil
}

// JALR: jalr rd, offset(rs1)
func (r *RiscvOut) JumpAndLinkRegister(dest, base string, offset int32) error {
	rd, ok := riscvGPRegs[dest]
	if !ok {
		return fmt.Errorf("invalid RISC-V register: %s", dest)
	}
	rs1, ok := riscvGPRegs[base]
	if !ok {
		return fmt.Errorf("invalid RISC-V register: %s", base)
	}

	if offset < -2048 || offset > 2047 {
		return fmt.Errorf("JALR offset out of range: %d", offset)
	}

	// JALR: opcode=1100111, funct3=000
	instr := r.encodeIType(0x67, 0x0, rd, rs1, offset)
	r.encodeInstr(instr)
	return nil
}

// BEQ: beq rs1, rs2, offset
func (r *RiscvOut) BranchEqual(src1, src2 string, offset int32) error {
	rs1, ok := riscvGPRegs[src1]
	if !ok {
		return fmt.Errorf("invalid RISC-V register: %s", src1)
	}
	rs2, ok := riscvGPRegs[src2]
	if !ok {
		return fmt.Errorf("invalid RISC-V register: %s", src2)
	}

	// Offset must be even and within ±4KB
	if offset%2 != 0 {
		return fmt.Errorf("branch offset must be even: %d", offset)
	}
	if offset < -(1<<12) || offset >= (1<<12) {
		return fmt.Errorf("branch offset out of range: %d", offset)
	}

	// BEQ: opcode=1100011, funct3=000
	instr := r.encodeBType(0x63, 0x0, rs1, rs2, offset)
	r.encodeInstr(instr)
	return nil
}

// BNE: bne rs1, rs2, offset
func (r *RiscvOut) BranchNotEqual(src1, src2 string, offset int32) error {
	rs1, ok := riscvGPRegs[src1]
	if !ok {
		return fmt.Errorf("invalid RISC-V register: %s", src1)
	}
	rs2, ok := riscvGPRegs[src2]
	if !ok {
		return fmt.Errorf("invalid RISC-V register: %s", src2)
	}

	if offset%2 != 0 {
		return fmt.Errorf("branch offset must be even: %d", offset)
	}
	if offset < -(1<<12) || offset >= (1<<12) {
		return fmt.Errorf("branch offset out of range: %d", offset)
	}

	// BNE: opcode=1100011, funct3=001
	instr := r.encodeBType(0x63, 0x1, rs1, rs2, offset)
	r.encodeInstr(instr)
	return nil
}

// RET (pseudo-instruction): jalr zero, 0(ra)
func (r *RiscvOut) Return() error {
	return r.JumpAndLinkRegister("zero", "ra", 0)
}

// ECALL: system call
func (r *RiscvOut) Ecall() {
	// ECALL: opcode=1110011, funct3=000, imm=0
	instr := r.encodeIType(0x73, 0x0, 0, 0, 0)
	r.encodeInstr(instr)
}

// Multiply/Divide Instructions (RV64M extension)

// Mul: rd = rs1 * rs2 (lower 64 bits)
func (r *RiscvOut) Mul(dest, src1, src2 string) error {
	rd, ok1 := riscvGPRegs[dest]
	rs1, ok2 := riscvGPRegs[src1]
	rs2, ok3 := riscvGPRegs[src2]
	if !ok1 || !ok2 || !ok3 {
		return fmt.Errorf("invalid register in mul %s, %s, %s", dest, src1, src2)
	}
	// MUL: opcode=0110011, funct3=000, funct7=0000001
	instr := r.encodeRType(0x33, 0x0, 0x01, rd, rs1, rs2)
	r.encodeInstr(instr)
	return nil
}

// Mulw: rd = (rs1 * rs2)[31:0] sign-extended
func (r *RiscvOut) Mulw(dest, src1, src2 string) error {
	rd, ok1 := riscvGPRegs[dest]
	rs1, ok2 := riscvGPRegs[src1]
	rs2, ok3 := riscvGPRegs[src2]
	if !ok1 || !ok2 || !ok3 {
		return fmt.Errorf("invalid register in mulw %s, %s, %s", dest, src1, src2)
	}
	// MULW: opcode=0111011, funct3=000, funct7=0000001
	instr := r.encodeRType(0x3b, 0x0, 0x01, rd, rs1, rs2)
	r.encodeInstr(instr)
	return nil
}

// Div: rd = rs1 / rs2 (signed)
func (r *RiscvOut) Div(dest, src1, src2 string) error {
	rd, ok1 := riscvGPRegs[dest]
	rs1, ok2 := riscvGPRegs[src1]
	rs2, ok3 := riscvGPRegs[src2]
	if !ok1 || !ok2 || !ok3 {
		return fmt.Errorf("invalid register in div %s, %s, %s", dest, src1, src2)
	}
	// DIV: opcode=0110011, funct3=100, funct7=0000001
	instr := r.encodeRType(0x33, 0x4, 0x01, rd, rs1, rs2)
	r.encodeInstr(instr)
	return nil
}

// Divu: rd = rs1 / rs2 (unsigned)
func (r *RiscvOut) Divu(dest, src1, src2 string) error {
	rd, ok1 := riscvGPRegs[dest]
	rs1, ok2 := riscvGPRegs[src1]
	rs2, ok3 := riscvGPRegs[src2]
	if !ok1 || !ok2 || !ok3 {
		return fmt.Errorf("invalid register in divu %s, %s, %s", dest, src1, src2)
	}
	// DIVU: opcode=0110011, funct3=101, funct7=0000001
	instr := r.encodeRType(0x33, 0x5, 0x01, rd, rs1, rs2)
	r.encodeInstr(instr)
	return nil
}

// Rem: rd = rs1 % rs2 (signed)
func (r *RiscvOut) Rem(dest, src1, src2 string) error {
	rd, ok1 := riscvGPRegs[dest]
	rs1, ok2 := riscvGPRegs[src1]
	rs2, ok3 := riscvGPRegs[src2]
	if !ok1 || !ok2 || !ok3 {
		return fmt.Errorf("invalid register in rem %s, %s, %s", dest, src1, src2)
	}
	// REM: opcode=0110011, funct3=110, funct7=0000001
	instr := r.encodeRType(0x33, 0x6, 0x01, rd, rs1, rs2)
	r.encodeInstr(instr)
	return nil
}

// Remu: rd = rs1 % rs2 (unsigned)
func (r *RiscvOut) Remu(dest, src1, src2 string) error {
	rd, ok1 := riscvGPRegs[dest]
	rs1, ok2 := riscvGPRegs[src1]
	rs2, ok3 := riscvGPRegs[src2]
	if !ok1 || !ok2 || !ok3 {
		return fmt.Errorf("invalid register in remu %s, %s, %s", dest, src1, src2)
	}
	// REMU: opcode=0110011, funct3=111, funct7=0000001
	instr := r.encodeRType(0x33, 0x7, 0x01, rd, rs1, rs2)
	r.encodeInstr(instr)
	return nil
}

// Logical Instructions

// And: rd = rs1 & rs2
func (r *RiscvOut) And(dest, src1, src2 string) error {
	rd, ok1 := riscvGPRegs[dest]
	rs1, ok2 := riscvGPRegs[src1]
	rs2, ok3 := riscvGPRegs[src2]
	if !ok1 || !ok2 || !ok3 {
		return fmt.Errorf("invalid register in and %s, %s, %s", dest, src1, src2)
	}
	// AND: opcode=0110011, funct3=111, funct7=0000000
	instr := r.encodeRType(0x33, 0x7, 0x00, rd, rs1, rs2)
	r.encodeInstr(instr)
	return nil
}

// Andi: rd = rs1 & imm
func (r *RiscvOut) Andi(dest, src string, imm int32) error {
	rd, ok1 := riscvGPRegs[dest]
	rs1, ok2 := riscvGPRegs[src]
	if !ok1 || !ok2 {
		return fmt.Errorf("invalid register in andi %s, %s", dest, src)
	}
	// ANDI: opcode=0010011, funct3=111
	instr := r.encodeIType(0x13, 0x7, rd, rs1, imm)
	r.encodeInstr(instr)
	return nil
}

// Or: rd = rs1 | rs2
func (r *RiscvOut) Or(dest, src1, src2 string) error {
	rd, ok1 := riscvGPRegs[dest]
	rs1, ok2 := riscvGPRegs[src1]
	rs2, ok3 := riscvGPRegs[src2]
	if !ok1 || !ok2 || !ok3 {
		return fmt.Errorf("invalid register in or %s, %s, %s", dest, src1, src2)
	}
	// OR: opcode=0110011, funct3=110, funct7=0000000
	instr := r.encodeRType(0x33, 0x6, 0x00, rd, rs1, rs2)
	r.encodeInstr(instr)
	return nil
}

// Ori: rd = rs1 | imm
func (r *RiscvOut) Ori(dest, src string, imm int32) error {
	rd, ok1 := riscvGPRegs[dest]
	rs1, ok2 := riscvGPRegs[src]
	if !ok1 || !ok2 {
		return fmt.Errorf("invalid register in ori %s, %s", dest, src)
	}
	// ORI: opcode=0010011, funct3=110
	instr := r.encodeIType(0x13, 0x6, rd, rs1, imm)
	r.encodeInstr(instr)
	return nil
}

// Xor: rd = rs1 ^ rs2
func (r *RiscvOut) Xor(dest, src1, src2 string) error {
	rd, ok1 := riscvGPRegs[dest]
	rs1, ok2 := riscvGPRegs[src1]
	rs2, ok3 := riscvGPRegs[src2]
	if !ok1 || !ok2 || !ok3 {
		return fmt.Errorf("invalid register in xor %s, %s, %s", dest, src1, src2)
	}
	// XOR: opcode=0110011, funct3=100, funct7=0000000
	instr := r.encodeRType(0x33, 0x4, 0x00, rd, rs1, rs2)
	r.encodeInstr(instr)
	return nil
}

// Xori: rd = rs1 ^ imm
func (r *RiscvOut) Xori(dest, src string, imm int32) error {
	rd, ok1 := riscvGPRegs[dest]
	rs1, ok2 := riscvGPRegs[src]
	if !ok1 || !ok2 {
		return fmt.Errorf("invalid register in xori %s, %s", dest, src)
	}
	// XORI: opcode=0010011, funct3=100
	instr := r.encodeIType(0x13, 0x4, rd, rs1, imm)
	r.encodeInstr(instr)
	return nil
}

// Shift Instructions

// Sll: rd = rs1 << rs2 (logical left)
func (r *RiscvOut) Sll(dest, src1, src2 string) error {
	rd, ok1 := riscvGPRegs[dest]
	rs1, ok2 := riscvGPRegs[src1]
	rs2, ok3 := riscvGPRegs[src2]
	if !ok1 || !ok2 || !ok3 {
		return fmt.Errorf("invalid register in sll %s, %s, %s", dest, src1, src2)
	}
	// SLL: opcode=0110011, funct3=001, funct7=0000000
	instr := r.encodeRType(0x33, 0x1, 0x00, rd, rs1, rs2)
	r.encodeInstr(instr)
	return nil
}

// Slli: rd = rs1 << shamt (logical left immediate)
func (r *RiscvOut) Slli(dest, src string, shamt uint32) error {
	rd, ok1 := riscvGPRegs[dest]
	rs1, ok2 := riscvGPRegs[src]
	if !ok1 || !ok2 {
		return fmt.Errorf("invalid register in slli %s, %s", dest, src)
	}
	// SLLI: opcode=0010011, funct3=001, imm[11:6]=000000, imm[5:0]=shamt
	instr := r.encodeIType(0x13, 0x1, rd, rs1, int32(shamt&0x3f))
	r.encodeInstr(instr)
	return nil
}

// Srl: rd = rs1 >> rs2 (logical right)
func (r *RiscvOut) Srl(dest, src1, src2 string) error {
	rd, ok1 := riscvGPRegs[dest]
	rs1, ok2 := riscvGPRegs[src1]
	rs2, ok3 := riscvGPRegs[src2]
	if !ok1 || !ok2 || !ok3 {
		return fmt.Errorf("invalid register in srl %s, %s, %s", dest, src1, src2)
	}
	// SRL: opcode=0110011, funct3=101, funct7=0000000
	instr := r.encodeRType(0x33, 0x5, 0x00, rd, rs1, rs2)
	r.encodeInstr(instr)
	return nil
}

// Srli: rd = rs1 >> shamt (logical right immediate)
func (r *RiscvOut) Srli(dest, src string, shamt uint32) error {
	rd, ok1 := riscvGPRegs[dest]
	rs1, ok2 := riscvGPRegs[src]
	if !ok1 || !ok2 {
		return fmt.Errorf("invalid register in srli %s, %s", dest, src)
	}
	// SRLI: opcode=0010011, funct3=101, imm[11:6]=000000, imm[5:0]=shamt
	instr := r.encodeIType(0x13, 0x5, rd, rs1, int32(shamt&0x3f))
	r.encodeInstr(instr)
	return nil
}

// Sra: rd = rs1 >> rs2 (arithmetic right)
func (r *RiscvOut) Sra(dest, src1, src2 string) error {
	rd, ok1 := riscvGPRegs[dest]
	rs1, ok2 := riscvGPRegs[src1]
	rs2, ok3 := riscvGPRegs[src2]
	if !ok1 || !ok2 || !ok3 {
		return fmt.Errorf("invalid register in sra %s, %s, %s", dest, src1, src2)
	}
	// SRA: opcode=0110011, funct3=101, funct7=0100000
	instr := r.encodeRType(0x33, 0x5, 0x20, rd, rs1, rs2)
	r.encodeInstr(instr)
	return nil
}

// Srai: rd = rs1 >> shamt (arithmetic right immediate)
func (r *RiscvOut) Srai(dest, src string, shamt uint32) error {
	rd, ok1 := riscvGPRegs[dest]
	rs1, ok2 := riscvGPRegs[src]
	if !ok1 || !ok2 {
		return fmt.Errorf("invalid register in srai %s, %s", dest, src)
	}
	// SRAI: opcode=0010011, funct3=101, imm[11:6]=010000, imm[5:0]=shamt
	imm := int32(0x400 | (shamt & 0x3f))
	instr := r.encodeIType(0x13, 0x5, rd, rs1, imm)
	r.encodeInstr(instr)
	return nil
}

// Comparison and Set Instructions

// Slt: rd = (rs1 < rs2) ? 1 : 0 (signed)
func (r *RiscvOut) Slt(dest, src1, src2 string) error {
	rd, ok1 := riscvGPRegs[dest]
	rs1, ok2 := riscvGPRegs[src1]
	rs2, ok3 := riscvGPRegs[src2]
	if !ok1 || !ok2 || !ok3 {
		return fmt.Errorf("invalid register in slt %s, %s, %s", dest, src1, src2)
	}
	// SLT: opcode=0110011, funct3=010, funct7=0000000
	instr := r.encodeRType(0x33, 0x2, 0x00, rd, rs1, rs2)
	r.encodeInstr(instr)
	return nil
}

// Slti: rd = (rs1 < imm) ? 1 : 0 (signed)
func (r *RiscvOut) Slti(dest, src string, imm int32) error {
	rd, ok1 := riscvGPRegs[dest]
	rs1, ok2 := riscvGPRegs[src]
	if !ok1 || !ok2 {
		return fmt.Errorf("invalid register in slti %s, %s", dest, src)
	}
	// SLTI: opcode=0010011, funct3=010
	instr := r.encodeIType(0x13, 0x2, rd, rs1, imm)
	r.encodeInstr(instr)
	return nil
}

// Sltu: rd = (rs1 < rs2) ? 1 : 0 (unsigned)
func (r *RiscvOut) Sltu(dest, src1, src2 string) error {
	rd, ok1 := riscvGPRegs[dest]
	rs1, ok2 := riscvGPRegs[src1]
	rs2, ok3 := riscvGPRegs[src2]
	if !ok1 || !ok2 || !ok3 {
		return fmt.Errorf("invalid register in sltu %s, %s, %s", dest, src1, src2)
	}
	// SLTU: opcode=0110011, funct3=011, funct7=0000000
	instr := r.encodeRType(0x33, 0x3, 0x00, rd, rs1, rs2)
	r.encodeInstr(instr)
	return nil
}

// Sltiu: rd = (rs1 < imm) ? 1 : 0 (unsigned)
func (r *RiscvOut) Sltiu(dest, src string, imm int32) error {
	rd, ok1 := riscvGPRegs[dest]
	rs1, ok2 := riscvGPRegs[src]
	if !ok1 || !ok2 {
		return fmt.Errorf("invalid register in sltiu %s, %s", dest, src)
	}
	// SLTIU: opcode=0010011, funct3=011
	instr := r.encodeIType(0x13, 0x3, rd, rs1, imm)
	r.encodeInstr(instr)
	return nil
}

// Floating-Point Instructions (RV64D extension)

// FaddD: fd = fs1 + fs2 (double precision)
func (r *RiscvOut) FaddD(dest, src1, src2 string) error {
	fd, ok1 := riscvFPRegs[dest]
	fs1, ok2 := riscvFPRegs[src1]
	fs2, ok3 := riscvFPRegs[src2]
	if !ok1 || !ok2 || !ok3 {
		return fmt.Errorf("invalid FP register in fadd.d %s, %s, %s", dest, src1, src2)
	}
	// FADD.D: opcode=1010011, funct3=000 (RNE rounding), funct7=0000001
	instr := r.encodeRType(0x53, 0x0, 0x01, fd, fs1, fs2)
	r.encodeInstr(instr)
	return nil
}

// FsubD: fd = fs1 - fs2 (double precision)
func (r *RiscvOut) FsubD(dest, src1, src2 string) error {
	fd, ok1 := riscvFPRegs[dest]
	fs1, ok2 := riscvFPRegs[src1]
	fs2, ok3 := riscvFPRegs[src2]
	if !ok1 || !ok2 || !ok3 {
		return fmt.Errorf("invalid FP register in fsub.d %s, %s, %s", dest, src1, src2)
	}
	// FSUB.D: opcode=1010011, funct3=000, funct7=0000101
	instr := r.encodeRType(0x53, 0x0, 0x05, fd, fs1, fs2)
	r.encodeInstr(instr)
	return nil
}

// FmulD: fd = fs1 * fs2 (double precision)
func (r *RiscvOut) FmulD(dest, src1, src2 string) error {
	fd, ok1 := riscvFPRegs[dest]
	fs1, ok2 := riscvFPRegs[src1]
	fs2, ok3 := riscvFPRegs[src2]
	if !ok1 || !ok2 || !ok3 {
		return fmt.Errorf("invalid FP register in fmul.d %s, %s, %s", dest, src1, src2)
	}
	// FMUL.D: opcode=1010011, funct3=000, funct7=0001001
	instr := r.encodeRType(0x53, 0x0, 0x09, fd, fs1, fs2)
	r.encodeInstr(instr)
	return nil
}

// FdivD: fd = fs1 / fs2 (double precision)
func (r *RiscvOut) FdivD(dest, src1, src2 string) error {
	fd, ok1 := riscvFPRegs[dest]
	fs1, ok2 := riscvFPRegs[src1]
	fs2, ok3 := riscvFPRegs[src2]
	if !ok1 || !ok2 || !ok3 {
		return fmt.Errorf("invalid FP register in fdiv.d %s, %s, %s", dest, src1, src2)
	}
	// FDIV.D: opcode=1010011, funct3=000, funct7=0001101
	instr := r.encodeRType(0x53, 0x0, 0x0d, fd, fs1, fs2)
	r.encodeInstr(instr)
	return nil
}

// FsqrtD: fd = sqrt(fs1) (double precision)
func (r *RiscvOut) FsqrtD(dest, src string) error {
	fd, ok1 := riscvFPRegs[dest]
	fs1, ok2 := riscvFPRegs[src]
	if !ok1 || !ok2 {
		return fmt.Errorf("invalid FP register in fsqrt.d %s, %s", dest, src)
	}
	// FSQRT.D: opcode=1010011, funct3=000, rs2=00000, funct7=0101101
	instr := r.encodeRType(0x53, 0x0, 0x2d, fd, fs1, 0)
	r.encodeInstr(instr)
	return nil
}

// FcvtDW: fd = (double)rs1 (int32 to double)
func (r *RiscvOut) FcvtDW(dest, src string) error {
	fd, ok1 := riscvFPRegs[dest]
	rs1, ok2 := riscvGPRegs[src]
	if !ok1 || !ok2 {
		return fmt.Errorf("invalid register in fcvt.d.w %s, %s", dest, src)
	}
	// FCVT.D.W: opcode=1010011, funct3=000, rs2=00000, funct7=1101001
	instr := r.encodeRType(0x53, 0x0, 0x69, fd, rs1, 0)
	r.encodeInstr(instr)
	return nil
}

// FcvtDL: fd = (double)rs1 (int64 to double)
func (r *RiscvOut) FcvtDL(dest, src string) error {
	fd, ok1 := riscvFPRegs[dest]
	rs1, ok2 := riscvGPRegs[src]
	if !ok1 || !ok2 {
		return fmt.Errorf("invalid register in fcvt.d.l %s, %s", dest, src)
	}
	// FCVT.D.L: opcode=1010011, funct3=000, rs2=00010, funct7=1101001
	instr := r.encodeRType(0x53, 0x0, 0x69, fd, rs1, 2)
	r.encodeInstr(instr)
	return nil
}

// FcvtWD: rd = (int32)fs1 (double to int32)
func (r *RiscvOut) FcvtWD(dest, src string) error {
	rd, ok1 := riscvGPRegs[dest]
	fs1, ok2 := riscvFPRegs[src]
	if !ok1 || !ok2 {
		return fmt.Errorf("invalid register in fcvt.w.d %s, %s", dest, src)
	}
	// FCVT.W.D: opcode=1010011, funct3=001 (RTZ), rs2=00000, funct7=1100001
	instr := r.encodeRType(0x53, 0x1, 0x61, rd, fs1, 0)
	r.encodeInstr(instr)
	return nil
}

// FcvtLD: rd = (int64)fs1 (double to int64)
func (r *RiscvOut) FcvtLD(dest, src string) error {
	rd, ok1 := riscvGPRegs[dest]
	fs1, ok2 := riscvFPRegs[src]
	if !ok1 || !ok2 {
		return fmt.Errorf("invalid register in fcvt.l.d %s, %s", dest, src)
	}
	// FCVT.L.D: opcode=1010011, funct3=001 (RTZ), rs2=00010, funct7=1100001
	instr := r.encodeRType(0x53, 0x1, 0x61, rd, fs1, 2)
	r.encodeInstr(instr)
	return nil
}

// FLD: fd = mem[rs1 + offset] (load double)
func (r *RiscvOut) Fld(dest, base string, offset int32) error {
	fd, ok1 := riscvFPRegs[dest]
	rs1, ok2 := riscvGPRegs[base]
	if !ok1 || !ok2 {
		return fmt.Errorf("invalid register in fld %s, %s", dest, base)
	}
	// FLD: opcode=0000111, funct3=011
	instr := r.encodeIType(0x07, 0x3, fd, rs1, offset)
	r.encodeInstr(instr)
	return nil
}

// FSD: mem[rs1 + offset] = fs2 (store double)
func (r *RiscvOut) Fsd(src, base string, offset int32) error {
	fs2, ok1 := riscvFPRegs[src]
	rs1, ok2 := riscvGPRegs[base]
	if !ok1 || !ok2 {
		return fmt.Errorf("invalid register in fsd %s, %s", src, base)
	}
	// FSD: opcode=0100111, funct3=011
	instr := r.encodeSType(0x27, 0x3, rs1, fs2, offset)
	r.encodeInstr(instr)
	return nil
}

// Branch comparisons for less than

// Blt: if (rs1 < rs2) goto PC + offset (signed)
func (r *RiscvOut) Blt(src1, src2 string, offset int32) error {
	rs1, ok1 := riscvGPRegs[src1]
	rs2, ok2 := riscvGPRegs[src2]
	if !ok1 || !ok2 {
		return fmt.Errorf("invalid register in blt %s, %s", src1, src2)
	}
	// BLT: opcode=1100011, funct3=100
	instr := r.encodeBType(0x63, 0x4, rs1, rs2, offset)
	r.encodeInstr(instr)
	return nil
}

// Bge: if (rs1 >= rs2) goto PC + offset (signed)
func (r *RiscvOut) Bge(src1, src2 string, offset int32) error {
	rs1, ok1 := riscvGPRegs[src1]
	rs2, ok2 := riscvGPRegs[src2]
	if !ok1 || !ok2 {
		return fmt.Errorf("invalid register in bge %s, %s", src1, src2)
	}
	// BGE: opcode=1100011, funct3=101
	instr := r.encodeBType(0x63, 0x5, rs1, rs2, offset)
	r.encodeInstr(instr)
	return nil
}

// Bltu: if (rs1 < rs2) goto PC + offset (unsigned)
func (r *RiscvOut) Bltu(src1, src2 string, offset int32) error {
	rs1, ok1 := riscvGPRegs[src1]
	rs2, ok2 := riscvGPRegs[src2]
	if !ok1 || !ok2 {
		return fmt.Errorf("invalid register in bltu %s, %s", src1, src2)
	}
	// BLTU: opcode=1100011, funct3=110
	instr := r.encodeBType(0x63, 0x6, rs1, rs2, offset)
	r.encodeInstr(instr)
	return nil
}

// Bgeu: if (rs1 >= rs2) goto PC + offset (unsigned)
func (r *RiscvOut) Bgeu(src1, src2 string, offset int32) error {
	rs1, ok1 := riscvGPRegs[src1]
	rs2, ok2 := riscvGPRegs[src2]
	if !ok1 || !ok2 {
		return fmt.Errorf("invalid register in bgeu %s, %s", src1, src2)
	}
	// BGEU: opcode=1100011, funct3=111
	instr := r.encodeBType(0x63, 0x7, rs1, rs2, offset)
	r.encodeInstr(instr)
	return nil
}

// Additional load/store instructions

// Lw: rd = (int32)mem[rs1 + offset] (load word, sign-extended)
func (r *RiscvOut) Lw(dest, base string, offset int32) error {
	rd, ok1 := riscvGPRegs[dest]
	rs1, ok2 := riscvGPRegs[base]
	if !ok1 || !ok2 {
		return fmt.Errorf("invalid register in lw %s, %s", dest, base)
	}
	// LW: opcode=0000011, funct3=010
	instr := r.encodeIType(0x03, 0x2, rd, rs1, offset)
	r.encodeInstr(instr)
	return nil
}

// Lwu: rd = (uint32)mem[rs1 + offset] (load word, zero-extended)
func (r *RiscvOut) Lwu(dest, base string, offset int32) error {
	rd, ok1 := riscvGPRegs[dest]
	rs1, ok2 := riscvGPRegs[base]
	if !ok1 || !ok2 {
		return fmt.Errorf("invalid register in lwu %s, %s", dest, base)
	}
	// LWU: opcode=0000011, funct3=110
	instr := r.encodeIType(0x03, 0x6, rd, rs1, offset)
	r.encodeInstr(instr)
	return nil
}

// Lh: rd = (int16)mem[rs1 + offset] (load halfword, sign-extended)
func (r *RiscvOut) Lh(dest, base string, offset int32) error {
	rd, ok1 := riscvGPRegs[dest]
	rs1, ok2 := riscvGPRegs[base]
	if !ok1 || !ok2 {
		return fmt.Errorf("invalid register in lh %s, %s", dest, base)
	}
	// LH: opcode=0000011, funct3=001
	instr := r.encodeIType(0x03, 0x1, rd, rs1, offset)
	r.encodeInstr(instr)
	return nil
}

// Lhu: rd = (uint16)mem[rs1 + offset] (load halfword, zero-extended)
func (r *RiscvOut) Lhu(dest, base string, offset int32) error {
	rd, ok1 := riscvGPRegs[dest]
	rs1, ok2 := riscvGPRegs[base]
	if !ok1 || !ok2 {
		return fmt.Errorf("invalid register in lhu %s, %s", dest, base)
	}
	// LHU: opcode=0000011, funct3=101
	instr := r.encodeIType(0x03, 0x5, rd, rs1, offset)
	r.encodeInstr(instr)
	return nil
}

// Lb: rd = (int8)mem[rs1 + offset] (load byte, sign-extended)
func (r *RiscvOut) Lb(dest, base string, offset int32) error {
	rd, ok1 := riscvGPRegs[dest]
	rs1, ok2 := riscvGPRegs[base]
	if !ok1 || !ok2 {
		return fmt.Errorf("invalid register in lb %s, %s", dest, base)
	}
	// LB: opcode=0000011, funct3=000
	instr := r.encodeIType(0x03, 0x0, rd, rs1, offset)
	r.encodeInstr(instr)
	return nil
}

// Lbu: rd = (uint8)mem[rs1 + offset] (load byte, zero-extended)
func (r *RiscvOut) Lbu(dest, base string, offset int32) error {
	rd, ok1 := riscvGPRegs[dest]
	rs1, ok2 := riscvGPRegs[base]
	if !ok1 || !ok2 {
		return fmt.Errorf("invalid register in lbu %s, %s", dest, base)
	}
	// LBU: opcode=0000011, funct3=100
	instr := r.encodeIType(0x03, 0x4, rd, rs1, offset)
	r.encodeInstr(instr)
	return nil
}

// Sw: mem[rs1 + offset] = rs2[31:0] (store word)
func (r *RiscvOut) Sw(src, base string, offset int32) error {
	rs2, ok1 := riscvGPRegs[src]
	rs1, ok2 := riscvGPRegs[base]
	if !ok1 || !ok2 {
		return fmt.Errorf("invalid register in sw %s, %s", src, base)
	}
	// SW: opcode=0100011, funct3=010
	instr := r.encodeSType(0x23, 0x2, rs1, rs2, offset)
	r.encodeInstr(instr)
	return nil
}

// Sh: mem[rs1 + offset] = rs2[15:0] (store halfword)
func (r *RiscvOut) Sh(src, base string, offset int32) error {
	rs2, ok1 := riscvGPRegs[src]
	rs1, ok2 := riscvGPRegs[base]
	if !ok1 || !ok2 {
		return fmt.Errorf("invalid register in sh %s, %s", src, base)
	}
	// SH: opcode=0100011, funct3=001
	instr := r.encodeSType(0x23, 0x1, rs1, rs2, offset)
	r.encodeInstr(instr)
	return nil
}

// Sb: mem[rs1 + offset] = rs2[7:0] (store byte)
func (r *RiscvOut) Sb(src, base string, offset int32) error {
	rs2, ok1 := riscvGPRegs[src]
	rs1, ok2 := riscvGPRegs[base]
	if !ok1 || !ok2 {
		return fmt.Errorf("invalid register in sb %s, %s", src, base)
	}
	// SB: opcode=0100011, funct3=000
	instr := r.encodeSType(0x23, 0x0, rs1, rs2, offset)
	r.encodeInstr(instr)
	return nil
}

// LeaSymbolToReg loads the effective address of a symbol (PC-relative)
func (r *RiscvOut) LeaSymbolToReg(dst, symbol string) {
	// Delegate to the RISC-V backend's implementation
	if r.out.backend != nil {
		if rvBackend, ok := r.out.backend.(*RISCV64Backend); ok {
			rvBackend.LeaSymbolToReg(dst, symbol)
		}
	}
}









