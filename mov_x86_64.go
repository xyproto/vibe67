// Completion: 100% - Instruction implementation complete
package main

// X86_64 Architecture implementation

func (x *X86_64) MovImmediate(w Writer, dest, val string) error {
	// Delegated to unified implementation in mov.go via Out
	// This is kept for the Architecture interface compatibility
	// but actual implementation is now in mov.go
	return nil
}

func (x *X86_64) IsValidRegister(reg string) bool {
	return IsRegister(ArchX86_64, reg)
}

func (x *X86_64) Syscall(w Writer) error {
	w.Write(0x0f) // syscall instruction for x86_64
	w.Write(0x05)
	return nil
}

func (x *X86_64) ELFMachineType() uint16 {
	return 0x3e // AMD x86-64
}

func (x *X86_64) Name() string {
	return "x86_64"
}

// RISC-V Architecture implementation

func (r *Riscv64) MovImmediate(w Writer, dest, val string) error {
	// Delegated to unified implementation in mov.go via Out
	// This is kept for the Architecture interface compatibility
	return nil
}

func (r *Riscv64) IsValidRegister(reg string) bool {
	return IsRegister(ArchRiscv64, reg)
}

func (r *Riscv64) Syscall(w Writer) error {
	w.WriteUnsigned(0x00000073) // ecall instruction
	return nil
}

func (r *Riscv64) ELFMachineType() uint16 {
	return 0xF3 // RISC-V
}

func (r *Riscv64) Name() string {
	return "riscv64"
}
