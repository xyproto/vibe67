package main

// Migration Example: How to use SafeBuffer instead of raw bytes.Buffer
//
// This file demonstrates the migration pattern for fixing Reset() bugs.
// It shows how the old dangerous pattern can be replaced with SafeBuffer.

/* OLD PATTERN (from codegen.go lines 716-750):

func (fc *FuncCompiler) compileWithSafety() error {
	// ... collect symbols ...

	// DANGER: Reset might lose data or reset at wrong time
	fc.eb.rodata.Reset()

	// Write symbols
	for _, symbol := range symbols {
		fc.eb.WriteRodata(data)
	}

	// ... more code ...

	// DANGER: Another reset in different place
	fc.eb.data.Reset()

	// Write more data
	for _, symbol := range dataSymbols {
		fc.eb.WriteData(data)
	}

	return nil
}

NEW PATTERN (using SafeBuffer):

func (fc *FuncCompiler) compileWithSafety() error {
	// Phase 1: Collect symbols (read-only phase)
	// ... collect symbols ...

	// Phase 2: Write rodata (write phase)
	rodataBuf := NewSafeBuffer("rodata")
	for _, symbol := range symbols {
		rodataBuf.Write(data)
	}
	rodataBuf.Commit()  // Explicitly mark as complete

	// Phase 3: Can now safely read from rodataBuf
	// ... use rodataBuf.Bytes() ...

	// Phase 4: Write data section (separate buffer)
	dataBuf := NewSafeBuffer("data")
	for _, symbol := range dataSymbols {
		dataBuf.Write(data)
	}
	dataBuf.Commit()

	// If recompilation needed, explicit reset:
	if needRecompile {
		rodataBuf.Reset()  // Clear and uncommit
		// ... rewrite rodata ...
		rodataBuf.Commit()
	}

	return nil
}

*/

// Example: Using ScopedBuffer for temporary assembly generation
//
// func emitTemporaryAsm() []byte {
// 	scope := NewScopedBuffer("temp_asm")
// 	defer scope.Complete()  // Ensures cleanup
//
// 	buf := scope.Buffer()
// 	buf.Write([]byte{0x48, 0x89, 0xc3})  // mov rbx, rax
// 	buf.Write([]byte{0xc3})              // ret
//
// 	scope.Complete()
// 	return scope.Bytes()
// }

// Example: ExecutableBuilder migration (future work)
//
// type ExecutableBuilder struct {
// 	// OLD:
// 	// elf, rodata, data, text bytes.Buffer
//
// 	// NEW:
// 	elf    *SafeBuffer
// 	rodata *SafeBuffer
// 	data   *SafeBuffer
// 	text   *SafeBuffer
//
// 	// ... rest of fields ...
// }
//
// func NewExecutableBuilder() *ExecutableBuilder {
// 	return &ExecutableBuilder{
// 		elf:    NewSafeBuffer("elf"),
// 		rodata: NewSafeBuffer("rodata"),
// 		data:   NewSafeBuffer("data"),
// 		text:   NewSafeBuffer("text"),
// 		// ... initialize rest ...
// 	}
// }

// Example: DynamicSections migration (future work)
//
// func (ds *DynamicSections) buildSymbolTable() {
// 	// OLD: ds.dynsym.Reset()
//
// 	// NEW:
// 	if ds.dynsym.IsCommitted() {
// 		ds.dynsym.Reset()
// 	}
// 	ds.dynsym.MustNotBeCommitted()  // Defensive check
//
// 	for _, sym := range ds.dynsymSyms {
// 		binary.Write(ds.dynsym, binary.LittleEndian, sym.name)
// 		// ...
// 	}
//
// 	ds.dynsym.Commit()  // Mark as ready for reading
// }









