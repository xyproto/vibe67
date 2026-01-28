// Completion: 100% - Platform-specific module complete
package main

import "strconv"

func (eb *ExecutableBuilder) SysWriteARM64(what_data string, what_data_len ...string) {
	eb.Emit("mov x8, " + eb.Lookup("SYS_WRITE"))
	eb.Emit("mov x0, " + eb.Lookup("STDOUT"))
	eb.Emit("mov x1, " + what_data)
	if len(what_data_len) == 0 {
		if c, ok := eb.consts[what_data]; ok {
			eb.Emit("mov x2, " + strconv.Itoa(len(c.value)))
		}
	} else {
		eb.Emit("mov x2, " + what_data_len[0])
	}
	eb.Emit("syscall")
}

func (eb *ExecutableBuilder) SysExitARM64(code ...string) {
	eb.Emit("mov x8, " + eb.Lookup("SYS_EXIT"))
	if len(code) == 0 {
		eb.Emit("mov x0, 0")
	} else {
		eb.Emit("mov x0, " + code[0])
	}
	eb.Emit("syscall")
}
