# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Vibe67 is a **novel and unique** high-performance systems programming language that compiles directly to native machine code (x86_64, ARM64, RISC-V). It combines functional programming minimalism with C-like performance and features a radical universal type system where everything is `map[uint64]float64`.

**The language is authoritatively defined by:**
- `GRAMMAR.md` - Formal syntax specification
- `LANGUAGESPEC.md` - Complete semantics and behavior
- The implementation itself - lexer, parser, codegen, and tests/examples

**Key Characteristics:**
- Direct machine code generation (no LLVM, no intermediate representation)
- Single-pass compilation from AST to native code
- Minimal binaries: 609 bytes for trivial programs, 2.7KB with println
- No libc dependency on Linux (uses direct syscalls)
- C FFI for interop with system libraries (SDL3, RayLib5, etc.)
- Manual memory management with arena allocators that call the kernel directly

## Design Philosophy and Goals

**Primary Use Cases:**
1. **Game Development** - Produce Steam-ready executables for commercial game releases
2. **Demoscene Productions** - Create 64k intros and demos with minimal binary sizes
3. **Arch Linux Utilities** - Write system tools and utilities (bonus goal)

**Core Principles:**
- **Avoid libc when possible** - Use direct Linux syscalls, only link libc for C FFI when necessary
- **Avoid c.malloc/c.free** - Prefer arena allocators (`alloc` within `arena` blocks)
- **Universal type system** - Everything is `map[uint64]float64` at runtime, use `as` casting only at C FFI boundaries
- **Immutable by default** - Variables declared with `=` are immutable, `:=` for mutable (update with `<-`, `++`, `--`)
- **Small executables** - DCE and optimizations to keep binaries competitive with C/C++
- **Automatic optimizations** - Pure functions memoized automatically (limited memory), FMA instructions when available

**Target Libraries:**
- **SDL3** - Primary graphics/audio/input library for games
- **RayLib5** - Alternative graphics library
- **Standard C library** - Math functions, basic I/O (when needed)

**C Interop Strategy:**
- Use `cstruct` to define C-compatible structures
- Use `as` for type casting at Vibe67/C boundaries
- Use `cnull` for C NULL pointers (note: may need implementation)
- Import C libraries with `import sdl3 as sdl` syntax

**Memory Management:**
- Arena allocators that call Linux kernel directly (no malloc wrapper)
- Nested arenas with automatic cleanup on scope exit
- Use `alloc` within `arena { ... }` blocks for temporary allocations
- Manual memory management without GC pauses

## Build & Test Commands

### Building the Compiler
```bash
# Build the vibe67 compiler itself (written in Go)
go build

# Or using Makefile
make

# Install to system
make install
```

### Running Tests
```bash
# Run all tests (recommended: fast and comprehensive)
go test -v ./...

# Run tests with quick timeout (default in CI)
go test -failfast -timeout 1m ./...

# Run specific test
go test -run TestArithmeticOperations

# Run tests for a specific file pattern
go test -run "TestArm64"
```

### Using the Compiler
```bash
# Compile a Vibe67 program
./vibe67 build examples/hello.v67 -o hello

# Compile and run
./vibe67 run examples/hello.v67

# Cross-compile to Windows
./vibe67 build hello.v67 -o hello.exe

# Verbose mode (shows detailed compilation info)
./vibe67 build hello.v67 -o hello -v

# Multi-file compilation
./vibe67 build file1.v67 file2.v67 -o program
```

## Architecture Overview

### Compilation Pipeline

The compiler follows a single-pass architecture:

```
Source Code (.v67)
  → Lexer (lexer.go)
  → Parser (parser.go)
  → AST (ast.go)
  → Code Generator (codegen.go + backend files)
  → Native Binary (ELF/PE/Mach-O)
```

**No intermediate representation (IR)** - the codegen directly emits machine code from the AST.

### Core Modules

**Frontend (Parsing):**
- `lexer.go` - Tokenizes source code into tokens
- `parser.go` - Recursive descent parser, builds AST
- `ast.go` - AST node definitions (expressions, statements, etc.)
- `cparser.go` - Parses C headers for FFI

**Code Generation:**
- `codegen.go` - Main code generator, orchestrates compilation
- `backend.go` - Code generator interface for multi-architecture support
- Architecture-specific backends:
  - `mov.go`, `add.go`, `sub.go`, `cmp.go`, etc. - x86_64 instruction emitters
  - `arm64_backend.go`, `arm64_codegen.go`, `arm64_instructions.go` - ARM64 support
  - `codegen_riscv_writer.go` - RISC-V support

**Binary Output:**
- `elf.go`, `elf_sections.go`, `elf_complete.go`, `elf_dynamic.go` - ELF generation (Linux)
- `codegen_pe_writer.go` - PE generation (Windows)
- `codegen_macho_writer.go` - Mach-O generation (macOS, incomplete)
- `emit.go` - Binary code emission utilities

**C FFI & Runtime:**
- `cffi.go`, `cffi_manager.go` - C Foreign Function Interface
- `calling_convention.go` - x86_64/ARM64 calling conventions
- `dynlib.go` - Dynamic library loading
- `c67_runtime.go` - Minimal runtime helpers

**Memory Management:**
- `arena.go` - Arena allocator implementation
- `hashmap.go` - HashMap implementation (universal type backing)

**Optimization:**
- `dependency_graph.go` - Dead code elimination (DCE) via reachability
- `compress.go` - Binary compression for small executables
- FMA optimization - Automatic use of fused multiply-add instructions

**CLI & Tooling:**
- `cli.go` - Command-line interface
- `main.go` - Entry point
- `fileident.go` - File extension handling (.v67, .vibe67)
- `filewatcher_*.go` - Hot reload support (Unix/Windows)

### Universal Type System

**This is fundamental:** Vibe67 has ONE runtime type: `map[uint64]float64`. All values ARE this map (not "represented as"):
- Numbers: `42` → `{0: 42.0}`
- Strings: `"Hi"` → `{0: 72.0, 1: 105.0}` (ASCII codes)
- Lists: `[1, 2, 3]` → `{0: 1.0, 1: 2.0, 2: 3.0}`
- Maps: `{x: 10}` → `{hash("x"): 10.0}`
- Empty: `[]` → `{}`

Compile-time type annotations (`: num`, `: str`, `: cptr`, `: cstring`) provide safety without affecting runtime representation. Use these annotations at C FFI boundaries with the `as` operator for casting.

### Critical Syntax Rules

**⚠️ Block Disambiguation (`{...}` blocks)**

When the parser encounters `{`, it determines the block type by examining contents:

1. **Map Literal:** First element contains `:` (before any `=>` or `~>`)
   ```vibe67
   config = { port: 8080, host: "localhost" }
   ```

2. **Match Block:** Contains `=>` or `~>` arrows
   - **Value match:** Expression before `{`, patterns match result
     ```vibe67
     x { 0 => "zero" | 5 => "five" ~> "other" }
     ```
   - **Guard match:** No expression before `{`, `|` at **line start**
     ```vibe67
     { | x == 0 => "zero" | x > 0 => "positive" ~> "negative" }
     ```

3. **Statement Block:** No `:` or arrows, executes sequentially
   ```vibe67
   compute = x -> { temp = x * 2; temp + 10 }
   ```

**⚠️ Shadow Keyword (Required for Shadowing)**

Variables that shadow outer scopes MUST use `shadow` keyword:
```vibe67
PORT = 8080                    // Module level
main = {
    shadow PORT = 9000         // ✓ OK: explicitly shadows
    PORT = 9000                // ✗ ERROR: missing shadow
}
```
- Case-insensitive checks (`x` shadows `X`)
- Prevents accidental shadowing bugs
- Required for function parameters shadowing module vars

**⚠️ Boolean Type System**

Booleans are NOT numbers:
- `yes` and `no` have special representation: `{0: val, 1: marker}`
- `yes == 1.0` is **FALSE** (different types)
- Comparison operators return booleans, NOT 0/1
- Use `as num` to convert: `(x > 5) as num` → `1.0` or `0.0`

**⚠️ Bitwise Operators (All Require `b` Suffix)**

```vibe67
<<b >>b <<<b >>>b    // Shifts and rotates
&b |b ^b !b ~b       // Bitwise logic
?b                   // Bit test (checks if bit is set)
```
- Eliminates ambiguity with logical `and`/`or` and pipe `|` operator
- Always suffix bitwise ops with `b`

### Language Features

**Variables:**
- **Immutable (default):** `x = 42` - Cannot be reassigned
- **Mutable:** `count := 0` - Can be updated with `count <- count + 1`, `count++`, `count--`
- **Type annotations:** `ptr: cptr = c.malloc(64)`, `name: str = "Vibe67"`
- **Shadowing:** Use `shadow` keyword when reusing names from outer scopes

**Functions and Lambdas:**
- Functions use `=` by convention: `add = (a, b) -> a + b`
- Lambdas are first-class: `double = x -> x * 2`
- Arrow omission: `(x, y) { x + y }` when params parenthesized + block body
- Implicit lambda: `main = { println("Running") }` → `main = () -> { println("Running") }`
- Variadic functions: `sum = (first, rest...) -> first + #rest`
- **Pure functions automatically memoized** with limited memory cache
- **Tail-call optimization always on** for match arms and recursive calls
- Function composition: `f <> g <> h` means `x -> f(g(h(x)))` (right-associative)

**Pattern Matching:**
- Value match: `sign = x { 0 => "zero" | 5 => "five" ~> "other" }`
- Guard match: `{ | x == 0 => "zero" | x > 0 => "positive" ~> "negative" }`
- `|` at **line start** distinguishes guard from pipe operator
- Default case: `~>` for fallthrough
- Tail-call optimized in match arms

**Loops:**
- Range loop: `@ i in 0..<10 { println(i) }`
- While loop: `@ count > 0 { count <- count - 1 }`
- Infinite loop: `@ { ... }`
- List iteration: `@ item in list { ... }`
- Parallel loop: `|| i in 0..10 { compute(i) }` - Fork-based, each iteration in separate process
- Break/continue: `break @1`, `continue @2` (with loop labels)

**Parallel Programming and ENet Channels:**
- Send to channel: `&8080 <- "message"`
- Receive from channel: `msg <= &8080`
- Parallel map: `results = data || transform`
- Parallel loops use fork() for true process isolation

**Operators:**
- Pipe: `data | transform | filter` (data flow)
- Composition: `f <> g <> h` (function composition, right-associative)
- Bitwise (all with `b` suffix): `<<b`, `>>b`, `&b`, `|b`, `^b`, `!b`, `~b`, `?b`
- List access: `head(xs)` (first element), `tail(xs)` (rest), `#xs` (length)
- Spread: `sum(1, 2, values..., 99)` (variadic expansion)

**Unsafe Blocks:**
- Built-in assembly-like language (similar to Battlestar programming language)
- Write inline machine code: `unsafe { ... }`
- Direct register manipulation and low-level operations
- Use sparingly, prefer high-level Vibe67 when possible

**Error Handling:**
- `or!` operator: `val = risky() or! 42` returns 42 if risky() fails (NaN or null)
- Error blocks: `file = open("data.txt") or! { println("Failed"); ret 1 }`
- Result types encode errors as NaN/null
- Error field access: `result.error` to check error state
- Error propagation: Functions return result types, callers use `or!` to handle

**Import/Export System:**
- Import syntax: `import module as alias`, `import github.com/user/repo@v1.0.0`
- Export modes:
  - `export *` - Global namespace (no prefix needed)
  - `export func1 func2` - Selective (prefix required to access)
  - No export - All available (prefix required)
- Import resolution: libraries → git → local paths
- Git imports support version specifiers: `@v1.0.0`, `@latest`, `@main`

**Program Execution:**
- Three entry modes:
  1. `main` function: `main = { ... }` or `main = (args) -> { ... }`
  2. `main` variable: `main = 42` (exit code)
  3. Top-level code: Executed directly, last expression is exit code
- Mixed mode: Top-level + main function requires explicit `main()` call

**Memory and Resources:**
- Arena blocks: `arena { data = alloc(1024); ... }` - auto-freed on exit
- Nested arenas supported with automatic cleanup
- Defer for cleanup: `defer c.free(ptr)` - LIFO execution order
- Direct kernel syscalls for arena allocation (no libc dependency)
- Use arenas instead of malloc/free when possible

### Common Pitfalls and Important Restrictions

**⚠️ Immutability:**
- Variables declared with `=` CANNOT be reassigned
  ```vibe67
  x = 42
  x = 43        // ✗ ERROR: cannot update immutable variable
  x := 42       // ✓ OK: mutable variable
  x <- 43       // ✓ OK: update mutable variable
  ```
- Cannot modify immutable list/map contents
- Use `:=` for variables that need to change

**⚠️ Boolean vs Number:**
- `yes` ≠ `1.0` and `no` ≠ `0.0` (different types!)
- Comparisons return booleans, not numbers
- Convert with `as num`: `(x > 5) as num` → `1.0` or `0.0`
- Default function returns are numbers (`1.0`), not booleans

**⚠️ Guard vs Pipe Operator:**
- `|` at **line start** = guard in match block
- `|` elsewhere = pipe operator for data flow
  ```vibe67
  // Guard match
  { | x > 0 => "pos" ~> "neg" }
  
  // Pipe operator
  result = data | transform | filter
  ```

**⚠️ Multiple Assignment:**
- Unpacking semantics: missing elements = `0`, extras ignored
  ```vibe67
  [a, b, c] = [1, 2]        // a=1, b=2, c=0
  [x, y] = [1, 2, 3, 4]     // x=1, y=2 (3,4 ignored)
  ```

**⚠️ Type Annotations:**
- Are metadata only, don't change runtime representation
- All values are always `map[uint64]float64`
- Foreign types (`cptr`, `cstring`) only for FFI boundaries
- No runtime type checking or enforcement

### Operator Precedence (Highest to Lowest)

1. Function call, field access: `f(x)`, `obj.field`, `arr[i]`
2. Unary: `+`, `-`, `not`, `!b`, `~b`, `#` (length)
3. Exponentiation: `**`
4. Multiplication/Division: `*`, `/`, `%`, `%%`
5. Addition/Subtraction: `+`, `-`
6. Bitwise shifts: `<<b`, `>>b`, `<<<b`, `>>>b`
7. Bitwise AND: `&b`
8. Bitwise XOR: `^b`
9. Bitwise OR: `|b`
10. Comparisons: `==`, `!=`, `<`, `<=`, `>`, `>=`
11. Logical AND: `and`
12. Logical OR: `or`
13. Pipe: `|` (data flow)
14. Range: `..`, `..<`
15. Composition: `<>` (function composition)
16. Update: `<-` (mutable assignment)
17. Assignment: `=`, `:=`

**C FFI and cstruct:**
- Define C-compatible structures: `cstruct Point { x: f32, y: f32 }`
- Field access with `as` casting: `cstruct Point { x as float64, y as float64 }`
- Import C libraries: `import sdl3 as sdl` (auto-detects headers with pkg-config on Linux)
- DWARF-based header parsing for automatic C bindings
- Cast at boundaries: `ptr as cptr`, `value as num`, `str as cstring`
- Access C functions: `sdl.SDL_Init(sdl.SDL_INIT_VIDEO)`
- C types for FFI: `cstring`, `cptr`, `cint`, `clong`, `cfloat`, `cdouble`, `cbool`, `cvoid`
- C NULL handling: Use `cnull` (may need implementation - check current status)
- Field access: Use `.` for cstruct fields
- Memory layout: cstruct follows C ABI for interoperability

### Dead Code Elimination (DCE)

The compiler uses `dependency_graph.go` to track function calls and eliminate unused code:
- Starts from `_start` (entry point)
- Builds call graph including lambdas and nested functions
- Only emits reachable functions
- Runtime helpers (`_vibe67_string_concat`, `_vibe67_printf`, etc.) are guarded by usage flags

**Important for minimal binaries:** Programs without `println`/`printf` avoid the ~10KB syscall runtime.

### Multi-Architecture Support

The compiler has a backend abstraction (`CodeGenerator` interface in `backend.go`):
- **x86_64** - Primary target, fully featured, uses legacy instruction emitters
- **ARM64** - Secondary target, functional but has known bugs (see TODO.md)
- **RISC-V** - Experimental, basic functionality implemented

When modifying codegen, check if changes need to propagate to all backends.

## Coding Style and Conventions

When working on the Vibe67 compiler:

**Documentation:**
- Do NOT add comments or documentation unless truly helpful for future developers
- Code should be self-explanatory where possible
- Follow existing project style and the spirit of Vibe67
- The language semantics are in LANGUAGESPEC.md, not in code comments

**Compiler Development:**
- Remember Vibe67 is novel and unique - don't try to make it like other languages
- Universal type system is fundamental - all values are `map[uint64]float64`
- Prefer arenas over malloc - avoid linking libc unless necessary for C FFI
- Keep binary sizes small - check DCE impact when adding features
- Test on multiple architectures when modifying codegen

**Testing:**
- Add tests for all new features
- Use existing `*_test.go` patterns
- Include examples in `examples/` directory for complex features
- Test C FFI integration with SDL3 and RayLib5 when relevant

## Common Development Tasks

### Adding a New Built-in Function

1. Add function signature to `parser.go` (in built-in function list)
2. Implement code generation in `codegen.go` (search for similar functions like `println`, `len`, etc.)
3. Add tests in appropriate `*_test.go` file
4. If it requires a runtime helper, add DCE guard (see `_vibe67_printf` example in `printf_syscall.go`)

### Adding a New Language Feature

1. Update `GRAMMAR.md` with syntax
2. Update `LANGUAGESPEC.md` with semantics
3. Add token(s) to `lexer.go` if needed
4. Implement parsing in `parser.go`
5. Add AST node to `ast.go` if needed
6. Implement code generation in `codegen.go` and architecture backends
7. Add comprehensive tests

### Fixing Binary Size Regressions

Check these areas:
- Ensure DCE is working: `depGraph.MarkUsed()` calls in `codegen.go`
- Verify runtime helpers have DCE guards: look for `if fc.usesX` conditions
- Check ELF segment merging: static vs dynamic linking decisions
- Look at TODO.md Priority 0 for ongoing binary size work

### Debugging Compilation Issues

**Use these tools liberally - they are your friends:**

1. **gdb** - Step through generated code, inspect registers, find crashes
   ```bash
   ./vibe67 build examples/gdb_test.v67 -o test
   gdb ./test
   # (gdb) break _start
   # (gdb) run
   # (gdb) info registers
   # (gdb) x/10i $rip
   ```

2. **objdump** - Disassemble binaries to see what code was actually generated
   ```bash
   objdump -d output_binary
   objdump -d output_binary | less
   objdump -M intel -d output_binary  # Intel syntax
   ```

3. **ndisasm** - Alternative disassembler, useful for raw binary analysis
   ```bash
   ndisasm -b 64 output_binary
   ndisasm -b 64 output_binary | grep -A5 "function_name"
   ```

4. **Compiler verbose mode** - See what the compiler is doing
   ```bash
   ./vibe67 build file.v67 -o output -v
   ```

5. **Bad address detector** - Check `bad_address_detector.go` for common issues (0xdeadbeef, etc.)

**Love these tools.** When something doesn't work:
- Disassemble with objdump/ndisasm to see the actual machine code
- Run in gdb to see what's happening at runtime
- Compare working vs broken code side-by-side
- Check register values, stack layout, instruction sequences
- These tools reveal the truth about what the compiler generated

### Platform-Specific Considerations

**Linux:**
- Uses direct syscalls (no libc) when possible
- Dynamic linking only when C FFI used
- Supports ELF with single LOAD segment for minimal size

**Windows:**
- Always uses dynamic linking (MSVCRT, KERNEL32)
- PE generation has some optimization issues (see TODO.md)
- Test with examples that compile to .exe

**macOS:**
- Mach-O writer incomplete (in progress)
- ARM64 target exists but needs testing

## Important Files to Read

- `LANGUAGESPEC.md` - Complete language semantics (v1.5.0)
- `GRAMMAR.md` - Formal grammar specification
- `README.md` - User-facing documentation with examples
- `TODO.md` - Current priorities and known issues (very detailed!)
- `codegen.go` lines 1-100 - Core compiler structure and documentation

## Testing Philosophy

- Tests are comprehensive: `*_test.go` files cover arithmetic, control flow, FFI, etc.
- Tests compile and execute actual Vibe67 programs
- Use `compileAndRun()` helper for integration tests
- ARM64 tests currently disabled in CI (see `.github/workflows/ci.yml`)

## Current Status & Priorities (from TODO.md)

**Production-Ready:**
- x86_64 Linux codegen (stable)
- Core language features (complete)
- Minimal binaries (609 bytes achieved for trivial programs)

**In Progress:**
- ARM64 backend fixes (lambda execution, C FFI resolution)
- Windows PE optimization
- macOS Mach-O support

**High Priority:**
- Binary size optimization for realistic programs (<10KB target)
- Cross-platform validation (Windows, macOS)
- Standard library (math, collections, string utilities)

See TODO.md for detailed roadmap and specific tasks.

## Problem-Solving Philosophy

**You are a world-class developer.** Approach problems with confidence and systematic thinking:

**Never Estimate Time or Complexity:**
- Don't say "this will take X hours" or "this is too complex"
- Focus on the path forward, not the perceived difficulty
- Break down problems systematically, not based on time estimates

**Be Bold in the Face of Complexity:**
- Complex problems may require complex solutions - that's acceptable
- Simple problems should have simple solutions - don't over-engineer
- Match solution complexity to problem complexity
- Don't shy away from challenging implementation work

**Use Proven Problem-Solving Techniques:**
- Apply Polya's "How to Solve It?" principles:
  1. **Understand the problem** - Read LANGUAGESPEC.md, GRAMMAR.md, examine tests
  2. **Devise a plan** - Look at similar implementations in the codebase
  3. **Carry out the plan** - Implement systematically with testing
  4. **Look back** - Verify correctness, check DCE impact, test across architectures
- Draw from computer science fundamentals (algorithms, data structures, compilation theory)
- Study existing patterns in the codebase before inventing new approaches

**When to Re-Plan:**
- **Progress/path/plan is okay** - Even if it takes time, forward movement is good
- **Going in circles is not okay** - If stuck repeating the same attempts, stop
- **Step back and re-plan** when needed - Take a small step back, reassess
- **Step up for perspective** - View the problem from a higher level when stuck
- **Change approaches** - If one technique isn't working, try a different angle

**Systematic Progress:**
- It's okay if solutions take time, as long as there is measurable progress
- Having a clear path forward is more important than speed
- When blocked, analyze why, then either:
  - Break the problem into smaller pieces
  - Look at the problem from a different angle
  - Study similar solved problems in the codebase
  - Re-read the relevant specification sections

**Love Your Tools:**
- **gdb** is your best friend for runtime debugging
- **objdump** shows you exactly what code was generated
- **ndisasm** reveals the truth about binary output
- Use these tools liberally - they solve problems faster than guessing
- When compiler output doesn't match expectations, disassemble and inspect
- Compare working code vs broken code with these tools
- The machine code doesn't lie - let it teach you

## Key Reminders for Development

When working on Vibe67, always remember:

1. **Vibe67 is novel and unique** - Don't try to make it conform to other language patterns
2. **Everything is `map[uint64]float64`** - This is not implementation detail, this IS Vibe67
3. **Avoid libc** - Use direct syscalls when possible, arenas instead of malloc
4. **Keep binaries small** - DCE is critical, check impact of every new feature
5. **Target: Games + Demos** - SDL3/RayLib5 integration is primary goal, Steam-ready executables
6. **Pure functions memoized** - Automatic optimization with limited memory cache
7. **Immutable by default** - `=` for immutable, `:=` for mutable, `<-` for updates
8. **No IR, direct codegen** - AST → machine code in one pass
9. **Test comprehensively** - Every feature needs tests, examples for complex features
10. **Follow existing style** - Minimal comments, self-documenting code, defer to LANGUAGESPEC.md

The compiler is an expert tool for experts who understand assembly, machine code, parsers, and low-level systems programming. Code with that mindset.
