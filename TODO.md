# TODO

## Priority 0: CRITICAL - Windows PE Code Generation Bug

**BLOCKER:** All Windows executables crash with ILLEGAL_INSTRUCTION at RVA 0x112F

### The Bug
10-byte gap between functions at RVA 0x112D-0x1136 containing:
- 5 bytes: D8 FF FF FF 90 (garbage)
- 5 bytes: F2 48 0F 2C F8 (cvttsd2si rdi, xmm0 - exit code conversion from codegen.go:940)

This exit conversion instruction appears ONLY in the gap (not at actual exit point).
Bug is deterministic - same bytes every compile.

### Investigation Results
PE format is 100% correct:
- Stack frame setup fixed (arena init after prologue)
- Section alignment fixed (restored missing alignTo calls)
- Import tables correct (ExitProcess, malloc, free all mapped)
- Shadow space allocated before all Windows calls
- Runtime helpers generated before cleanup code

### Root Cause
bytes.Buffer doesn't have gaps - something actively writes to wrong position.
Exit code conversion at line 940 appears at file offset 0x532 instead of end of main.

### Fix Strategy
1. Add buffer position logging around codegen.go:940
2. Trace fc.eb.text.Len() before and after exit conversion emit
3. Check if buffer position gets reset/modified between lambda and main code
4. Compare position where we THINK we're writing vs actual file offset
5. Look for duplicate Emit() of same instruction bytes

### Quick Test
Compile with logging:
```go
fmt.Fprintf(os.Stderr, "DEBUG: About to emit exit conversion at pos=%d\n", fc.eb.text.Len())
fc.out.Emit([]byte{0xf2, 0x48, 0x0f, 0x2c, 0xf8})
fmt.Fprintf(os.Stderr, "DEBUG: After emit exit conversion, pos=%d\n", fc.eb.text.Len())
```

If position is around 0x12F (303 bytes), that's where the garbage is.
If position is much higher (1000+), then something else is writing to 0x12F.

---

## Priority 1: Fixes and verifications

- [ ] Fix variable scope tracking in lambda compilation (module-level mutable globals).
- [ ] Verify import system properly initializes closures across modules.

## Priority 2: Demoscene & Size Optimization (<8KB Goal)

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
    - [ ] **Fix Unsafe Bug**: Fix register assignment limitation (`rax <- ptr`) to allow raw memory iteration.
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

## Priority 3: Platform & Architecture

- [ ] **Linux/ARM64**: Polish the ARM64 backend to parity with x86_64.
- [ ] **Windows/x86_64**: Fix code generation gap causing crashes (see Priority 0)
- [ ] **macOS**: Finish Mach-O support.

## Priority 5: Self-hosting

The ultimate proof of language quality.

- [ ] Write a basic C67 parser in C67.
- [ ] Write the Code Generator in C67.
- [ ] Bootstrap: Use the Go compiler to compile the C67 compiler, then use that to compile itself.

## Priority 6: Nice to have and optimizations

- [ ] Implement Windows decompressor stub with VirtualAlloc (LOW PRIORITY - compression is optional).
- [x] Implement print/println for Windows using printf. ✅ COMPLETE
- [ ] Implement proper import table generation for PE files.
- [ ] Optimize O(n²) string iteration in codegen.
