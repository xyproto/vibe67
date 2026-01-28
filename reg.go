// Completion: 100% - Utility module complete
package main

// Register definitions for all supported architectures

type Register struct {
	Name     string
	Size     int   // Size in bits
	Encoding uint8 // Encoding for instruction generation
}

// x86_64 registers
var x86_64Registers = map[string]Register{
	// 64-bit general purpose registers
	"rax": {Name: "rax", Size: 64, Encoding: 0},
	"rcx": {Name: "rcx", Size: 64, Encoding: 1},
	"rdx": {Name: "rdx", Size: 64, Encoding: 2},
	"rbx": {Name: "rbx", Size: 64, Encoding: 3},
	"rsp": {Name: "rsp", Size: 64, Encoding: 4},
	"rbp": {Name: "rbp", Size: 64, Encoding: 5},
	"rsi": {Name: "rsi", Size: 64, Encoding: 6},
	"rdi": {Name: "rdi", Size: 64, Encoding: 7},
	"r8":  {Name: "r8", Size: 64, Encoding: 8},
	"r9":  {Name: "r9", Size: 64, Encoding: 9},
	"r10": {Name: "r10", Size: 64, Encoding: 10},
	"r11": {Name: "r11", Size: 64, Encoding: 11},
	"r12": {Name: "r12", Size: 64, Encoding: 12},
	"r13": {Name: "r13", Size: 64, Encoding: 13},
	"r14": {Name: "r14", Size: 64, Encoding: 14},
	"r15": {Name: "r15", Size: 64, Encoding: 15},

	// 32-bit registers
	"eax": {Name: "eax", Size: 32, Encoding: 0},
	"ecx": {Name: "ecx", Size: 32, Encoding: 1},
	"edx": {Name: "edx", Size: 32, Encoding: 2},
	"ebx": {Name: "ebx", Size: 32, Encoding: 3},

	// 8-bit registers (low byte)
	"al": {Name: "al", Size: 8, Encoding: 0},
	"cl": {Name: "cl", Size: 8, Encoding: 1},
	"dl": {Name: "dl", Size: 8, Encoding: 2},
	"bl": {Name: "bl", Size: 8, Encoding: 3},

	// AVX-512 ZMM registers (512-bit, 8x float64)
	"zmm0":  {Name: "zmm0", Size: 512, Encoding: 0},
	"zmm1":  {Name: "zmm1", Size: 512, Encoding: 1},
	"zmm2":  {Name: "zmm2", Size: 512, Encoding: 2},
	"zmm3":  {Name: "zmm3", Size: 512, Encoding: 3},
	"zmm4":  {Name: "zmm4", Size: 512, Encoding: 4},
	"zmm5":  {Name: "zmm5", Size: 512, Encoding: 5},
	"zmm6":  {Name: "zmm6", Size: 512, Encoding: 6},
	"zmm7":  {Name: "zmm7", Size: 512, Encoding: 7},
	"zmm8":  {Name: "zmm8", Size: 512, Encoding: 8},
	"zmm9":  {Name: "zmm9", Size: 512, Encoding: 9},
	"zmm10": {Name: "zmm10", Size: 512, Encoding: 10},
	"zmm11": {Name: "zmm11", Size: 512, Encoding: 11},
	"zmm12": {Name: "zmm12", Size: 512, Encoding: 12},
	"zmm13": {Name: "zmm13", Size: 512, Encoding: 13},
	"zmm14": {Name: "zmm14", Size: 512, Encoding: 14},
	"zmm15": {Name: "zmm15", Size: 512, Encoding: 15},
	"zmm16": {Name: "zmm16", Size: 512, Encoding: 16},
	"zmm17": {Name: "zmm17", Size: 512, Encoding: 17},
	"zmm18": {Name: "zmm18", Size: 512, Encoding: 18},
	"zmm19": {Name: "zmm19", Size: 512, Encoding: 19},
	"zmm20": {Name: "zmm20", Size: 512, Encoding: 20},
	"zmm21": {Name: "zmm21", Size: 512, Encoding: 21},
	"zmm22": {Name: "zmm22", Size: 512, Encoding: 22},
	"zmm23": {Name: "zmm23", Size: 512, Encoding: 23},
	"zmm24": {Name: "zmm24", Size: 512, Encoding: 24},
	"zmm25": {Name: "zmm25", Size: 512, Encoding: 25},
	"zmm26": {Name: "zmm26", Size: 512, Encoding: 26},
	"zmm27": {Name: "zmm27", Size: 512, Encoding: 27},
	"zmm28": {Name: "zmm28", Size: 512, Encoding: 28},
	"zmm29": {Name: "zmm29", Size: 512, Encoding: 29},
	"zmm30": {Name: "zmm30", Size: 512, Encoding: 30},
	"zmm31": {Name: "zmm31", Size: 512, Encoding: 31},

	// AVX YMM registers (256-bit, 4x float64)
	"ymm0":  {Name: "ymm0", Size: 256, Encoding: 0},
	"ymm1":  {Name: "ymm1", Size: 256, Encoding: 1},
	"ymm2":  {Name: "ymm2", Size: 256, Encoding: 2},
	"ymm3":  {Name: "ymm3", Size: 256, Encoding: 3},
	"ymm4":  {Name: "ymm4", Size: 256, Encoding: 4},
	"ymm5":  {Name: "ymm5", Size: 256, Encoding: 5},
	"ymm6":  {Name: "ymm6", Size: 256, Encoding: 6},
	"ymm7":  {Name: "ymm7", Size: 256, Encoding: 7},
	"ymm8":  {Name: "ymm8", Size: 256, Encoding: 8},
	"ymm9":  {Name: "ymm9", Size: 256, Encoding: 9},
	"ymm10": {Name: "ymm10", Size: 256, Encoding: 10},
	"ymm11": {Name: "ymm11", Size: 256, Encoding: 11},
	"ymm12": {Name: "ymm12", Size: 256, Encoding: 12},
	"ymm13": {Name: "ymm13", Size: 256, Encoding: 13},
	"ymm14": {Name: "ymm14", Size: 256, Encoding: 14},
	"ymm15": {Name: "ymm15", Size: 256, Encoding: 15},

	// SSE XMM registers (128-bit, 2x float64)
	"xmm0":  {Name: "xmm0", Size: 128, Encoding: 0},
	"xmm1":  {Name: "xmm1", Size: 128, Encoding: 1},
	"xmm2":  {Name: "xmm2", Size: 128, Encoding: 2},
	"xmm3":  {Name: "xmm3", Size: 128, Encoding: 3},
	"xmm4":  {Name: "xmm4", Size: 128, Encoding: 4},
	"xmm5":  {Name: "xmm5", Size: 128, Encoding: 5},
	"xmm6":  {Name: "xmm6", Size: 128, Encoding: 6},
	"xmm7":  {Name: "xmm7", Size: 128, Encoding: 7},
	"xmm8":  {Name: "xmm8", Size: 128, Encoding: 8},
	"xmm9":  {Name: "xmm9", Size: 128, Encoding: 9},
	"xmm10": {Name: "xmm10", Size: 128, Encoding: 10},
	"xmm11": {Name: "xmm11", Size: 128, Encoding: 11},
	"xmm12": {Name: "xmm12", Size: 128, Encoding: 12},
	"xmm13": {Name: "xmm13", Size: 128, Encoding: 13},
	"xmm14": {Name: "xmm14", Size: 128, Encoding: 14},
	"xmm15": {Name: "xmm15", Size: 128, Encoding: 15},

	// AVX-512 mask registers (k0-k7)
	"k0": {Name: "k0", Size: 64, Encoding: 0},
	"k1": {Name: "k1", Size: 64, Encoding: 1},
	"k2": {Name: "k2", Size: 64, Encoding: 2},
	"k3": {Name: "k3", Size: 64, Encoding: 3},
	"k4": {Name: "k4", Size: 64, Encoding: 4},
	"k5": {Name: "k5", Size: 64, Encoding: 5},
	"k6": {Name: "k6", Size: 64, Encoding: 6},
	"k7": {Name: "k7", Size: 64, Encoding: 7},
}

// ARM64 registers
var arm64Registers = map[string]Register{
	// 64-bit general purpose registers
	"x0":  {Name: "x0", Size: 64, Encoding: 0},
	"x1":  {Name: "x1", Size: 64, Encoding: 1},
	"x2":  {Name: "x2", Size: 64, Encoding: 2},
	"x3":  {Name: "x3", Size: 64, Encoding: 3},
	"x4":  {Name: "x4", Size: 64, Encoding: 4},
	"x5":  {Name: "x5", Size: 64, Encoding: 5},
	"x6":  {Name: "x6", Size: 64, Encoding: 6},
	"x7":  {Name: "x7", Size: 64, Encoding: 7},
	"x8":  {Name: "x8", Size: 64, Encoding: 8},
	"x9":  {Name: "x9", Size: 64, Encoding: 9},
	"x10": {Name: "x10", Size: 64, Encoding: 10},
	"x11": {Name: "x11", Size: 64, Encoding: 11},
	"x12": {Name: "x12", Size: 64, Encoding: 12},
	"x13": {Name: "x13", Size: 64, Encoding: 13},
	"x14": {Name: "x14", Size: 64, Encoding: 14},
	"x15": {Name: "x15", Size: 64, Encoding: 15},
	"x16": {Name: "x16", Size: 64, Encoding: 16},
	"x17": {Name: "x17", Size: 64, Encoding: 17},
	"x18": {Name: "x18", Size: 64, Encoding: 18},
	"x19": {Name: "x19", Size: 64, Encoding: 19},
	"x20": {Name: "x20", Size: 64, Encoding: 20},
	"x21": {Name: "x21", Size: 64, Encoding: 21},
	"x22": {Name: "x22", Size: 64, Encoding: 22},
	"x23": {Name: "x23", Size: 64, Encoding: 23},
	"x24": {Name: "x24", Size: 64, Encoding: 24},
	"x25": {Name: "x25", Size: 64, Encoding: 25},
	"x26": {Name: "x26", Size: 64, Encoding: 26},
	"x27": {Name: "x27", Size: 64, Encoding: 27},
	"x28": {Name: "x28", Size: 64, Encoding: 28},
	"x29": {Name: "x29", Size: 64, Encoding: 29}, // Frame pointer
	"x30": {Name: "x30", Size: 64, Encoding: 30}, // Link register
	"sp":  {Name: "sp", Size: 64, Encoding: 31},  // Stack pointer

	// 32-bit registers
	"w0": {Name: "w0", Size: 32, Encoding: 0},
	"w1": {Name: "w1", Size: 32, Encoding: 1},
	"w2": {Name: "w2", Size: 32, Encoding: 2},
	"w3": {Name: "w3", Size: 32, Encoding: 3},

	// SVE scalable vector registers (128-2048 bits, implementation defined)
	"z0":  {Name: "z0", Size: 512, Encoding: 0}, // Size shown as 512 for reference
	"z1":  {Name: "z1", Size: 512, Encoding: 1},
	"z2":  {Name: "z2", Size: 512, Encoding: 2},
	"z3":  {Name: "z3", Size: 512, Encoding: 3},
	"z4":  {Name: "z4", Size: 512, Encoding: 4},
	"z5":  {Name: "z5", Size: 512, Encoding: 5},
	"z6":  {Name: "z6", Size: 512, Encoding: 6},
	"z7":  {Name: "z7", Size: 512, Encoding: 7},
	"z8":  {Name: "z8", Size: 512, Encoding: 8},
	"z9":  {Name: "z9", Size: 512, Encoding: 9},
	"z10": {Name: "z10", Size: 512, Encoding: 10},
	"z11": {Name: "z11", Size: 512, Encoding: 11},
	"z12": {Name: "z12", Size: 512, Encoding: 12},
	"z13": {Name: "z13", Size: 512, Encoding: 13},
	"z14": {Name: "z14", Size: 512, Encoding: 14},
	"z15": {Name: "z15", Size: 512, Encoding: 15},
	"z16": {Name: "z16", Size: 512, Encoding: 16},
	"z17": {Name: "z17", Size: 512, Encoding: 17},
	"z18": {Name: "z18", Size: 512, Encoding: 18},
	"z19": {Name: "z19", Size: 512, Encoding: 19},
	"z20": {Name: "z20", Size: 512, Encoding: 20},
	"z21": {Name: "z21", Size: 512, Encoding: 21},
	"z22": {Name: "z22", Size: 512, Encoding: 22},
	"z23": {Name: "z23", Size: 512, Encoding: 23},
	"z24": {Name: "z24", Size: 512, Encoding: 24},
	"z25": {Name: "z25", Size: 512, Encoding: 25},
	"z26": {Name: "z26", Size: 512, Encoding: 26},
	"z27": {Name: "z27", Size: 512, Encoding: 27},
	"z28": {Name: "z28", Size: 512, Encoding: 28},
	"z29": {Name: "z29", Size: 512, Encoding: 29},
	"z30": {Name: "z30", Size: 512, Encoding: 30},
	"z31": {Name: "z31", Size: 512, Encoding: 31},

	// NEON vector registers (128-bit, 2x float64)
	"v0":  {Name: "v0", Size: 128, Encoding: 0},
	"v1":  {Name: "v1", Size: 128, Encoding: 1},
	"v2":  {Name: "v2", Size: 128, Encoding: 2},
	"v3":  {Name: "v3", Size: 128, Encoding: 3},
	"v4":  {Name: "v4", Size: 128, Encoding: 4},
	"v5":  {Name: "v5", Size: 128, Encoding: 5},
	"v6":  {Name: "v6", Size: 128, Encoding: 6},
	"v7":  {Name: "v7", Size: 128, Encoding: 7},
	"v8":  {Name: "v8", Size: 128, Encoding: 8},
	"v9":  {Name: "v9", Size: 128, Encoding: 9},
	"v10": {Name: "v10", Size: 128, Encoding: 10},
	"v11": {Name: "v11", Size: 128, Encoding: 11},
	"v12": {Name: "v12", Size: 128, Encoding: 12},
	"v13": {Name: "v13", Size: 128, Encoding: 13},
	"v14": {Name: "v14", Size: 128, Encoding: 14},
	"v15": {Name: "v15", Size: 128, Encoding: 15},
	"v16": {Name: "v16", Size: 128, Encoding: 16},
	"v17": {Name: "v17", Size: 128, Encoding: 17},
	"v18": {Name: "v18", Size: 128, Encoding: 18},
	"v19": {Name: "v19", Size: 128, Encoding: 19},
	"v20": {Name: "v20", Size: 128, Encoding: 20},
	"v21": {Name: "v21", Size: 128, Encoding: 21},
	"v22": {Name: "v22", Size: 128, Encoding: 22},
	"v23": {Name: "v23", Size: 128, Encoding: 23},
	"v24": {Name: "v24", Size: 128, Encoding: 24},
	"v25": {Name: "v25", Size: 128, Encoding: 25},
	"v26": {Name: "v26", Size: 128, Encoding: 26},
	"v27": {Name: "v27", Size: 128, Encoding: 27},
	"v28": {Name: "v28", Size: 128, Encoding: 28},
	"v29": {Name: "v29", Size: 128, Encoding: 29},
	"v30": {Name: "v30", Size: 128, Encoding: 30},
	"v31": {Name: "v31", Size: 128, Encoding: 31},

	// SVE predicate registers (scalable mask registers)
	"p0":  {Name: "p0", Size: 64, Encoding: 0},
	"p1":  {Name: "p1", Size: 64, Encoding: 1},
	"p2":  {Name: "p2", Size: 64, Encoding: 2},
	"p3":  {Name: "p3", Size: 64, Encoding: 3},
	"p4":  {Name: "p4", Size: 64, Encoding: 4},
	"p5":  {Name: "p5", Size: 64, Encoding: 5},
	"p6":  {Name: "p6", Size: 64, Encoding: 6},
	"p7":  {Name: "p7", Size: 64, Encoding: 7},
	"p8":  {Name: "p8", Size: 64, Encoding: 8},
	"p9":  {Name: "p9", Size: 64, Encoding: 9},
	"p10": {Name: "p10", Size: 64, Encoding: 10},
	"p11": {Name: "p11", Size: 64, Encoding: 11},
	"p12": {Name: "p12", Size: 64, Encoding: 12},
	"p13": {Name: "p13", Size: 64, Encoding: 13},
	"p14": {Name: "p14", Size: 64, Encoding: 14},
	"p15": {Name: "p15", Size: 64, Encoding: 15},
}

// RISC-V registers
var riscvRegisters = map[string]Register{
	// General purpose registers
	"x0":  {Name: "x0", Size: 64, Encoding: 0},   // zero
	"x1":  {Name: "x1", Size: 64, Encoding: 1},   // ra (return address)
	"x2":  {Name: "x2", Size: 64, Encoding: 2},   // sp (stack pointer)
	"x3":  {Name: "x3", Size: 64, Encoding: 3},   // gp (global pointer)
	"x4":  {Name: "x4", Size: 64, Encoding: 4},   // tp (thread pointer)
	"x5":  {Name: "x5", Size: 64, Encoding: 5},   // t0
	"x6":  {Name: "x6", Size: 64, Encoding: 6},   // t1
	"x7":  {Name: "x7", Size: 64, Encoding: 7},   // t2
	"x8":  {Name: "x8", Size: 64, Encoding: 8},   // s0/fp
	"x9":  {Name: "x9", Size: 64, Encoding: 9},   // s1
	"x10": {Name: "x10", Size: 64, Encoding: 10}, // a0
	"x11": {Name: "x11", Size: 64, Encoding: 11}, // a1
	"x12": {Name: "x12", Size: 64, Encoding: 12}, // a2
	"x13": {Name: "x13", Size: 64, Encoding: 13}, // a3
	"x14": {Name: "x14", Size: 64, Encoding: 14}, // a4
	"x15": {Name: "x15", Size: 64, Encoding: 15}, // a5
	"x16": {Name: "x16", Size: 64, Encoding: 16}, // a6
	"x17": {Name: "x17", Size: 64, Encoding: 17}, // a7
	"x18": {Name: "x18", Size: 64, Encoding: 18}, // s2
	"x19": {Name: "x19", Size: 64, Encoding: 19}, // s3
	"x20": {Name: "x20", Size: 64, Encoding: 20}, // s4
	"x21": {Name: "x21", Size: 64, Encoding: 21}, // s5
	"x22": {Name: "x22", Size: 64, Encoding: 22}, // s6
	"x23": {Name: "x23", Size: 64, Encoding: 23}, // s7
	"x24": {Name: "x24", Size: 64, Encoding: 24}, // s8
	"x25": {Name: "x25", Size: 64, Encoding: 25}, // s9
	"x26": {Name: "x26", Size: 64, Encoding: 26}, // s10
	"x27": {Name: "x27", Size: 64, Encoding: 27}, // s11
	"x28": {Name: "x28", Size: 64, Encoding: 28}, // t3
	"x29": {Name: "x29", Size: 64, Encoding: 29}, // t4
	"x30": {Name: "x30", Size: 64, Encoding: 30}, // t5
	"x31": {Name: "x31", Size: 64, Encoding: 31}, // t6

	// ABI names
	"zero": {Name: "zero", Size: 64, Encoding: 0},
	"ra":   {Name: "ra", Size: 64, Encoding: 1},
	"sp":   {Name: "sp", Size: 64, Encoding: 2},
	"gp":   {Name: "gp", Size: 64, Encoding: 3},
	"tp":   {Name: "tp", Size: 64, Encoding: 4},
	"t0":   {Name: "t0", Size: 64, Encoding: 5},
	"t1":   {Name: "t1", Size: 64, Encoding: 6},
	"t2":   {Name: "t2", Size: 64, Encoding: 7},
	"s0":   {Name: "s0", Size: 64, Encoding: 8},
	"fp":   {Name: "fp", Size: 64, Encoding: 8}, // Same as s0
	"s1":   {Name: "s1", Size: 64, Encoding: 9},
	"a0":   {Name: "a0", Size: 64, Encoding: 10},
	"a1":   {Name: "a1", Size: 64, Encoding: 11},
	"a2":   {Name: "a2", Size: 64, Encoding: 12},
	"a3":   {Name: "a3", Size: 64, Encoding: 13},
	"a4":   {Name: "a4", Size: 64, Encoding: 14},
	"a5":   {Name: "a5", Size: 64, Encoding: 15},
	"a6":   {Name: "a6", Size: 64, Encoding: 16},
	"a7":   {Name: "a7", Size: 64, Encoding: 17},

	// RVV vector registers (scalable 128-2048 bits, VLEN implementation defined)
	"v0":  {Name: "v0", Size: 512, Encoding: 0}, // Size shown as 512 for reference
	"v1":  {Name: "v1", Size: 512, Encoding: 1},
	"v2":  {Name: "v2", Size: 512, Encoding: 2},
	"v3":  {Name: "v3", Size: 512, Encoding: 3},
	"v4":  {Name: "v4", Size: 512, Encoding: 4},
	"v5":  {Name: "v5", Size: 512, Encoding: 5},
	"v6":  {Name: "v6", Size: 512, Encoding: 6},
	"v7":  {Name: "v7", Size: 512, Encoding: 7},
	"v8":  {Name: "v8", Size: 512, Encoding: 8},
	"v9":  {Name: "v9", Size: 512, Encoding: 9},
	"v10": {Name: "v10", Size: 512, Encoding: 10},
	"v11": {Name: "v11", Size: 512, Encoding: 11},
	"v12": {Name: "v12", Size: 512, Encoding: 12},
	"v13": {Name: "v13", Size: 512, Encoding: 13},
	"v14": {Name: "v14", Size: 512, Encoding: 14},
	"v15": {Name: "v15", Size: 512, Encoding: 15},
	"v16": {Name: "v16", Size: 512, Encoding: 16},
	"v17": {Name: "v17", Size: 512, Encoding: 17},
	"v18": {Name: "v18", Size: 512, Encoding: 18},
	"v19": {Name: "v19", Size: 512, Encoding: 19},
	"v20": {Name: "v20", Size: 512, Encoding: 20},
	"v21": {Name: "v21", Size: 512, Encoding: 21},
	"v22": {Name: "v22", Size: 512, Encoding: 22},
	"v23": {Name: "v23", Size: 512, Encoding: 23},
	"v24": {Name: "v24", Size: 512, Encoding: 24},
	"v25": {Name: "v25", Size: 512, Encoding: 25},
	"v26": {Name: "v26", Size: 512, Encoding: 26},
	"v27": {Name: "v27", Size: 512, Encoding: 27},
	"v28": {Name: "v28", Size: 512, Encoding: 28},
	"v29": {Name: "v29", Size: 512, Encoding: 29},
	"v30": {Name: "v30", Size: 512, Encoding: 30},
	"v31": {Name: "v31", Size: 512, Encoding: 31},
}

// GetRegister returns register info for the given machine and register name
func GetRegister(machine Arch, regName string) (Register, bool) {
	switch machine {
	case ArchX86_64:
		reg, ok := x86_64Registers[regName]
		return reg, ok
	case ArchARM64:
		reg, ok := arm64Registers[regName]
		return reg, ok
	case ArchRiscv64:
		reg, ok := riscvRegisters[regName]
		return reg, ok
	default:
		return Register{}, false
	}
}

// IsRegister checks if a string is a valid register name for the given machine
func IsRegister(machine Arch, name string) bool {
	_, ok := GetRegister(machine, name)
	return ok
}
