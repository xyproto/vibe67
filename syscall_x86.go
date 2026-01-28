// Completion: 100% - Platform-specific module complete
package main

import "strconv"

func (eb *ExecutableBuilder) SysWriteX86_64(what_data string, what_data_len ...string) {
	eb.Emit("mov rax, " + eb.Lookup("SYS_WRITE"))
	eb.Emit("mov rdi, " + eb.Lookup("STDOUT"))
	eb.Emit("mov rsi, " + what_data)
	if len(what_data_len) == 0 {
		if c, ok := eb.consts[what_data]; ok {
			eb.Emit("mov rdx, " + strconv.Itoa(len(c.value)))
		}
	} else {
		eb.Emit("mov rdx, " + what_data_len[0])
	}
	eb.Emit("syscall")
}

func (eb *ExecutableBuilder) SysExitX86_64(code ...string) {
	eb.Emit("mov rax, " + eb.Lookup("SYS_EXIT"))
	if len(code) == 0 {
		eb.Emit("mov rdi, 0")
	} else {
		eb.Emit("mov rdi, " + code[0])
	}
	eb.Emit("syscall")
}
