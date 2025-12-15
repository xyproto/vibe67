# TODO

## Completed
- [x] Let `println` handle multiple arguments.
- [x] Implement peephole optimization patterns (infrastructure exists in optimizer.go).
- [x] Add register pressure tracking to identify spill-heavy code
- [x] Add vector width detection for target platform
- [x] Add test cases to reproduce the closure capture issue
- [x] Add CPUID detection for BMI1/BMI2 support
- [x] Implement POPCNT instruction for bit counting
- [x] Implement TZCNT instruction for trailing zeros
- [x] Implement LZCNT instruction for leading zeros
- [x] Use BMI1/BMI2 instructions when available on x86-64
- [x] Add SIMD intrinsics for common operations
- [x] Implement simple register use/def analysis
- [x] Create data structures for live ranges
- [x] Add loop analysis to detect vectorization candidates

## High Priority - Feasible Sub-tasks

### SIMD and Vectorization
- [x] Add loop analysis to detect vectorization candidates
- [ ] Implement simple loop dependency analysis
- [x] Add vector width detection for target platform
- [ ] Auto-vectorize simple parallel loops using existing SIMD infrastructure
- [x] Add SIMD intrinsics for common operations

### Module-level mutable globals in lambdas
- [x] Add test cases to reproduce the closure capture issue
- [ ] Document current behavior vs expected behavior
- [ ] Fix variable scope tracking in lambda compilation
- [ ] Ensure mutable globals are properly referenced through rbp

### Register Allocation Improvements
- [x] Add register pressure tracking to identify spill-heavy code
- [x] Implement simple register use/def analysis
- [x] Create data structures for live ranges
- [ ] Implement live range analysis for better register allocation
- [ ] Add register reuse hints based on live ranges
- [ ] Implement linear scan register allocation to reduce spilling

### Import System
- [ ] Add test for cross-module closure initialization
- [ ] Fix closure variable capture in imported modules
- [ ] Verify import system properly initializes closures across modules
- [ ] Add circular dependency detection

## Architecture-Specific

### x86-64 Optimizations
- [x] Add CPUID detection for BMI1/BMI2 support
- [x] Implement POPCNT instruction for bit counting
- [x] Implement TZCNT instruction for trailing zeros
- [x] Implement LZCNT instruction for leading zeros
- [x] Use BMI1/BMI2 instructions when available on x86-64

### ARM64 Optimizations
- [ ] Add CSEL instruction support in ARM64 backend
- [ ] Replace conditional branches with CSEL where beneficial
- [ ] Use conditional select (CSEL) instead of branches on ARM64
- [ ] Add NEON instruction wrappers in ARM64 backend
- [ ] Implement NEON SIMD for vector operations
- [ ] Leverage NEON for SIMD operations on ARM64

### RISC-V Optimizations
- [ ] Add compressed instruction support in RISC-V backend
- [ ] Implement 16-bit instruction encoding for common operations
- [ ] Use compressed instructions for smaller code on RISC-V
- [ ] Add branch compression optimization

## Advanced Features (Future)

### Self-hosting Bootstrap
- [ ] Compile basic C67 parser in C67
- [ ] Compile C67 lexer in C67
- [ ] Compile C67 code generator in C67
- [ ] Implement self-hosting bootstrap (compile C67 compiler in C67)

### Advanced Optimizations
- [ ] Add call site profiling infrastructure
- [ ] Implement method lookup cache for polymorphic calls
- [ ] Add cache invalidation on type changes
- [ ] Implement polymorphic inline caching for dynamic dispatch optimization

### Defer Statement Enhancements
- [ ] Define exception propagation semantics for defer
- [ ] Implement defer stack unwinding on error
- [ ] Add exception propagation semantics for defer statements
- [ ] Add defer ordering guarantees in documentation

### Pattern Matching Enhancements
- [ ] Add support for tuple pattern matching: `(x, y) = tuple`
- [ ] Add support for nested pattern matching: `[[a, b], c] = nested_list`
- [ ] Add support for pattern guards in match expressions
- [ ] Extend pattern matching to support tuple destructuring and nested patterns

### Incremental Compilation
- [ ] Add file change detection (hot reload infrastructure exists)
- [ ] Implement function-level compilation cache
- [ ] Add dependency tracking between compilation units
- [ ] Add incremental compilation result caching

## Code Quality Improvements

### Performance
- [ ] Optimize O(n²) string iteration (codegen.go:10624)

### Code Generation
- [ ] Add explicit float precision conversions (codegen.go:5692)
- [ ] Implement length parameter for string operations (codegen.go:5821)
- [ ] Replace malloc with arena allocation for strings (codegen.go:6491, 7400)
- [ ] Add proper map iteration for string extraction (codegen.go:16853)

### Platform Support
- [ ] Implement Windows decompressor stub with VirtualAlloc (decompressor_stub.go:66)
- [ ] Implement ARM64 decompressor stub (compress.go:262)
- [ ] Implement proper import table generation for PE (pe.go:426)
- [ ] Implement RISC-V PLT generation (pltgot_rv64.go:11)

### Feature Completeness
- [ ] Implement function composition operator `<>` (codegen.go:16770)
- [ ] Handle "host:port" format in network operations (codegen.go:16801)
- [ ] Implement proper transformations for match expressions (codegen.go:18012, 18020)
- [ ] Re-enable blocks-as-arguments feature (parser.go:3689, 4030)
- [ ] Re-enable compression with debugged decompressor (default.go:144)
