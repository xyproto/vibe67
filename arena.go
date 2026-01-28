// Arena allocator for Vibe67 runtime
// Provides fast bump allocation with scope-based deallocation
package main

import (
	"fmt"
)

// ArenaScope represents different allocation scopes
type ArenaScope int

const (
	ArenaGlobal   ArenaScope = iota // Program lifetime
	ArenaFrame                      // Per-frame allocation
	ArenaFunction                   // Per-function call
	ArenaBlock                      // arena { ... } block
)

// Arena represents an allocation arena in the generated code
type Arena struct {
	name       string
	scope      ArenaScope
	baseReg    string // Register holding base pointer
	currentReg string // Register holding current pointer
	sizeReg    string // Register holding size
	labelNum   int    // Unique label number
}

// Arena runtime structure (in generated code):
// struct Arena {
//     void* base;      // Start of arena memory
//     void* current;   // Current allocation pointer
//     size_t size;     // Total arena size
//     size_t used;     // Bytes used
// }

// Code generation for arena operations

// generateArenaInit generates code to initialize an arena
// This is called at program start or arena block entry
func (fc *C67Compiler) generateArenaInit(arena *Arena, sizeBytes int) {
	// Allocate arena memory with mmap (Linux) or malloc (Windows/macOS)
	if fc.eb.target.OS() == OSLinux {
		// mmap(NULL, size, PROT_READ|PROT_WRITE, MAP_PRIVATE|MAP_ANONYMOUS, -1, 0)
		fc.out.XorRegWithReg("rdi", "rdi")                      // addr = NULL
		fc.out.MovImmToReg("rsi", fmt.Sprintf("%d", sizeBytes)) // length
		fc.out.MovImmToReg("rdx", "7")                          // PROT_READ|PROT_WRITE|PROT_EXEC = 7
		fc.out.MovImmToReg("r10", "34")                         // MAP_PRIVATE|MAP_ANONYMOUS = 0x22 = 34
		fc.out.MovImmToReg("r8", "-1")                          // fd = -1
		fc.out.XorRegWithReg("r9", "r9")                        // offset = 0
		fc.out.MovImmToReg("rax", "9")                          // sys_mmap = 9
		fc.out.Syscall()
	} else {
		// Fall back to malloc for Windows/macOS
		if !fc.hasCFunction("malloc") {
			fc.importCFunction("malloc", "msvcrt.dll")
		}
		fc.out.MovImmToReg("rdi", fmt.Sprintf("%d", sizeBytes))
		mallocSymbol := "__imp_malloc"
		fc.out.CallSymbol(mallocSymbol)
	}

	// rax now contains the arena base pointer
	// Store in arena structure (we'll use stack for now)
	fc.out.MovRegToMem("rax", "rbp", -16) // arena.base
	fc.out.MovRegToMem("rax", "rbp", -24) // arena.current = base
	fc.out.MovImmToReg("rcx", fmt.Sprintf("%d", sizeBytes))
	fc.out.MovRegToMem("rcx", "rbp", -32) // arena.size
	fc.out.XorRegWithReg("rcx", "rcx")
	fc.out.MovRegToMem("rcx", "rbp", -40) // arena.used = 0
}

// generateArenaAlloc generates code to allocate from current arena
// Returns pointer in rax
func (fc *C67Compiler) generateArenaAlloc(sizeBytes int) {
	// Load current pointer
	fc.out.MovMemToReg("rax", "rbp", -24) // rax = arena.current

	// Allocate by bumping pointer
	fc.out.MovRegToReg("rcx", "rax") // Save allocation pointer
	fc.out.AddImmToReg("rax", int64(sizeBytes))
	fc.out.MovRegToMem("rax", "rbp", -24) // Update arena.current

	// Update used count
	fc.out.MovMemToReg("rdx", "rbp", -40) // Load arena.used
	fc.out.AddImmToReg("rdx", int64(sizeBytes))
	fc.out.MovRegToMem("rdx", "rbp", -40) // Store arena.used

	// Return allocation pointer in rax
	fc.out.MovRegToReg("rax", "rcx")
}

// generateArenaReset generates code to reset arena to initial state
func (fc *C67Compiler) generateArenaReset() {
	// Reset current = base
	fc.out.MovMemToReg("rax", "rbp", -16) // Load arena.base
	fc.out.MovRegToMem("rax", "rbp", -24) // Store to arena.current

	// Reset used = 0
	fc.out.XorRegWithReg("rax", "rax")
	fc.out.MovRegToMem("rax", "rbp", -40)
}

// generateArenaFree generates code to free arena memory
// This is called at program end or arena block exit
func (fc *C67Compiler) generateArenaFree() {
	// Load base pointer
	fc.out.MovMemToReg("rdi", "rbp", -16) // rdi = arena.base

	if fc.eb.target.OS() == OSLinux {
		// munmap(addr, length)
		fc.out.MovMemToReg("rsi", "rbp", -32) // rsi = arena.size
		fc.out.MovImmToReg("rax", "11")       // sys_munmap = 11
		fc.out.Syscall()
	} else {
		// Fall back to free for Windows/macOS
		if !fc.hasCFunction("free") {
			fc.importCFunction("free", "msvcrt.dll")
		}
		freeSymbol := "__imp_free"
		fc.out.CallSymbol(freeSymbol)
	}
}

// Helper functions for calling C runtime

func (fc *C67Compiler) callMalloc() {
	// Import malloc if not already imported
	if !fc.hasCFunction("malloc") {
		fc.importCFunction("malloc", "libc.so.6")
	}

	// Call malloc through PLT
	mallocSymbol := "malloc@plt"
	if fc.eb.target.OS() == OSWindows {
		mallocSymbol = "__imp_malloc"
	}
	fc.out.CallSymbol(mallocSymbol)
}

func (fc *C67Compiler) callRealloc() {
	// Similar to malloc
	if !fc.hasCFunction("realloc") {
		fc.importCFunction("realloc", "libc.so.6")
	}

	reallocSymbol := "realloc@plt"
	if fc.eb.target.OS() == OSWindows {
		reallocSymbol = "__imp_realloc"
	}
	fc.out.CallSymbol(reallocSymbol)
}

func (fc *C67Compiler) callFree() {
	// Import free if not already imported
	if !fc.hasCFunction("free") {
		fc.importCFunction("free", "libc.so.6")
	}

	// Call free through PLT
	freeSymbol := "free@plt"
	if fc.eb.target.OS() == OSWindows {
		freeSymbol = "__imp_free"
	}
	fc.out.CallSymbol(freeSymbol)
}

// Check if C function is already imported
func (fc *C67Compiler) hasCFunction(name string) bool {
	// Check in imported functions
	for _, fn := range fc.importedFunctions {
		if fn == name {
			return true
		}
	}
	return false
}

// Import a C function for calling
func (fc *C67Compiler) importCFunction(name, library string) {
	// Add to dynamic linker if we have one
	if fc.eb.dynlinker != nil {
		fc.eb.dynlinker.ImportFunction(library, name)
	}

	// Track that we've imported it
	fc.importedFunctions = append(fc.importedFunctions, name)
}

// Arena-aware allocation functions

// compileStringLiteral now uses arena allocation
func (fc *C67Compiler) compileStringLiteralWithArena(s string) {
	// Calculate size needed (length + null terminator)
	size := len(s) + 1

	// Allocate from current arena
	fc.generateArenaAlloc(size)

	// rax now points to allocated memory
	// Copy string data
	fc.out.MovRegToReg("rdi", "rax") // Save pointer

	// Store each byte (using standard mov instructions)
	for i, ch := range []byte(s) {
		fc.out.MovImmToReg("rax", fmt.Sprintf("%d", ch))
		fc.out.MovByteRegToMem("rax", "rdi", i)
	}
	// Null terminator
	fc.out.XorRegWithReg("rax", "rax")
	fc.out.MovByteRegToMem("rax", "rdi", len(s))

	// Convert pointer to float64 for Vibe67
	fc.out.Cvtsi2sd("xmm0", "rdi")
}

// compileListLiteral now uses arena allocation
func (fc *C67Compiler) compileListLiteralWithArena(elements []Expression) {
	// Lists in Vibe67 are maps with integer keys
	// Calculate size for map structure (simplified)
	// For now: array of float64 values
	size := len(elements) * 8 // 8 bytes per float64

	// Allocate from current arena
	fc.generateArenaAlloc(size)

	// rax points to array
	fc.out.MovRegToReg("r14", "rax") // Save base pointer

	// Store each element
	for i, elem := range elements {
		fc.compileExpression(elem) // Result in xmm0
		fc.out.MovXmmToMem("xmm0", "r14", i*8)
	}

	// Return pointer as float64
	fc.out.Cvtsi2sd("xmm0", "r14")
}

// compileMapLiteral now uses arena allocation
func (fc *C67Compiler) compileMapLiteralWithArena(keys, values []Expression) {
	// Similar to list, but with key-value pairs
	// For now: simple array of alternating keys and values
	size := len(keys) * 16 // 16 bytes per pair (key + value)

	// Allocate from current arena
	fc.generateArenaAlloc(size)

	// rax points to map data
	fc.out.MovRegToReg("r14", "rax")

	// Store key-value pairs
	for i := range keys {
		// Store key
		fc.compileExpression(keys[i])
		fc.out.MovXmmToMem("xmm0", "r14", i*16)

		// Store value
		fc.compileExpression(values[i])
		fc.out.MovXmmToMem("xmm0", "r14", i*16+8)
	}

	// Return pointer as float64
	fc.out.Cvtsi2sd("xmm0", "r14")
}

// Default arena sizes and growth parameters
const (
	// Initial sizes - generous defaults for typical applications
	DefaultGlobalArenaSize   = 16 * 1024 * 1024 // 16 MB (was 1MB)
	DefaultFrameArenaSize    = 4 * 1024 * 1024  // 4 MB (was 256KB)
	DefaultFunctionArenaSize = 1024 * 1024      // 1 MB (was 64KB)
	DefaultBlockArenaSize    = 512 * 1024       // 512 KB (was 32KB)

	// Growth parameters
	// 1.3x growth is gentler than 2x, wastes less memory
	// Example: 16MB → 20.8MB → 27MB → 35.1MB → 45.6MB → 59.3MB → 77MB → 100MB
	ArenaGrowthNumerator   = 13 // Multiply by 13
	ArenaGrowthDenominator = 10 // Divide by 10 = 1.3x growth

	// Maximum arena size before failing (1GB)
	MaxArenaSize = 1024 * 1024 * 1024
)

// callArenaAlloc generates code to allocate from current arena
// Input: rdi = size to allocate
// Output: rax = pointer to allocated memory
func (fc *C67Compiler) callArenaAlloc() {
	// Mark that this program uses arenas
	fc.usesArenas = true

	// Use the existing vibe67_arena_alloc runtime function
	// It expects: rdi = arena_ptr, rsi = size
	// Returns: rax = allocated pointer

	// Save size to stack first (rdi will be overwritten)
	fc.out.PushReg("rdi")

	// Load arena pointer from meta-arena[currentArena-1]
	// currentArena is 1-based (1 = meta-arena[0], the default arena)
	arenaIndex := fc.currentArena - 1
	offset := arenaIndex * 8

	fc.out.LeaSymbolToReg("rdi", "_vibe67_arena_meta")
	fc.out.MovMemToReg("rdi", "rdi", 0)      // rdi = meta-arena array pointer
	fc.out.MovMemToReg("rdi", "rdi", offset) // rdi = arena struct pointer

	// Restore size to rsi
	fc.out.PopReg("rsi") // rsi = size

	// Call arena allocator (rdi = arena_ptr, rsi = size)
	fc.trackFunctionCall("_vibe67_arena_alloc")
	fc.out.CallSymbol("_vibe67_arena_alloc")

	// Result is in rax
}
