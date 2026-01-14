// Completion: 100% - Instruction implementation complete
package main

func (a *ARM64) MovImmediate(w Writer, dest, val string) error {
	// Delegated to unified implementation in mov.go via Out
	// This is kept for the Architecture interface compatibility
	return nil
}

func (a *ARM64) IsValidRegister(reg string) bool {
	return IsRegister(ArchARM64, reg)
}

func (a *ARM64) Syscall(w Writer) error {
	// svc #0 instruction for aarch64 (little-endian: 0x01 0x00 0x00 0xd4)
	w.Write(0x01)
	w.Write(0x00)
	w.Write(0x00)
	w.Write(0xd4)
	return nil
}

func (a *ARM64) ELFMachineType() uint16 {
	return 0xB7 // ARM64
}

func (a *ARM64) Name() string {
	return "aarch64"
}









