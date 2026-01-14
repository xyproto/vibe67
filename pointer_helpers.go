// Completion: 100% - Helper module complete
package main

// pointer_helpers.go - Helper functions for pointer/float64 conversions in Vibe67
//
// Vibe67 stores all values as float64 (in XMM registers), including pointers.
// Pointers must be reinterpreted as raw bit patterns, NOT converted numerically.
//
// These helpers ensure consistent and correct pointer handling throughout the compiler.

// EmitPointerToFloat64 converts a pointer in a GPR to float64 bits in an XMM register
// This preserves the raw bit pattern of the pointer.
//
// Parameters:
//   - out: The output code generator
//   - destXmm: Destination XMM register (e.g., "xmm0")
//   - srcGpr: Source general-purpose register containing the pointer (e.g., "rax", "rbx")
//
// Example:
//
//	EmitPointerToFloat64(fc.out, "xmm0", "rax")  // Converts pointer in rax to float64 in xmm0
//
// Implementation: Uses MOVQ instruction to move 64 bits without conversion
func EmitPointerToFloat64(out *Out, destXmm string, srcGpr string) {
	out.MovqRegToXmm(destXmm, srcGpr)
}

// EmitFloat64ToPointer converts a float64 containing pointer bits to a GPR
// This extracts the raw bit pattern, NOT a numeric conversion.
//
// Parameters:
//   - out: The output code generator
//   - destGpr: Destination general-purpose register (e.g., "rax", "rbx")
//   - srcXmm: Source XMM register containing the pointer bits as float64 (e.g., "xmm0")
//
// Example:
//
//	EmitFloat64ToPointer(fc.out, "rbx", "xmm1")  // Extracts pointer bits from xmm1 to rbx
//
// Implementation: Uses MOVQ instruction to move 64 bits without conversion
func EmitFloat64ToPointer(out *Out, destGpr string, srcXmm string) {
	out.MovqXmmToReg(destGpr, srcXmm)
}

// EmitLoadPointerFromStack loads a pointer stored as float64 on the stack
// This is a convenience function combining load + conversion.
//
// Parameters:
//   - out: The output code generator
//   - destGpr: Destination general-purpose register for the pointer (e.g., "rbx")
//   - baseReg: Base register for stack addressing (usually "rbp" or "rsp")
//   - offset: Offset from base register (in bytes)
//
// Example:
//
//	EmitLoadPointerFromStack(fc.out, "rbx", "rbp", -16)  // Load pointer from [rbp-16]
//
// Implementation: Load to XMM first, then convert to GPR
func EmitLoadPointerFromStack(out *Out, destGpr string, baseReg string, offset int) {
	// Use a temporary XMM register (xmm15 is rarely used)
	out.MovMemToXmm("xmm15", baseReg, offset)
	EmitFloat64ToPointer(out, destGpr, "xmm15")
}

// EmitStorePointerToStack stores a pointer from a GPR as float64 on the stack
// This is a convenience function combining conversion + store.
//
// Parameters:
//   - out: The output code generator
//   - srcGpr: Source general-purpose register containing the pointer (e.g., "rax")
//   - baseReg: Base register for stack addressing (usually "rbp" or "rsp")
//   - offset: Offset from base register (in bytes)
//
// Example:
//
//	EmitStorePointerToStack(fc.out, "rax", "rbp", -16)  // Store pointer to [rbp-16]
//
// Implementation: Convert to XMM first, then store
func EmitStorePointerToStack(out *Out, srcGpr string, baseReg string, offset int) {
	// Use a temporary XMM register (xmm15 is rarely used)
	EmitPointerToFloat64(out, "xmm15", srcGpr)
	out.MovXmmToMem("xmm15", baseReg, offset)
}









