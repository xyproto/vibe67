// Completion: 100% - Utility module complete
package main

import (
	"fmt"
	"os"
)

// Syscall generates a raw syscall instruction for unsafe blocks
func (o *Out) Syscall() {
	if o.backend != nil {
		o.backend.Syscall()
		return
	}
	// Fallback for x86_64 (uses methods in this file)
	switch o.target.Arch() {
	case ArchX86_64:
		o.syscallX86()
	}
}

// x86-64: syscall instruction
func (o *Out) syscallX86() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "syscall: ")
	}
	o.Write(0x0F)
	o.Write(0x05)
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ARM64: svc #0
func (o *Out) syscallARM64() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "svc #0: ")
	}
	// SVC #0: 1101 0100 000 imm16 000 01
	// Encoding: 0xD4000001
	instr := uint32(0xD4000001)
	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// RISC-V: ecall
func (o *Out) syscallRISCV() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "ecall: ")
	}
	// ECALL: 000000000000 00000 000 00000 1110011
	// Encoding: 0x00000073
	instr := uint32(0x00000073)
	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// SysWrite generates a write system call using architecture-specific registers
func (eb *ExecutableBuilder) SysWrite(what_data string, what_data_len ...string) {
	switch eb.target.Arch() {
	case ArchX86_64:
		eb.SysWriteX86_64(what_data, what_data_len...)
	case ArchARM64:
		eb.SysWriteARM64(what_data, what_data_len...)
	case ArchRiscv64:
		eb.SysWriteRiscv64(what_data, what_data_len...)
	}
}

// SysExit generates an exit system call using architecture-specific registers
func (eb *ExecutableBuilder) SysExit(code ...string) {
	switch eb.target.Arch() {
	case ArchX86_64:
		eb.SysExitX86_64(code...)
	case ArchARM64:
		eb.SysExitARM64(code...)
	case ArchRiscv64:
		eb.SysExitRiscv64(code...)
	}
}
