# TODO

## Priority 0: Windows PE - WORKING! ✅🎉

**CURRENT STATUS:** Windows PE compilation fully functional! SDL3 example working end-to-end!

### What Works ✅
1. Minimal programs (`main = { 42 }`) - EXIT CODE 42 ✅
2. Variable arithmetic (`x = 10; y = 32; x + y`) - EXIT CODE 42 ✅  
3. Function calls (`add(20, 22)`) - EXIT CODE 42 ✅
4. **Increment/decrement operators** (`x++`, `x--`) - FULLY WORKING ✅
5. **Loops with `max inf`** - Working! ✅
6. **Simple printf** - `printf("Hello\n")` works ✅
7. **SDL3 full example** - Window, rendering, event loop, auto-quit after 300 frames! ✅
8. **SDL3 constant resolution** - `sdl.SDL_INIT_VIDEO` correctly resolves to 32 ✅
9. **Arena blocks without alloc** - `arena { println("Hi") }` works ✅
10. **alloc() outside arena blocks** - Works! ✅
11. **break @1 for loop exit** - Works! ✅
12. Compilation completes without errors ✅

### Known Remaining Issues 🔧
1. **SDL3 event processing** - SDL_PollEvent returns 0 (no events detected)
   - Window displays correctly
   - Rendering works
   - Exits after 300 frames as expected
   - But mouse/keyboard events aren't being captured
   - May be SDL3 API usage issue, not compiler issue

2. **Arena alloc() inside arena blocks crashes** - `arena { x := alloc(100) }` causes access violation
   - Issue: Calling convention for arena functions on Windows
   - Impact: TestArenaBlock passes, but alloc inside arena blocks fails

3. **String conversion crashes** - `x as string` causes access violation  
   - Likely related to arena system
   - Impact: TestArenaStringAllocation fails

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
