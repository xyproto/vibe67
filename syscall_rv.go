// Completion: 100% - Platform-specific module complete
package main

import "strconv"

// SysWriteRiscv64 generates a write system call for RISC-V
func (eb *ExecutableBuilder) SysWriteRiscv64(what_data string, what_data_len ...string) {
	eb.Emit("li a7, 64")           // write syscall number for RISC-V
	eb.Emit("li a0, 1")            // stdout file descriptor
	eb.Emit("la a1, " + what_data) // buffer address
	if len(what_data_len) == 0 {
		if c, ok := eb.consts[what_data]; ok {
			eb.Emit("li a2, " + strconv.Itoa(len(c.value)))
		}
	} else {
		eb.Emit("li a2, " + what_data_len[0])
	}
	eb.Emit("ecall")
}

// SysExitRiscv64 generates an exit system call for RISC-V
func (eb *ExecutableBuilder) SysExitRiscv64(code ...string) {
	eb.Emit("li a7, 93") // exit syscall number for RISC-V
	if len(code) == 0 {
		eb.Emit("li a0, 0")
	} else {
		eb.Emit("li a0, " + code[0])
	}
	eb.Emit("ecall")
}









