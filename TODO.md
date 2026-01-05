# TODO

## Priority 0: ARM64/macOS Test Failures (Immediate)

### Operators Not Implemented
- [ ] Implement bit test operator (`?b`) for ARM64
- [ ] Implement match guards for ARM64

### C FFI Integration Issues
- [ ] Fix C function calls (sin, cos, strlen, malloc) - currently treated as undefined instead of using C FFI
- [ ] Ensure C library functions are properly resolved through dynamic linking

### Lambda/Block Issues
- [ ] Fix lambda execution - lambdas compile but don't execute correctly (empty output)
- [ ] Debug lambda parameter passing and return values
- [ ] Fix lambda variable capture in ARM64

### Type Conversion & String Handling
- [ ] Implement `str()` function for ARM64 (number to string conversion)
- [ ] Fix f-string compilation for ARM64 (currently hangs due to missing str() function)
- [ ] Ensure string concatenation works in f-string context

## Priority 0: Fixes and verifications (General)

- [ ] Verify import system properly initializes closures across modules.

## Priority 1: Demoscene & Size Optimization (<8KB Goal)

The goal is to enable the creation of competitive 64k intros (Linux/x86_64) using SDL3/RayLib. "Hello World" should be <1KB, not 21KB.

- [ ] **Tiny ELF Writer ("-tiny" flag)**
    - [ ] Implement custom ELF header generation (overlapping headers/segments).
    - [ ] Remove page alignment padding (align=1, disable standard 0x1000 alignment).
    - [ ] Merge `.text`, `.data`, and `.rodata` into a single `RX` or `RWX` segment.
    - [ ] Implement `DT_HASH` usage for symbol resolution (smaller than `DT_GNU_HASH`).
- [ ] **Dead Code Elimination (DCE)**
    - [ ] Implement function-level reachability analysis.
    - [ ] Strip unused global variables and constants.
    - [ ] Aggressively remove unused runtime helper functions (e.g., FMA checks if FMA unused).
- [ ] **Asset Compression**
    - [ ] Finish the built-in decompressor stub (LZ4 or custom simple algorithm).
    - [ ] Allow embedding compressed resources directly into the `.text` segment.
- [ ] **Shader Minification**
    - [ ] Add support for embedding and minifying GLSL strings at compile time.

## Priority 2: Language quality and tooling

- [ ] **Developer Tooling (Critical)**
    - [ ] **Language Server Protocol (LSP)**: Implement a basic LSP for VS Code/Neovim (Go-to-definition, simple completions).
    - [ ] **Debug Info**: Generate DWARF v5 debug information for GDB/LLDB support.
    - [ ] **Formatter**: Implement `c67 fmt` for canonical code style.
- [ ] **Compiler Correctness & Robustness**
    - [ ] **Register Allocation**: Upgrade from simple allocator to Linear Scan or Graph Coloring for denser code.
    - [ ] **Fuzzing**: Set up fuzz testing for the parser to prevent crashes on invalid input.
- [ ] **Performance Proof**
    - [ ] Create a benchmark suite comparing C67 vs C (gcc -O2/-O3) vs Go.
    - [ ] Optimize the `match` compiler to generate jump tables for density/speed.

## Priority 3: Language Features

Refining the "Vibe" into a rigorous specification.

- [ ] **Safety & Types**
    - [ ] Implement `µ` operator semantics for explicit memory ownership/movement.
    - [ ] Add `??` null coalescing operator and `?` optional type suffix.
    - [ ] Implement compile-time division-by-zero checks.
- [ ] **Metaprogramming**
    - [ ] "Comptime" evaluation: Execute pure C67 functions at compile time to generate constants (tables, sin/cos LUTS).
- [ ] **Advanced Pattern Matching**
    - [ ] Tuple destructuring: `(x, y) = point`.
    - [ ] Nested patterns: `[[a, b], c] = list`.

## Priority 4: Platform & Architecture

- [ ] **Linux/ARM64**: Polish the ARM64 backend to parity with x86_64.
- [ ] **Windows/x86_64**: Ensure PE header generation produces valid, small executables (currently focused on ELF).
- [ ] **macOS**: Finish Mach-O support.

## Priority 5: Self-hosting

The ultimate proof of language quality.

- [ ] Write a basic C67 parser in C67.
- [ ] Write the Code Generator in C67.
- [ ] Bootstrap: Use the Go compiler to compile the C67 compiler, then use that to compile itself.

## Priority 6: Nice to have and optimizations

- [ ] Implement Windows decompressor stub with VirtualAlloc.
- [ ] Implement proper import table generation for PE files.
- [ ] Optimize O(n²) string iteration in codegen.
