// Syscall-based printf implementation
// This generates a complete printf runtime function that uses syscalls instead of libc
package main

import (
	"fmt"
	"os"
	"strconv"
)

// GeneratePrintfSyscallRuntime generates syscall-based printf runtime helpers
// Note: The main printf implementation uses compile-time parsing with inline code emission.
// This function generates helper labels that might be used by some code paths.
func (fc *C67Compiler) GeneratePrintfSyscallRuntime() {
	if fc.eb.target.OS() != OSLinux {
		// On non-Linux systems, we still need libc printf
		return
	}

	// Only generate if printf or print_syscall is actually used
	if !fc.usedFunctions["printf"] && !fc.usedFunctions["_c67_print_syscall"] {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Generating syscall-based printf runtime helpers\n")
	}

	// Generate helper data labels
	fc.eb.Define("_printf_minus", "-")
	fc.eb.Define("_printf_true", "true")
	fc.eb.Define("_printf_false", "false")
	fc.eb.Define("_float_format_str", "%.6f\x00") // Null-terminated for C functions

	// Note: We don't generate runtime printf functions - we use compile-time
	// format string parsing with inline code emission instead.
	// Float formatting is done inline for simplicity and efficiency.
}

// ============================================================================
// UNUSED RUNTIME PRINTF FUNCTIONS (Kept as reference)
// ============================================================================
// The functions below implement a runtime printf with format string parsing.
// They are NOT currently used because the actual implementation uses compile-time
// parsing with inline code emission (see compilePrintfSyscall above).
// These are kept as reference for future work on dynamic printf if needed.
// ============================================================================

// generatePrintfMain generates the main printf dispatcher (UNUSED - see note above)
func (fc *C67Compiler) generatePrintfMain() {
	fc.eb.MarkLabel("_c67_printf_syscall")

	// Prologue
	fc.out.PushReg("rbp")
	fc.out.MovRegToReg("rbp", "rsp")
	fc.out.PushReg("rbx")
	fc.out.PushReg("r12")
	fc.out.PushReg("r13")
	fc.out.PushReg("r14")
	fc.out.PushReg("r15")
	fc.out.SubImmFromReg("rsp", 88)

	// Save integer argument registers (r8, r9)
	fc.out.MovRegToMem("r8", "rbp", -80)
	fc.out.MovRegToMem("r9", "rbp", -88)

	// Save XMM registers (xmm0-xmm2)
	fc.out.MovXmmToMem("xmm0", "rbp", -96)
	fc.out.MovXmmToMem("xmm1", "rbp", -104)
	fc.out.MovXmmToMem("xmm2", "rbp", -112)

	// Initialize registers
	// r12 = format string pointer
	// r13 = arg1, r14 = arg2, r15 = arg3
	// rbx = int arg index
	// r10 = float arg index
	fc.out.MovRegToReg("r12", "rdi")
	fc.out.MovRegToReg("r13", "rsi")
	fc.out.MovRegToReg("r14", "rdx")
	fc.out.MovRegToReg("r15", "rcx")
	fc.out.XorRegWithReg("rbx", "rbx")
	fc.out.XorRegWithReg("r10", "r10")

	// Main loop
	loopStart := fc.eb.text.Len()
	fc.eb.MarkLabel("_printf_main_loop")

	// Load current character: movzx rax, byte [r12]
	fc.out.Emit([]byte{0x49, 0x0f, 0xb6, 0x04, 0x24}) // movzx rax, byte [r12]

	// Test if null terminator
	fc.out.Emit([]byte{0x84, 0xc0}) // test al, al
	doneJumpPos := fc.eb.text.Len()
	fc.out.Emit([]byte{0x0f, 0x84, 0x00, 0x00, 0x00, 0x00}) // je _printf_done

	// Check if '%'
	fc.out.CmpRegToImm("rax", 37)
	formatSpecJumpPos := fc.eb.text.Len()
	fc.out.Emit([]byte{0x0f, 0x84, 0x00, 0x00, 0x00, 0x00}) // je _printf_format_spec

	// Regular character - write it using syscall
	fc.out.MovImmToReg("rdi", "1")      // stdout
	fc.out.LeaMemToReg("rsi", "r12", 0) // current char
	fc.out.MovImmToReg("rdx", "1")      // length
	fc.out.MovImmToReg("rax", "1")      // sys_write
	fc.out.Syscall()

	fc.out.IncReg("r12")
	backToLoopPos1 := fc.eb.text.Len()
	fc.out.Emit([]byte{0xe9, 0x00, 0x00, 0x00, 0x00}) // jmp _printf_main_loop
	fc.patchJumpOffset(backToLoopPos1+1, loopStart)

	// Handle format specifier
	formatSpecStart := fc.eb.text.Len()
	fc.patchJumpOffset(formatSpecJumpPos+2, formatSpecStart)
	fc.eb.MarkLabel("_printf_format_spec")

	fc.out.IncReg("r12")
	fc.out.Emit([]byte{0x49, 0x0f, 0xb6, 0x04, 0x24}) // movzx rax, byte [r12]

	// Check for %% (escaped percent)
	fc.out.CmpRegToImm("rax", 37)
	percentJumpPos := fc.eb.text.Len()
	fc.out.Emit([]byte{0x74, 0x00}) // je _printf_print_percent (short jump)

	// Check format specifiers
	// %d or %i - integer
	fc.out.CmpRegToImm("rax", 100) // 'd'
	intJump1 := fc.eb.text.Len()
	fc.out.Emit([]byte{0x74, 0x00}) // je _printf_handle_int

	fc.out.CmpRegToImm("rax", 105) // 'i'
	intJump2 := fc.eb.text.Len()
	fc.out.Emit([]byte{0x74, 0x00}) // je _printf_handle_int

	fc.out.CmpRegToImm("rax", 118) // 'v' - value (treat as int for now)
	intJump3 := fc.eb.text.Len()
	fc.out.Emit([]byte{0x74, 0x00}) // je _printf_handle_int

	// %s - string
	fc.out.CmpRegToImm("rax", 115) // 's'
	strJumpPos := fc.eb.text.Len()
	fc.out.Emit([]byte{0x74, 0x00}) // je _printf_handle_str

	// %f or %g - float
	fc.out.CmpRegToImm("rax", 102) // 'f'
	floatJump1 := fc.eb.text.Len()
	fc.out.Emit([]byte{0x74, 0x00}) // je _printf_handle_float

	fc.out.CmpRegToImm("rax", 103) // 'g'
	floatJump2 := fc.eb.text.Len()
	fc.out.Emit([]byte{0x74, 0x00}) // je _printf_handle_float

	// %t - boolean (true/false)
	fc.out.CmpRegToImm("rax", 116) // 't'
	boolJumpPos := fc.eb.text.Len()
	fc.out.Emit([]byte{0x74, 0x00}) // je _printf_handle_bool

	// Unknown format - skip
	fc.out.IncReg("r12")
	unknownJumpPos := fc.eb.text.Len()
	fc.out.Emit([]byte{0xe9, 0x00, 0x00, 0x00, 0x00}) // jmp _printf_main_loop
	fc.patchJumpOffset(unknownJumpPos+1, loopStart)

	// Handle escaped %
	percentStart := fc.eb.text.Len()
	fc.patchShortJump(percentJumpPos+1, percentStart)
	fc.out.MovImmToReg("rdi", "1")
	fc.out.LeaMemToReg("rsi", "r12", 0)
	fc.out.MovImmToReg("rdx", "1")
	fc.out.MovImmToReg("rax", "1")
	fc.out.Syscall()
	fc.out.IncReg("r12")
	percentJump := fc.eb.text.Len()
	fc.out.Emit([]byte{0xe9, 0x00, 0x00, 0x00, 0x00})
	fc.patchJumpOffset(percentJump+1, loopStart)

	// Handle integer (%d, %i, %v)
	intStart := fc.eb.text.Len()
	fc.patchShortJump(intJump1+1, intStart)
	fc.patchShortJump(intJump2+1, intStart)
	fc.patchShortJump(intJump3+1, intStart)
	fc.eb.MarkLabel("_printf_handle_int")

	// Get argument based on rbx index
	fc.out.CmpRegToImm("rbx", 0)
	useR13Jump := fc.eb.text.Len()
	fc.out.Emit([]byte{0x74, 0x00}) // je

	fc.out.CmpRegToImm("rbx", 1)
	useR14Jump := fc.eb.text.Len()
	fc.out.Emit([]byte{0x74, 0x00}) // je

	fc.out.CmpRegToImm("rbx", 2)
	useR15Jump := fc.eb.text.Len()
	fc.out.Emit([]byte{0x74, 0x00}) // je

	// No more args
	skipIntJump := fc.eb.text.Len()
	fc.out.Emit([]byte{0xeb, 0x00}) // jmp (short)

	// Load r13
	r13Start := fc.eb.text.Len()
	fc.patchShortJump(useR13Jump+1, r13Start)
	fc.out.MovRegToReg("rax", "r13")
	toIntJump1 := fc.eb.text.Len()
	fc.out.Emit([]byte{0xeb, 0x00})

	// Load r14
	r14Start := fc.eb.text.Len()
	fc.patchShortJump(useR14Jump+1, r14Start)
	fc.out.MovRegToReg("rax", "r14")
	toIntJump2 := fc.eb.text.Len()
	fc.out.Emit([]byte{0xeb, 0x00})

	// Load r15
	r15Start := fc.eb.text.Len()
	fc.patchShortJump(useR15Jump+1, r15Start)
	fc.out.MovRegToReg("rax", "r15")

	// Convert and print integer
	convertIntStart := fc.eb.text.Len()
	fc.patchShortJump(toIntJump1+1, convertIntStart)
	fc.patchShortJump(toIntJump2+1, convertIntStart)

	fc.out.IncReg("rbx")
	fc.out.PushReg("rbx")
	fc.out.PushReg("r10")
	fc.out.PushReg("r12")
	fc.out.CallRelative(0) // Will be patched to call _printf_print_integer
	printIntCallPos := fc.eb.text.Len() - 4
	fc.out.PopReg("r12")
	fc.out.PopReg("r10")
	fc.out.PopReg("rbx")

	afterIntStart := fc.eb.text.Len()
	fc.patchShortJump(skipIntJump+1, afterIntStart)
	fc.out.IncReg("r12")
	afterIntJumpPos := fc.eb.text.Len()
	fc.out.Emit([]byte{0xe9, 0x00, 0x00, 0x00, 0x00})
	fc.patchJumpOffset(afterIntJumpPos+1, loopStart)

	// Handle string (%s)
	strStart := fc.eb.text.Len()
	fc.patchShortJump(strJumpPos+1, strStart)
	fc.eb.MarkLabel("_printf_handle_str")

	// Similar argument loading
	fc.out.CmpRegToImm("rbx", 0)
	strR13Jump := fc.eb.text.Len()
	fc.out.Emit([]byte{0x74, 0x00})

	fc.out.CmpRegToImm("rbx", 1)
	strR14Jump := fc.eb.text.Len()
	fc.out.Emit([]byte{0x74, 0x00})

	fc.out.CmpRegToImm("rbx", 2)
	strR15Jump := fc.eb.text.Len()
	fc.out.Emit([]byte{0x74, 0x00})

	skipStrJump := fc.eb.text.Len()
	fc.out.Emit([]byte{0xeb, 0x00})

	strR13Start := fc.eb.text.Len()
	fc.patchShortJump(strR13Jump+1, strR13Start)
	fc.out.MovRegToReg("rdi", "r13")
	toStrJump1 := fc.eb.text.Len()
	fc.out.Emit([]byte{0xeb, 0x00})

	strR14Start := fc.eb.text.Len()
	fc.patchShortJump(strR14Jump+1, strR14Start)
	fc.out.MovRegToReg("rdi", "r14")
	toStrJump2 := fc.eb.text.Len()
	fc.out.Emit([]byte{0xeb, 0x00})

	strR15Start := fc.eb.text.Len()
	fc.patchShortJump(strR15Jump+1, strR15Start)
	fc.out.MovRegToReg("rdi", "r15")

	convertStrStart := fc.eb.text.Len()
	fc.patchShortJump(toStrJump1+1, convertStrStart)
	fc.patchShortJump(toStrJump2+1, convertStrStart)

	fc.out.IncReg("rbx")
	fc.out.PushReg("rbx")
	fc.out.PushReg("r10")
	fc.out.PushReg("r12")
	fc.out.CallRelative(0) // Will be patched to call _printf_print_string
	printStrCallPos := fc.eb.text.Len() - 4
	fc.out.PopReg("r12")
	fc.out.PopReg("r10")
	fc.out.PopReg("rbx")

	afterStrStart := fc.eb.text.Len()
	fc.patchShortJump(skipStrJump+1, afterStrStart)
	fc.out.IncReg("r12")
	afterStrJumpPos := fc.eb.text.Len()
	fc.out.Emit([]byte{0xe9, 0x00, 0x00, 0x00, 0x00})
	fc.patchJumpOffset(afterStrJumpPos+1, loopStart)

	// Handle float (%f, %g)
	floatStart := fc.eb.text.Len()
	fc.patchShortJump(floatJump1+1, floatStart)
	fc.patchShortJump(floatJump2+1, floatStart)
	fc.eb.MarkLabel("_printf_handle_float")

	// Load float from saved xmm registers
	fc.out.CmpRegToImm("r10", 0)
	floatXmm0Jump := fc.eb.text.Len()
	fc.out.Emit([]byte{0x74, 0x00})

	fc.out.CmpRegToImm("r10", 1)
	floatXmm1Jump := fc.eb.text.Len()
	fc.out.Emit([]byte{0x74, 0x00})

	fc.out.CmpRegToImm("r10", 2)
	floatXmm2Jump := fc.eb.text.Len()
	fc.out.Emit([]byte{0x74, 0x00})

	skipFloatJump := fc.eb.text.Len()
	fc.out.Emit([]byte{0xeb, 0x00})

	floatXmm0Start := fc.eb.text.Len()
	fc.patchShortJump(floatXmm0Jump+1, floatXmm0Start)
	fc.out.MovMemToXmm("xmm0", "rbp", -96)
	toFloatJump1 := fc.eb.text.Len()
	fc.out.Emit([]byte{0xeb, 0x00})

	floatXmm1Start := fc.eb.text.Len()
	fc.patchShortJump(floatXmm1Jump+1, floatXmm1Start)
	fc.out.MovMemToXmm("xmm0", "rbp", -104)
	toFloatJump2 := fc.eb.text.Len()
	fc.out.Emit([]byte{0xeb, 0x00})

	floatXmm2Start := fc.eb.text.Len()
	fc.patchShortJump(floatXmm2Jump+1, floatXmm2Start)
	fc.out.MovMemToXmm("xmm0", "rbp", -112)

	convertFloatStart := fc.eb.text.Len()
	fc.patchShortJump(toFloatJump1+1, convertFloatStart)
	fc.patchShortJump(toFloatJump2+1, convertFloatStart)

	fc.out.IncReg("r10")
	fc.out.PushReg("rbx")
	fc.out.PushReg("r10")
	fc.out.PushReg("r12")
	fc.out.CallRelative(0) // Will be patched to call _printf_print_float
	printFloatCallPos := fc.eb.text.Len() - 4
	fc.out.PopReg("r12")
	fc.out.PopReg("r10")
	fc.out.PopReg("rbx")

	afterFloatStart := fc.eb.text.Len()
	fc.patchShortJump(skipFloatJump+1, afterFloatStart)
	fc.out.IncReg("r12")
	afterFloatJumpPos := fc.eb.text.Len()
	fc.out.Emit([]byte{0xe9, 0x00, 0x00, 0x00, 0x00})
	fc.patchJumpOffset(afterFloatJumpPos+1, loopStart)

	// Handle boolean (%t)
	boolStart := fc.eb.text.Len()
	fc.patchShortJump(boolJumpPos+1, boolStart)
	fc.eb.MarkLabel("_printf_handle_bool")

	// Get argument (similar to int)
	fc.out.CmpRegToImm("rbx", 0)
	boolR13Jump := fc.eb.text.Len()
	fc.out.Emit([]byte{0x74, 0x00})

	fc.out.CmpRegToImm("rbx", 1)
	boolR14Jump := fc.eb.text.Len()
	fc.out.Emit([]byte{0x74, 0x00})

	fc.out.CmpRegToImm("rbx", 2)
	boolR15Jump := fc.eb.text.Len()
	fc.out.Emit([]byte{0x74, 0x00})

	skipBoolJump := fc.eb.text.Len()
	fc.out.Emit([]byte{0xeb, 0x00})

	boolR13Start := fc.eb.text.Len()
	fc.patchShortJump(boolR13Jump+1, boolR13Start)
	fc.out.MovRegToReg("rax", "r13")
	toBoolJump1 := fc.eb.text.Len()
	fc.out.Emit([]byte{0xeb, 0x00})

	boolR14Start := fc.eb.text.Len()
	fc.patchShortJump(boolR14Jump+1, boolR14Start)
	fc.out.MovRegToReg("rax", "r14")
	toBoolJump2 := fc.eb.text.Len()
	fc.out.Emit([]byte{0xeb, 0x00})

	boolR15Start := fc.eb.text.Len()
	fc.patchShortJump(boolR15Jump+1, boolR15Start)
	fc.out.MovRegToReg("rax", "r15")

	convertBoolStart := fc.eb.text.Len()
	fc.patchShortJump(toBoolJump1+1, convertBoolStart)
	fc.patchShortJump(toBoolJump2+1, convertBoolStart)

	fc.out.IncReg("rbx")

	// Test if zero
	fc.out.Emit([]byte{0x48, 0x85, 0xc0}) // test rax, rax
	falseJumpPos := fc.eb.text.Len()
	fc.out.Emit([]byte{0x74, 0x00}) // jz .print_false

	// Print "true"
	fc.out.LeaSymbolToReg("rdi", "_printf_true")
	trueJumpPos := fc.eb.text.Len()
	fc.out.Emit([]byte{0xeb, 0x00}) // jmp

	// Print "false"
	falseStart := fc.eb.text.Len()
	fc.patchShortJump(falseJumpPos+1, falseStart)
	fc.out.LeaSymbolToReg("rdi", "_printf_false")

	afterBoolChoiceStart := fc.eb.text.Len()
	fc.patchShortJump(trueJumpPos+1, afterBoolChoiceStart)

	fc.out.PushReg("rbx")
	fc.out.PushReg("r10")
	fc.out.PushReg("r12")
	fc.out.CallRelative(0) // Will be patched to call _printf_print_string
	printBoolCallPos := fc.eb.text.Len() - 4
	fc.out.PopReg("r12")
	fc.out.PopReg("r10")
	fc.out.PopReg("rbx")

	afterBoolStart := fc.eb.text.Len()
	fc.patchShortJump(skipBoolJump+1, afterBoolStart)
	fc.out.IncReg("r12")
	afterBoolJumpPos := fc.eb.text.Len()
	fc.out.Emit([]byte{0xe9, 0x00, 0x00, 0x00, 0x00})
	fc.patchJumpOffset(afterBoolJumpPos+1, loopStart)

	// Done - cleanup and return
	doneStart := fc.eb.text.Len()
	fc.patchJumpOffset(doneJumpPos+2, doneStart)
	fc.eb.MarkLabel("_printf_done")

	fc.out.AddImmToReg("rsp", 88)
	fc.out.PopReg("r15")
	fc.out.PopReg("r14")
	fc.out.PopReg("r13")
	fc.out.PopReg("r12")
	fc.out.PopReg("rbx")
	fc.out.PopReg("rbp")
	fc.out.Ret()

	// Now generate helper functions and patch the call offsets
	printIntegerStart := fc.eb.text.Len()
	fc.generatePrintInteger()
	fc.patchCallOffset(printIntCallPos, printIntegerStart)

	printStringStart := fc.eb.text.Len()
	fc.generatePrintString()
	fc.patchCallOffset(printStrCallPos, printStringStart)
	fc.patchCallOffset(printBoolCallPos, printStringStart) // Bool uses print_string

	printFloatStart := fc.eb.text.Len()
	fc.generatePrintFloat()
	fc.patchCallOffset(printFloatCallPos, printFloatStart)
}

// Helper to patch short jumps (8-bit offset)
func (fc *C67Compiler) patchShortJump(offsetPos int, targetPos int) {
	bytes := fc.eb.text.Bytes()
	offset := int8(targetPos - (offsetPos + 1))
	bytes[offsetPos] = byte(offset)
}

// Helper to patch long jumps (32-bit offset)
func (fc *C67Compiler) patchJumpOffset(offsetPos int, targetPos int) {
	bytes := fc.eb.text.Bytes()
	offset := int32(targetPos - (offsetPos + 4))
	bytes[offsetPos] = byte(offset)
	bytes[offsetPos+1] = byte(offset >> 8)
	bytes[offsetPos+2] = byte(offset >> 16)
	bytes[offsetPos+3] = byte(offset >> 24)
}

// Helper to patch call offsets
func (fc *C67Compiler) patchCallOffset(offsetPos int, targetPos int) {
	bytes := fc.eb.text.Bytes()
	offset := int32(targetPos - (offsetPos + 4))
	bytes[offsetPos] = byte(offset)
	bytes[offsetPos+1] = byte(offset >> 8)
	bytes[offsetPos+2] = byte(offset >> 16)
	bytes[offsetPos+3] = byte(offset >> 24)
}

// compilePrintfSyscall compiles a printf call using inline syscalls
// This is a simplified approach that parses the format string at compile time
// and emits inline syscalls for each segment
func (fc *C67Compiler) compilePrintfSyscall(call *CallExpr, formatStr *StringExpr) {
	processedFormat := processEscapeSequences(formatStr.Value)

	// Parse format string and emit inline code for each segment
	argIndex := 0
	i := 0
	runes := []rune(processedFormat)

	for i < len(runes) {
		if runes[i] == '%' && i+1 < len(runes) {
			next := runes[i+1]

			if next == '%' {
				// Escaped %% - print single %
				fc.emitSyscallPrintChar('%')
				i += 2
				continue
			}

			// Check for format with precision (like %.15g)
			precision := 6 // default precision
			if next == '.' {
				// Skip precision specifier - find the actual format character
				i += 2 // skip %.
				precisionStart := i
				for i < len(runes) && (runes[i] >= '0' && runes[i] <= '9') {
					i++
				}
				if i >= len(runes) {
					compilerError("printf: incomplete format specifier")
				}
				if i > precisionStart {
					precisionStr := string(runes[precisionStart:i])
					if p, err := strconv.Atoi(precisionStr); err == nil {
						precision = p
					}
				}
				next = runes[i]
				i++ // we'll increment by 1 below, so total advance is correct
			} else {
				i += 2
			}

			// Format specifier - get the corresponding argument
			if argIndex+1 >= len(call.Args) {
				compilerError("printf: not enough arguments for format string")
			}

			arg := call.Args[argIndex+1] // +1 to skip format string
			argIndex++

			switch next {
			case 'd', 'i', 'l', 'u': // Integer/long/unsigned
				fc.compileExpression(arg)
				// xmm0 contains the number - convert to int and print
				fc.out.Cvttsd2si("rax", "xmm0")
				fc.emitSyscallPrintInteger()

			case 'v': // Value (smart format: print integers as int, floats as float)
				fc.compileExpression(arg)
				// xmm0 contains the value
				// Check if it's an exact integer by comparing with rounded value
				fc.out.SubImmFromReg("rsp", 16)
				fc.out.MovXmmToMem("xmm0", "rsp", 0) // Save original
				fc.out.Cvttsd2si("rax", "xmm0")      // Convert to int
				fc.out.Cvtsi2sd("xmm1", "rax")       // Convert back to float
				fc.out.Ucomisd("xmm0", "xmm1")       // Compare
				fc.out.AddImmToReg("rsp", 16)

				// Jump if not equal or unordered (NaN) -> print as float
				floatJump := fc.eb.text.Len()
				fc.out.JumpConditional(JumpNotEqual, 0)
				nanJump := fc.eb.text.Len()
				fc.out.JumpConditional(JumpParity, 0)

				// Equal: print as integer
				fc.emitSyscallPrintInteger()
				intDone := fc.eb.text.Len()
				fc.out.JumpUnconditional(0)

				// Not equal or NaN: print as float
				floatStart := fc.eb.text.Len()
				fc.patchJumpImmediate(floatJump+2, int32(floatStart-(floatJump+6)))
				fc.patchJumpImmediate(nanJump+2, int32(floatStart-(nanJump+6)))
				fc.emitSyscallPrintFloatPrecise(precision)

				// Done
				donePos := fc.eb.text.Len()
				fc.patchJumpImmediate(intDone+1, int32(donePos-(intDone+5)))

			case 's': // String
				fc.compileExpression(arg)
				// xmm0 contains C67 string pointer - print it
				fc.emitSyscallPrintC67String()

			case 'f', 'g': // Float
				fc.compileExpression(arg)
				// xmm0 contains float - use precise formatter
				fc.emitSyscallPrintFloatPrecise(precision)

			case 't', 'b': // Boolean (t=true/false, b=yes/no)
				fc.compileExpression(arg)
				// xmm0 contains value - print "true" or "false"
				if next == 'b' {
					fc.emitSyscallPrintBooleanYesNo()
				} else {
					fc.emitSyscallPrintBoolean()
				}

			default:
				compilerError("printf: unsupported format specifier %%%c", next)
			}
		} else {
			// Regular character or string segment - collect until next %
			start := i
			for i < len(runes) && !(runes[i] == '%' && i+1 < len(runes)) {
				i++
			}

			// Emit this segment as a string literal
			segment := string(runes[start:i])
			if len(segment) > 0 {
				fc.emitSyscallPrintLiteral(segment)
			}
		}
	}
}

// emitSyscallPrintLiteral emits code to print a literal string using syscalls
func (fc *C67Compiler) emitSyscallPrintLiteral(str string) {
	labelName := fmt.Sprintf("printf_lit_%d", fc.stringCounter)
	fc.stringCounter++
	fc.eb.Define(labelName, str)

	fc.out.MovImmToReg("rax", "1") // sys_write
	fc.out.MovImmToReg("rdi", "1") // stdout
	fc.out.LeaSymbolToReg("rsi", labelName)
	fc.out.MovImmToReg("rdx", fmt.Sprintf("%d", len(str)))
	fc.out.Syscall()
}

// emitSyscallPrintChar emits code to print a single character
func (fc *C67Compiler) emitSyscallPrintChar(ch rune) {
	fc.out.SubImmFromReg("rsp", 8)
	fc.out.MovImmToReg("rax", fmt.Sprintf("%d", ch))
	fc.out.MovRegToMem("rax", "rsp", 0)
	fc.out.MovImmToReg("rax", "1") // sys_write
	fc.out.MovImmToReg("rdi", "1") // stdout
	fc.out.MovRegToReg("rsi", "rsp")
	fc.out.MovImmToReg("rdx", "1")
	fc.out.Syscall()
	fc.out.AddImmToReg("rsp", 8)
}

// emitSyscallPrintInteger emits code to print an integer in rax
func (fc *C67Compiler) emitSyscallPrintInteger() {
	fc.out.PushReg("rbx")
	fc.out.PushReg("rcx")
	fc.out.PushReg("rdx")

	// Check if negative
	fc.out.Emit([]byte{0x48, 0x85, 0xc0}) // test rax, rax
	positiveJump := fc.eb.text.Len()
	fc.out.Emit([]byte{0x79, 0x00}) // jns (will patch)

	// Negative: print minus, negate, continue
	fc.out.PushReg("rax")
	fc.out.SubImmFromReg("rsp", 8)
	fc.out.MovImmToReg("rax", "45") // '-'
	fc.out.MovRegToMem("rax", "rsp", 0)
	fc.out.MovImmToReg("rax", "1") // sys_write
	fc.out.MovImmToReg("rdi", "1")
	fc.out.MovRegToReg("rsi", "rsp")
	fc.out.MovImmToReg("rdx", "1")
	fc.out.Syscall()
	fc.out.AddImmToReg("rsp", 8)
	fc.out.PopReg("rax")
	fc.out.NegReg("rax")

	// Patch positive jump to here
	positiveStart := fc.eb.text.Len()
	fc.eb.text.Bytes()[positiveJump+1] = byte(positiveStart - (positiveJump + 2))

	// Convert to string in buffer
	fc.out.SubImmFromReg("rsp", 32)
	fc.out.LeaMemToReg("rbx", "rsp", 32) // Start at rsp+32, will decrement before writing
	fc.out.MovImmToReg("rcx", "10")

	convertStart := fc.eb.text.Len()
	fc.out.XorRegWithReg("rdx", "rdx")
	fc.out.Emit([]byte{0x48, 0xf7, 0xf1}) // div rcx
	fc.out.AddImmToReg("rdx", 48)
	fc.out.DecReg("rbx")
	fc.out.Emit([]byte{0x88, 0x13})       // mov [rbx], dl
	fc.out.Emit([]byte{0x48, 0x85, 0xc0}) // test rax, rax
	convertJump := fc.eb.text.Len()
	fc.out.Emit([]byte{0x75, 0x00}) // jnz (will patch)
	fc.eb.text.Bytes()[convertJump+1] = byte(convertStart - (convertJump + 2))

	// Calculate length: rsp+32 - rbx
	fc.out.LeaMemToReg("rdx", "rsp", 32)
	fc.out.SubRegFromReg("rdx", "rbx")

	// Write using syscall
	fc.out.MovImmToReg("rax", "1")
	fc.out.MovImmToReg("rdi", "1")
	fc.out.MovRegToReg("rsi", "rbx")
	fc.out.Syscall()

	fc.out.AddImmToReg("rsp", 32)
	fc.out.PopReg("rdx")
	fc.out.PopReg("rcx")
	fc.out.PopReg("rbx")
}

// emitSyscallPrintC67String emits code to print a C67 string (in xmm0)
func (fc *C67Compiler) emitSyscallPrintC67String() {
	// Call the existing print syscall helper
	fc.out.MovqXmmToReg("rdi", "xmm0")
	fc.out.CallSymbol("_c67_print_syscall")
}

// emitSyscallPrintBoolean emits code to print true/false based on xmm0
func (fc *C67Compiler) emitSyscallPrintBoolean() {
	// Convert to integer
	fc.out.Cvttsd2si("rax", "xmm0")
	fc.out.Emit([]byte{0x48, 0x85, 0xc0}) // test rax, rax

	falseJump := fc.eb.text.Len()
	fc.out.Emit([]byte{0x74, 0x00}) // je (will patch)

	// Print "true"
	fc.emitSyscallPrintLiteral("true")
	trueJump := fc.eb.text.Len()
	fc.out.Emit([]byte{0xeb, 0x00}) // jmp (will patch)

	// Print "false"
	falseStart := fc.eb.text.Len()
	bytes := fc.eb.text.Bytes()
	bytes[falseJump+1] = byte(falseStart - (falseJump + 2))
	fc.emitSyscallPrintLiteral("false")

	// End
	endStart := fc.eb.text.Len()
	bytes[trueJump+1] = byte(endStart - (trueJump + 2))
}

// emitSyscallPrintBooleanYesNo emits code to print yes/no based on xmm0
func (fc *C67Compiler) emitSyscallPrintBooleanYesNo() {
	// Convert to integer
	fc.out.Cvttsd2si("rax", "xmm0")
	fc.out.Emit([]byte{0x48, 0x85, 0xc0}) // test rax, rax

	falseJump := fc.eb.text.Len()
	fc.out.Emit([]byte{0x74, 0x00}) // je (will patch)

	// Print "yes"
	fc.emitSyscallPrintLiteral("yes")
	trueJump := fc.eb.text.Len()
	fc.out.Emit([]byte{0xeb, 0x00}) // jmp (will patch)

	// Print "no"
	falseStart := fc.eb.text.Len()
	bytes := fc.eb.text.Bytes()
	bytes[falseJump+1] = byte(falseStart - (falseJump + 2))
	fc.emitSyscallPrintLiteral("no")

	// End
	endStart := fc.eb.text.Len()
	bytes[trueJump+1] = byte(endStart - (trueJump + 2))
}

// generatePrintString generates helper to print C-style strings
func (fc *C67Compiler) generatePrintString() {
	fc.eb.MarkLabel("_printf_print_string")

	fc.out.PushReg("rbx")
	fc.out.MovRegToReg("rbx", "rdi")
	fc.out.XorRegWithReg("rdx", "rdx")

	// Calculate strlen
	strlenStart := fc.eb.text.Len()
	fc.out.Emit([]byte{0x80, 0x3c, 0x13, 0x00}) // cmp byte [rbx+rdx], 0
	strlenDoneJump := fc.eb.text.Len()
	fc.out.Emit([]byte{0x74, 0x00}) // je
	fc.out.IncReg("rdx")
	strlenBackJump := fc.eb.text.Len()
	fc.out.Emit([]byte{0xeb, 0x00}) // jmp back
	fc.patchShortJump(strlenBackJump+1, strlenStart)

	strlenDone := fc.eb.text.Len()
	fc.patchShortJump(strlenDoneJump+1, strlenDone)

	// Write using syscall
	fc.out.MovRegToReg("rsi", "rdi")
	fc.out.MovImmToReg("rdi", "1")
	fc.out.MovImmToReg("rax", "1")
	fc.out.Syscall()

	fc.out.PopReg("rbx")
	fc.out.Ret()
}

// generatePrintInteger generates helper to print signed integers
func (fc *C67Compiler) generatePrintInteger() {
	fc.eb.MarkLabel("_printf_print_integer")

	fc.out.PushReg("rbx")
	fc.out.PushReg("rcx")
	fc.out.PushReg("rdx")

	// Check if negative
	fc.out.Emit([]byte{0x48, 0x85, 0xc0}) // test rax, rax
	positiveJump := fc.eb.text.Len()
	fc.out.Emit([]byte{0x79, 0x00}) // jns

	// Negative: print minus and negate
	fc.out.PushReg("rax")
	fc.out.MovImmToReg("rdi", "1")
	fc.out.LeaSymbolToReg("rsi", "_printf_minus")
	fc.out.MovImmToReg("rdx", "1")
	fc.out.MovImmToReg("rax", "1")
	fc.out.Syscall()
	fc.out.PopReg("rax")
	fc.out.NegReg("rax")

	positiveStart := fc.eb.text.Len()
	fc.patchShortJump(positiveJump+1, positiveStart)

	// Allocate buffer on stack
	fc.out.SubImmFromReg("rsp", 32)
	fc.out.LeaMemToReg("rbx", "rsp", 31)
	fc.out.Emit([]byte{0xc6, 0x03, 0x00}) // mov byte [rbx], 0
	fc.out.MovImmToReg("rcx", "10")

	// Convert to decimal
	convertStart := fc.eb.text.Len()
	fc.out.XorRegWithReg("rdx", "rdx")
	fc.out.Emit([]byte{0x48, 0xf7, 0xf1}) // div rcx
	fc.out.AddImmToReg("rdx", 48)         // to ASCII
	fc.out.DecReg("rbx")
	fc.out.Emit([]byte{0x88, 0x13})       // mov [rbx], dl
	fc.out.Emit([]byte{0x48, 0x85, 0xc0}) // test rax, rax
	convertJump := fc.eb.text.Len()
	fc.out.Emit([]byte{0x75, 0x00}) // jnz
	fc.patchShortJump(convertJump+1, convertStart)

	// Print the string
	fc.out.MovRegToReg("rdi", "rbx")
	fc.out.CallRelative(0) // Will be patched
	printStrFromIntCallPos := fc.eb.text.Len() - 4

	fc.out.AddImmToReg("rsp", 32)
	fc.out.PopReg("rdx")
	fc.out.PopReg("rcx")
	fc.out.PopReg("rbx")
	fc.out.Ret()

	// Patch the call to print_string (it's already generated above)
	// Calculate offset back to print_string
	printStrOffset := fc.eb.labels["_printf_print_string"] - (printStrFromIntCallPos + 4)
	bytes := fc.eb.text.Bytes()
	bytes[printStrFromIntCallPos] = byte(printStrOffset)
	bytes[printStrFromIntCallPos+1] = byte(printStrOffset >> 8)
	bytes[printStrFromIntCallPos+2] = byte(printStrOffset >> 16)
	bytes[printStrFromIntCallPos+3] = byte(printStrOffset >> 24)
}

// generatePrintUnsigned generates helper to print unsigned integers
func (fc *C67Compiler) generatePrintUnsigned() {
	fc.eb.MarkLabel("_printf_print_unsigned")

	// Same as print_integer but without negative handling
	fc.out.PushReg("rbx")
	fc.out.PushReg("rcx")
	fc.out.PushReg("rdx")

	fc.out.SubImmFromReg("rsp", 32)
	fc.out.LeaMemToReg("rbx", "rsp", 31)
	fc.out.Emit([]byte{0xc6, 0x03, 0x00}) // mov byte [rbx], 0
	fc.out.MovImmToReg("rcx", "10")

	convertStart := fc.eb.text.Len()
	fc.out.XorRegWithReg("rdx", "rdx")
	fc.out.Emit([]byte{0x48, 0xf7, 0xf1}) // div rcx
	fc.out.AddImmToReg("rdx", 48)
	fc.out.DecReg("rbx")
	fc.out.Emit([]byte{0x88, 0x13})       // mov [rbx], dl
	fc.out.Emit([]byte{0x48, 0x85, 0xc0}) // test rax, rax
	convertJump := fc.eb.text.Len()
	fc.out.Emit([]byte{0x75, 0x00})
	fc.patchShortJump(convertJump+1, convertStart)

	fc.out.MovRegToReg("rdi", "rbx")
	fc.out.CallRelative(0)
	printStrFromUnsignedCallPos := fc.eb.text.Len() - 4

	fc.out.AddImmToReg("rsp", 32)
	fc.out.PopReg("rdx")
	fc.out.PopReg("rcx")
	fc.out.PopReg("rbx")
	fc.out.Ret()

	// Patch the call
	printStrOffset := fc.eb.labels["_printf_print_string"] - (printStrFromUnsignedCallPos + 4)
	bytes := fc.eb.text.Bytes()
	bytes[printStrFromUnsignedCallPos] = byte(printStrOffset)
	bytes[printStrFromUnsignedCallPos+1] = byte(printStrOffset >> 8)
	bytes[printStrFromUnsignedCallPos+2] = byte(printStrOffset >> 16)
	bytes[printStrFromUnsignedCallPos+3] = byte(printStrOffset >> 24)
}

// generatePrintFloat generates helper to print floats (simplified version)
func (fc *C67Compiler) generatePrintFloat() {
	fc.eb.MarkLabel("_printf_print_float")

	// For now, convert float to integer and print
	// Full float printing is complex - would need decimal conversion
	// This is a simplified version that just prints the integer part
	fc.out.PushReg("rbx")
	fc.out.PushReg("rcx")
	fc.out.PushReg("rdx")

	// Convert float in xmm0 to integer in rax
	fc.out.Cvttsd2si("rax", "xmm0")

	// Check if negative
	fc.out.Emit([]byte{0x48, 0x85, 0xc0}) // test rax, rax
	positiveJump := fc.eb.text.Len()
	fc.out.Emit([]byte{0x79, 0x00}) // jns

	// Negative
	fc.out.PushReg("rax")
	fc.out.MovImmToReg("rdi", "1")
	fc.out.LeaSymbolToReg("rsi", "_printf_minus")
	fc.out.MovImmToReg("rdx", "1")
	fc.out.MovImmToReg("rax", "1")
	fc.out.Syscall()
	fc.out.PopReg("rax")
	fc.out.NegReg("rax")

	positiveStart := fc.eb.text.Len()
	fc.patchShortJump(positiveJump+1, positiveStart)

	// Convert integer part
	fc.out.SubImmFromReg("rsp", 32)
	fc.out.LeaMemToReg("rbx", "rsp", 31)
	fc.out.Emit([]byte{0xc6, 0x03, 0x00})
	fc.out.MovImmToReg("rcx", "10")

	convertStart := fc.eb.text.Len()
	fc.out.XorRegWithReg("rdx", "rdx")
	fc.out.Emit([]byte{0x48, 0xf7, 0xf1})
	fc.out.AddImmToReg("rdx", 48)
	fc.out.DecReg("rbx")
	fc.out.Emit([]byte{0x88, 0x13})
	fc.out.Emit([]byte{0x48, 0x85, 0xc0})
	convertJump := fc.eb.text.Len()
	fc.out.Emit([]byte{0x75, 0x00})
	fc.patchShortJump(convertJump+1, convertStart)

	fc.out.MovRegToReg("rdi", "rbx")
	fc.out.CallRelative(0)
	printStrFromFloatCallPos := fc.eb.text.Len() - 4

	fc.out.AddImmToReg("rsp", 32)
	fc.out.PopReg("rdx")
	fc.out.PopReg("rcx")
	fc.out.PopReg("rbx")
	fc.out.Ret()

	// Patch the call
	printStrOffset := fc.eb.labels["_printf_print_string"] - (printStrFromFloatCallPos + 4)
	bytes := fc.eb.text.Bytes()
	bytes[printStrFromFloatCallPos] = byte(printStrOffset)
	bytes[printStrFromFloatCallPos+1] = byte(printStrOffset >> 8)
	bytes[printStrFromFloatCallPos+2] = byte(printStrOffset >> 16)
	bytes[printStrFromFloatCallPos+3] = byte(printStrOffset >> 24)
}

// emitSyscallPrintFloatPrecise prints a float with 6 decimal places
// Input: xmm0 = float64 value
// FULLY INLINE - zero function calls, direct syscalls only!
func (fc *C67Compiler) emitSyscallPrintFloatPrecise(precision int) {
	// Allocate 160 bytes: 128 for work area + 32 for emitSyscallPrintInteger's stack use
	// This ensures our saved value stays at a consistent offset
	fc.out.SubImmFromReg("rsp", 160)
	if precision < 0 {
		precision = 6
	}
	if precision > 15 {
		precision = 15
	}

	// Save xmm0 at the TOP of our stack frame (offset 152, safe from all modifications)
	fc.out.MovXmmToMem("xmm0", "rsp", 152)

	// ===== Print integer part INLINE (no function calls) =====
	fc.out.MovMemToXmm("xmm0", "rsp", 152)
	fc.out.Emit([]byte{0xf2, 0x48, 0x0f, 0x2c, 0xc0}) // cvttsd2si rax, xmm0

	// Convert integer to string inline
	fc.out.PushReg("rbx")
	fc.out.PushReg("rcx")
	fc.out.PushReg("rdx")

	// Check if negative
	fc.out.Emit([]byte{0x48, 0x85, 0xc0}) // test rax, rax
	positiveJump := fc.eb.text.Len()
	fc.out.Emit([]byte{0x79, 0x00}) // jns (will patch)

	// Negative: print minus, negate
	fc.out.PushReg("rax")
	fc.out.MovImmToReg("r15", "45") // '-'
	fc.out.MovRegToMem("r15", "rsp", 8)
	fc.out.MovImmToReg("rax", "1")
	fc.out.MovImmToReg("rdi", "1")
	fc.out.LeaMemToReg("rsi", "rsp", 8)
	fc.out.MovImmToReg("rdx", "1")
	fc.out.Syscall()
	fc.out.PopReg("rax")
	fc.out.NegReg("rax")

	// Patch positive jump
	positiveStart := fc.eb.text.Len()
	fc.eb.text.Bytes()[positiveJump+1] = byte(positiveStart - (positiveJump + 2))

	// Convert to string
	fc.out.LeaMemToReg("rbx", "rsp", 32)
	fc.out.MovImmToReg("rcx", "10")

	convertStart := fc.eb.text.Len()
	fc.out.XorRegWithReg("rdx", "rdx")
	fc.out.Emit([]byte{0x48, 0xf7, 0xf1}) // div rcx
	fc.out.AddImmToReg("rdx", 48)
	fc.out.DecReg("rbx")
	fc.out.Emit([]byte{0x88, 0x13})       // mov [rbx], dl
	fc.out.Emit([]byte{0x48, 0x85, 0xc0}) // test rax, rax
	convertJump := fc.eb.text.Len()
	fc.out.Emit([]byte{0x75, 0x00}) // jnz (will patch)
	fc.eb.text.Bytes()[convertJump+1] = byte(convertStart - (convertJump + 2))

	// Calculate length
	fc.out.LeaMemToReg("rdx", "rsp", 32)
	fc.out.SubRegFromReg("rdx", "rbx")

	// Write
	fc.out.MovImmToReg("rax", "1")
	fc.out.MovImmToReg("rdi", "1")
	fc.out.MovRegToReg("rsi", "rbx")
	fc.out.Syscall()

	fc.out.PopReg("rdx")
	fc.out.PopReg("rcx")
	fc.out.PopReg("rbx")

	// ===== Print decimal point INLINE =====
	fc.out.MovImmToReg("rax", "46") // '.'
	fc.out.MovRegToMem("rax", "rsp", 0)
	fc.out.MovImmToReg("rax", "1")
	fc.out.MovImmToReg("rdi", "1")
	fc.out.MovRegToReg("rsi", "rsp")
	fc.out.MovImmToReg("rdx", "1")
	fc.out.Syscall()

	// ===== Extract decimal digits - exact working assembly =====
	fc.out.MovMemToXmm("xmm0", "rsp", 152)            // Load saved value
	fc.out.Emit([]byte{0xf2, 0x48, 0x0f, 0x2c, 0xc0}) // cvttsd2si rax, xmm0
	fc.out.Emit([]byte{0xf2, 0x48, 0x0f, 0x2a, 0xc8}) // cvtsi2sd xmm1, rax
	fc.out.MovMemToXmm("xmm0", "rsp", 152)            // Reload (critical!)
	fc.out.Emit([]byte{0xf2, 0x0f, 0x5c, 0xc1})       // subsd xmm0, xmm1

	multiplier := 1
	for i := 0; i < precision; i++ {
		multiplier *= 10
	}
	fc.out.MovImmToReg("rax", fmt.Sprintf("%d", multiplier))
	fc.out.Emit([]byte{0xf2, 0x48, 0x0f, 0x2a, 0xc8}) // cvtsi2sd xmm1, rax
	fc.out.Emit([]byte{0xf2, 0x0f, 0x59, 0xc1})       // mulsd xmm0, xmm1

	// Add 0.5 for proper rounding (store on stack and load)
	fc.out.MovImmToReg("rax", "0x3FE0000000000000") // 0.5 in IEEE 754
	fc.out.MovRegToMem("rax", "rsp", 112)
	fc.out.MovMemToXmm("xmm1", "rsp", 112)
	fc.out.Emit([]byte{0xf2, 0x0f, 0x58, 0xc1}) // addsd xmm0, xmm1

	fc.out.Emit([]byte{0xf2, 0x48, 0x0f, 0x2c, 0xc0}) // cvttsd2si rax, xmm0

	fc.out.MovImmToReg("rcx", "10")

	for i := precision - 1; i >= 0; i-- {
		fc.out.XorRegWithReg("rdx", "rdx")
		fc.out.Emit([]byte{0x48, 0xf7, 0xf1})
		fc.out.AddImmToReg("rdx", 48)
		fc.out.MovByteRegToMem("dl", "rsp", 64+i)
	}

	// ===== Print 6 digits INLINE =====
	fc.out.MovImmToReg("rax", "1")
	fc.out.MovImmToReg("rdi", "1")
	fc.out.LeaMemToReg("rsi", "rsp", 64)
	fc.out.MovImmToReg("rdx", fmt.Sprintf("%d", precision))
	fc.out.Syscall()

	fc.out.AddImmToReg("rsp", 160) // Match the initial allocation
}
