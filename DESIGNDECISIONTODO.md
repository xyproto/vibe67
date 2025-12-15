# Design Decisions TODO

This document tracks missing, conflicting, or unclear design decisions that need to be resolved in C67.

## Type System & Semantics

### 1. String Encoding Consistency
**Issue:** Multiple representations for strings exist in the codebase
- Strings as maps: `{0: 72.0, 1: 101.0, ...}` (documented in GRAMMAR.md)
- String arena allocation (see arena.go)
- C string conversion (`c67_string_to_cstr`)
- String literals in `.rodata`

**Decision:**
- [x] `.bytes` and `.runes` return new maps (explicit copy, predictable ownership)
- [x] Strings are in `.rodata` when immutable literal, arena when constructed at runtime
- [x] Immutable strings (=) are readonly, mutable (:=) can be modified (copy-on-write from .rodata)
- [x] String concatenation always creates new arena allocation (functional style)

### 2. Number Type Precision
**Issue:** Float64 is used for all numbers, but precision/overflow behavior unclear
- codegen.go has `TODO: Add explicit cvtsd2ss/cvtss2sd if needed for precision`
- No specification for when/how precision is lost
- Overflow behavior undefined (should it wrap? error? saturate?)

**Decision:**
- [x] Overflow wraps silently (C semantics, predictable, no runtime checks)
- [x] No precision loss warnings (trust the programmer, minimal philosophy)
- [x] Bitwise operations truncate float64 to int64 implicitly (convenient, matches C cast)
- [x] Document that all numbers are float64, precision limits are user responsibility

### 3. Map vs List Distinction
**Issue:** Both are `map[uint64]float64`, but behavior differs
- Lists have sequential keys 0, 1, 2, ...
- Maps have hashed keys
- What happens when mixing operations?

**Decision:**
- [x] Lists and maps are the same type - maps with sequential keys are lists
- [x] Appending to map with non-sequential keys is allowed (just adds key-value pair)
- [x] `#` returns highest key + 1 for maps (works for both lists and sparse maps)
- [x] Iteration order undefined for non-sequential keys (implementation dependent)
- [x] This is a feature, not a bug - simplicity over distinction

### 4. Closure Capture Semantics
**Issue:** codegen.go says "TODO: Full implementation with proper closure generation"
- Closures currently work but implementation incomplete
- Capture by value or reference?
- Mutable vs immutable capture?

**Decision:**
- [x] Capture by reference (matches closures in Swift, Kotlin, allows mutation)
- [x] Captured variables can be mutated (practical for real-world usage)
- [x] Closures capture pointers to arena/stack, no special allocation needed
- [x] No explicit capture lists (minimal syntax, unlike C++ or Rust)

## Memory Management

### 5. Arena vs Stack vs Heap
**Issue:** Multiple allocation strategies without clear rules
- Arena allocation (arena.go)
- Stack allocation (mentions in comments)
- Heap allocation (malloc/free)
- `.rodata` for constants

**Decision:**
- [x] Arena for temporary data within function scope (automatic cleanup)
- [x] Stack for small primitives and local variables (fast, automatic)
- [x] `.rodata` for string/number literals (efficient, shareable)
- [x] Heap (malloc) only via C FFI for explicit long-lived data
- [x] Users don't control allocation - compiler decides (minimal, like Go)

### 6. String Immutability vs Allocation
**Issue:** Strings can be immutable (`=`) or mutable (`:=`), but allocation unclear
- codegen.go: "TODO: Replace with arena allocation (if mutable) or .rodata (if immutable)"
- When does a string move from .rodata to arena?

**Decision:**
- [x] Immutable strings (=) always in .rodata, never copied
- [x] Mutable strings (:=) start in .rodata, copy to arena on first mutation
- [x] Substrings create new map with references (no copy until mutation)
- [x] String slices are just maps with subset of keys (zero-copy view)

### 7. Defer Stack Implementation
**Issue:** GRAMMAR.md documents defer, but implementation unclear
- How is the defer stack stored?
- How does it interact with arena allocation?
- What happens with defer + error returns?

**Decision:**
- [x] Defer stack stored per-function in local stack frame
- [x] `defer` with `ret @` in loops executes on loop exit, not function exit
- [x] Deferred functions can error - error propagates to caller
- [x] No maximum defer depth (limited by stack, like recursion)

## Concurrency & Parallelism

### 8. ENet Channel Semantics
**Issue:** ENet channels documented but implementation details missing
- Synchronous or asynchronous?
- Buffered or unbuffered?
- What happens on full/empty channel?

**Decision:**
- [x] Channels are asynchronous with fixed buffer (like Go with buffer size 64)
- [x] Send blocks when full, receive blocks when empty (backpressure)
- [x] Network failures return error NaN (consistent with error handling)
- [x] Channels can be closed, receive on closed channel returns error

### 9. Fork vs Threads
**Issue:** `||` uses fork(), but some operations might need threads
- Fork is heavy for fine-grained parallelism
- No shared memory with fork
- Windows doesn't have fork

**Decision:**
- [x] Keep `||` with fork() on Unix (simple, isolated, no shared state bugs)
- [x] Windows uses threads with message passing (platform limitation)
- [x] No configurable backend - platform chooses best (minimal, orthogonal)
- [x] Parallel operations communicate via channels only (no shared memory)

### 10. Atomic Operations
**Issue:** atomic.go exists but integration unclear
- When are atomic operations used?
- How do they interact with the map type system?
- Are atomic operations portable across architectures?

**Decision:**
- [x] Atomic operations must be explicit via C FFI (stdatomic.h)
- [x] No operations are atomic by default (performance, simplicity)
- [x] No language-level atomic maps (use channels for sync)
- [x] Lock-free structures via C FFI only (expert territory)

## Error Handling

### 11. Error Code Exhaustion
**Issue:** Error codes are 4 bytes, limited to ~4 billion codes
- Standard codes use ASCII (e.g., "dv0\0")
- What happens when codes collide?
- How to namespace error codes?

**Decision:**
- [x] Error codes use 4-char ASCII strings (human readable, debuggable)
- [x] Namespace with prefix: "io:eof\0", "net:timeout\0" (colon separator)
- [x] No registration required (simple, minimal)
- [x] User documents their error codes (trust the programmer)

### 12. Error vs Panic
**Issue:** No panic mechanism, only errors
- Some errors are unrecoverable (OOM, stack overflow)
- Should there be a panic/abort mechanism?

**Decision:**
- [x] No panic - errors only (consistent, predictable)
- [x] Stack overflow: OS signal handler, program terminates
- [x] OOM: returns error NaN, caller handles or propagates
- [x] No error recovery blocks (use `or!` operator, simple)

### 13. Null vs Error vs Zero
**Issue:** Three different "failure" representations
- Null pointer (0.0 for pointers)
- Error NaN (NaN-boxed error)
- Empty map `{}`

**Decision:**
- [x] Null (0.0) for pointers, Error NaN for failed operations, empty map {} for no data
- [x] `or!` handles error NaN only (focused on error handling)
- [x] Intentional zero is just zero - no special marker needed
- [x] Empty map `{}` is falsy in conditionals (convenient, like Python)

## Control Flow

### 14. Match Arm Return Values
**Issue:** Match arms can return values or jump labels
- `x { 0 => "zero" ~> "other" }` (values)
- `x { 0 => 1 ~> 2 }` (jump labels? or integers?)

**Decision:**
- [x] Match arms return values only (expressions, not statements)
- [x] No jump labels in match (use separate `match` then `goto` if needed)
- [x] All match arms must return same type (type safety)
- [x] Match is always an expression (functional style, like Rust/Swift)

### 15. Loop Max Inference
**Issue:** `max` keyword required for some loops, optional for others
- When is `max` required?
- Can it be inferred?
- What's the default if omitted?

**Decision:**
- [x] `max` required when bounds cannot be proven at compile time
- [x] Compiler infers max for simple cases (literal bounds, constants)
- [x] Warning when max is inferred (helps avoid accidents)
- [x] Default max is 1,000,000 if truly unknown (safety net)

### 16. Tail Call Optimization Guarantees
**Issue:** TCO "always on" but edge cases unclear
- What if tail call exceeds stack depth?
- What if tail call has deferred functions?
- Mutual recursion support?

**Decision:**
- [x] TCO guaranteed for direct recursion in tail position
- [x] Mutual recursion NOT guaranteed TCO (complexity, diminishing returns)
- [x] Tail call with defer is NOT optimized (correctness over performance)
- [x] No explicit depth limit (unlimited stack growth via TCO)

## FFI & Interop

### 17. C Struct Packing
**Issue:** cstruct has alignment but packing rules unclear
- `packed` vs `aligned` keywords mentioned
- How do they interact?
- What's the default?

**Decision:**
- [x] Default packing follows C ABI for platform (ensures C compatibility)
- [x] `packed` forces 1-byte alignment (like __attribute__((packed)))
- [x] `aligned(N)` forces N-byte alignment (like __attribute__((aligned)))
- [x] Platform-specific packing in cparser.go (already implemented)

### 18. Variadic C Function Calls
**Issue:** C variadic functions (printf, etc.) have special calling conventions
- Current implementation may not handle correctly
- va_list representation unclear

**Decision:**
- [x] Use libffi for variadic C functions (proper calling convention)
- [x] Type coercion: float64 stays double, int64 promoted as needed
- [x] No C67 variadic functions (keep language simple)
- [x] Type hints via cffi annotations when needed

### 19. C Header Parsing Limitations
**Issue:** DWARF-based C header parsing has limitations
- Macros not visible
- Inline functions not visible
- Complex types may fail

**Decision:**
- [x] Manual FFI declarations allowed via `extern` keyword
- [x] C macros: user defines them as C67 constants (simple, explicit)
- [x] Inline functions: user wraps in C helper file (practical workaround)
- [x] Fallback when DWARF unavailable: require manual declarations

## Optimization

### 20. SIMD Auto-Vectorization Heuristics
**Issue:** New SIMD vectorization added, but heuristics unclear
- When to vectorize vs not?
- AVX2 vs AVX-512 selection criteria?
- How to handle non-multiple-of-4/8 arrays?

**Decision:**
- [x] Cost model: vectorize if loop trip count > 16 and no dependencies
- [x] Vector width: AVX-512 if available, else AVX2, else SSE (runtime detection)
- [x] Non-aligned: use unaligned loads/stores (performance hit but correct)
- [x] Explicit hints via comments: `// @vectorize` or `// @novectorize`

### 21. Peephole Optimization Priority
**Issue:** Multiple peephole patterns, priority unclear
- Absorption laws added
- De Morgan's laws added (with short-circuit note)
- Which patterns applied first?

**Decision:**
- [x] Order: constant folding → peephole → vectorization → register allocation
- [x] All optimizations preserve semantics (never change behavior)
- [x] No optimization levels - always optimize (simple, predictable)
- [x] Document each optimization pattern in optimizer.go

### 22. Constant Folding Limits
**Issue:** How much constant folding to do?
- Compile-time evaluation depth?
- Recursive constant folding?
- Should there be limits?

**Decision:**
- [x] Max depth: 1000 levels (prevents infinite compile loops)
- [x] All pure operations can be constant-folded (math, logic, bitwise)
- [x] Error during folding: emit error at compile time (fail fast)
- [x] No compile-time function execution (complexity, keep simple)

## Platform Support

### 23. Windows Support Completeness
**Issue:** Windows support mentioned but incomplete
- PE format support (pe.go has TODOs)
- No fork() on Windows
- DLL loading different

**Decision:**
- [x] Windows is Tier 1 platform (PE, DLL, threads for || operator)
- [x] fork() replacement: use CreateProcess with thread pool
- [x] Parallel uses thread pool on Windows (different impl, same semantics)
- [x] PE import table complete (validate with real Windows programs)

### 24. ARM64 vs x86_64 Parity
**Issue:** ARM64 marked as 90% complete
- What's the remaining 10%?
- Are there semantic differences?
- Performance differences?

**Decision:**
- [x] ARM64 is Tier 1 platform (90% complete → 100%)
- [x] Remaining 10%: NEON vectorization, some edge cases
- [x] Semantic parity required (same behavior as x86_64)
- [x] Use NEON when vectorizing on ARM64 (equivalent to SSE/AVX)

### 25. RISC-V Support Level
**Issue:** RISC-V marked as 80% complete
- PLT/GOT generation incomplete (pltgot_rv64.go TODO)
- Testing incomplete
- What's production-ready?

**Decision:**
- [x] RISC-V is Tier 2 (experimental, 80% → aim for 95%)
- [x] Complete PLT/GOT for dynamic linking
- [x] RVV (RISC-V Vector) extension required for vectorization
- [x] Production-ready when passes all tests on real hardware

## Module System

### 26. Import Resolution Priority
**Issue:** Import priority: libraries > git repos > local dirs
- What if names collide?
- How to force specific resolution?
- Cache invalidation strategy?

**Decision:**
- [x] Override syntax: `import "github:user/repo#tag"` forces git
- [x] Cache in `~/.c67/cache/` with git SHA for invalidation
- [x] Invalidate on manual delete or `c67 clean` command
- [x] Version conflicts: first import wins (simple, like Go before modules)

### 27. Export Star Semantics
**Issue:** `export *` puts functions in global namespace
- Name collision handling?
- Can you import two `export *` modules?
- What about transitive exports?

**Decision:**
- [x] Name collision is compile error (explicit > implicit)
- [x] Cannot import two `export *` modules with same names
- [x] Transitive exports NOT allowed (prevents namespace pollution)
- [x] Can re-export via explicit `export foo` after import

### 28. Circular Import Handling
**Issue:** Not documented how circular imports work
- Are they allowed?
- Compile-time vs runtime resolution?

**Decision:**
- [x] Circular imports are compile error (prevents initialization issues)
- [x] Compiler detects cycles during import resolution
- [x] No runtime resolution needed (keep simple)
- [x] Warning suggests refactoring to break cycle

## Syntax & Parsing

### 29. Block-as-Argument Conflict
**Issue:** parser.go has "TODO: Blocks-as-arguments disabled (conflicts with match expressions)"
- Is this feature wanted?
- How to resolve conflict?
- Alternative syntax?

**Decision:**
- [x] Permanently disabled (conflicts with match, not worth complexity)
- [x] Use explicit function parameter instead of block syntax
- [x] Keeps grammar simpler and more orthogonal
- [x] Document in LANGUAGESPEC.md as design choice

### 30. Struct Literal Syntax
**Issue:** parser.go: "TODO: Struct literal syntax conflicts with lambda match"
- Current workaround unclear
- Is this a permanent limitation?

**Decision:**
- [x] No dedicated struct literal syntax needed
- [x] Map literal `{x: 1, y: 2}` serves as struct literal (orthogonal!)
- [x] Named structs just add type checking to maps
- [x] Keeps syntax minimal - maps are universal data structure

### 31. Operator Precedence Edge Cases
**Issue:** Some operators have unclear precedence
- `or!` precedence vs `or` vs `||`
- `<>` (composition) precedence
- Operator usage: logical not, bitwise not, ownership/movement

**Decision:**
- [x] `or!` has lower precedence than `or` (error handling last)
- [x] `<>` (compose) binds tighter than arithmetic (functional style)
- [x] `not` is logical NOT (boolean negation: `not true` → `false`)
- [x] `!b` is bitwise NOT (binary negation: `!0xFF` → `0xFFFFFFFFFFFFFF00`)
- [x] `-` is for subtraction and negative numbers
- [x] `µ` is for memory ownership/movement (unique symbol, visually distinct)
- [x] Full precedence table in GRAMMAR.md

## Performance & Debugging

### 32. Debug Information Format
**Issue:** No mention of debug info generation
- DWARF for Linux?
- PDB for Windows?
- Source maps?

**Decision:**
- [x] Emit DWARF on Linux/Mac, PDB on Windows
- [x] Debug info includes line numbers and variable names
- [x] Inlined code tracked with DWARF inline markers
- [x] Always emit debug info (binary size not critical for C67 use cases)

### 33. Profiling Support
**Issue:** No profiling hooks documented
- How to profile C67 programs?
- Built-in profiler?
- External tool integration?

**Decision:**
- [x] Use external tools: perf, Instruments, VTune (don't reinvent)
- [x] Emit frame pointers for better profiler support
- [x] No built-in profiler (keep compiler simple)
- [x] Document profiling workflow in README.md

### 34. Benchmark Framework
**Issue:** TODO.md mentions "add simple benchmarks" for SIMD
- Should there be a standard benchmark framework?
- How to measure performance?
- Built-in or external?

**Decision:**
- [x] No standard benchmark framework (use external tools)
- [x] Manual timing via C FFI to clock_gettime/QueryPerformanceCounter
- [x] Comparison: user runs program twice, compares results
- [x] Keep it simple - benchmarking is advanced use case

## Stdlib & Builtins

### 35. Builtin Function Minimalism
**Issue:** Philosophy is "minimal builtins", but where's the line?
- `head()`/`tail()` are builtin
- Math functions via C FFI
- String operations via operators vs functions?

**Decision:**
- [x] Builtins: operators (+,-,*,/), map ops (head, tail, #), control flow
- [x] Everything else via C FFI (math, IO, strings beyond basics)
- [x] Criteria: must be impossible/impractical to implement in C67 itself
- [x] No deprecation needed - get it right first time (minimal!)

### 36. Printf Format Strings
**Issue:** printf exists but format string parsing unclear
- Does it use C printf semantics?
- What format codes are supported?
- How does it handle C67 types?

**Decision:**
- [x] Use C printf semantics exactly (compatibility, familiar)
- [x] Format codes: %f for numbers, %s for strings, standard C codes
- [x] Maps/lists: use %p for pointer, manual iteration to print elements
- [x] No custom codes - keep it simple, use C FFI for fancy formatting

### 37. File I/O
**Issue:** No documented file I/O beyond SDL IOFromFile
- Standard file operations?
- Buffering strategy?
- Error handling?

**Decision:**
- [x] Use C FFI (fopen, fread, fwrite, fclose)
- [x] No builtin file I/O (keep language minimal)
- [x] Async I/O via external libraries if needed
- [x] Memory-mapped files via mmap FFI

## Security

### 38. Unsafe Block Auditing
**Issue:** Unsafe blocks documented but no safety analysis
- Should unsafe code be auditable?
- Taint tracking?
- Should there be unsafe function marking?

**Decision:**
- [x] Unsafe blocks are lexically scoped (clear boundaries)
- [x] Functions containing unsafe NOT automatically marked (explicit only)
- [x] No automatic audit trail (complexity, use code review)
- [x] Unsafe cannot be disabled - trust programmer (C67 philosophy)

### 39. Safety & Runtime Protection
**Issue:** Balance between performance and preventing common programming errors
- Null pointer dereferences
- Division by zero
- Stack overflow
- Buffer overflows
- Array bounds violations

**Decision:**
- [x] **Null pointer checks:** Enabled by default, emit check before dereference (prevents crashes)
- [x] **Division by zero:** Enabled by default, check divisor before div/mod (prevents SIGFPE)
- [x] **Stack overflow:** Guard pages from OS (standard, no compiler work needed)
- [x] **Buffer overflow:** Bounds checking in safe mode (`-check-bounds` flag), off by default for performance
- [x] **Array access:** Map lookups safe by default (return 0.0 for missing keys), no UB
- [x] Can disable all safety checks with `-unsafe` flag (maximum performance)
- [x] Philosophy: Prevent crashes that corrupt state, allow programmer control for performance

### 40. Code Signing & Verification
**Issue:** No mention of executable signing
- Should executables be signable?
- Binary verification?

**Decision:**
- [x] No built-in code signing (use platform tools: codesign, signtool)
- [x] Reproducible builds: yes, same input → same output (deterministic)
- [x] No binary verification in compiler (external concern)
- [x] Supply chain: document dependencies, user verifies

## Testing & Validation

### 41. Test Organization
**Issue:** Tests scattered across *_test.go files
- No clear test hierarchy
- No integration vs unit test distinction

**Decision:**
- [x] Organization: *_test.go files by feature area (current approach works)
- [x] No separate dirs - Go test convention is fine
- [x] Test naming: Test<Feature>_<Scenario>
- [x] No required coverage target (quality > quantity)

### 42. Fuzzing Support
**Issue:** No fuzzing mentioned
- Should there be fuzzer integration?
- Corpus management?

**Decision:**
- [x] Future work: use go-fuzz for parser/lexer
- [x] Not priority now (basic functionality first)
- [x] Corpus stored separately if implemented
- [x] No continuous fuzzing initially (resource intensive)

### 43. Compiler Test Suite
**Issue:** No comprehensive compiler test suite
- Should there be official test suite?
- Conformance tests?

**Decision:**
- [x] Current *_test.go files serve as conformance tests
- [x] No separate test suite repo (keep it simple)
- [x] Tests evolve with language (no version skew)
- [x] All tests must pass before release (100% required)

---

## Prioritization Guide

**Critical (blocking issues):**
- #1, #2, #4, #5, #8, #11, #26

**High Priority (affects daily use):**
- #6, #9, #13, #14, #17, #20, #29, #35

**Medium Priority (nice to have):**
- #3, #7, #10, #15, #18, #21, #27, #36

**Low Priority (future work):**
- #12, #16, #19, #22, #31-34, #37-43

**Platform-Specific:**
- #23 (Windows), #24 (ARM64), #25 (RISC-V)

---

## How to Resolve

Each decision should:
1. Be discussed in GitHub issues
2. Have a design doc (if complex)
3. Update relevant .md files (GRAMMAR.md, LANGUAGESPEC.md)
4. Include test cases
5. Document in CHANGELOG

Track progress by moving items from this file to:
- ✅ RESOLVED.md (for decisions made)
- 🚫 REJECTED.md (for decisions rejected with rationale)
