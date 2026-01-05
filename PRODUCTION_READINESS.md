# C67 Production Readiness Analysis

**Date**: 2026-01-05
**Status**: Prototype → Production path defined

## Binary Size: 21KB → <1KB (5 Specific Solutions)

### Current State Analysis
- Hello World: 21,081 bytes
- ELF segments: 6 program headers with 4KB page alignment
- Dynamic linking overhead: ~4KB for .interp, PLT, GOT
- Unused runtime: printf, arena allocators, FMA checks

### Solution 1: Strip Dynamic Linker Dependency
**Saves**: ~4KB
**Method**: 
- Remove .interp section entirely
- Use only direct syscalls (write, exit, mmap)
- No libc dependency
**Implementation**: Already partially done for Linux, complete it

### Solution 2: Merge Segments to Single RWX
**Saves**: ~8KB (page alignment waste)
**Method**:
- Current: 3 LOAD segments with 4KB alignment = 12KB minimum
- Target: 1 LOAD segment with 1-byte alignment = actual code size
- Merge .text, .data, .rodata into single segment
**Implementation**: Custom ELF writer mode

### Solution 3: Dead Code Elimination
**Saves**: ~5KB
**Method**:
- Reachability analysis from _start
- Remove unused functions (printf formatting, arena allocators if unused)
- Strip FMA/AVX checks if not used
**Implementation**: Add DCE pass before codegen

### Solution 4: Minimal ELF Header (Overlap Trick)
**Saves**: ~150 bytes
**Method**:
- Overlap ELF header with PHDR entry
- Smallest valid ELF: 52 bytes (e_ident + minimal fields)
- Pack program headers tightly
**Implementation**: Research ELF golf techniques

### Solution 5: Add `-tiny` Flag
**Saves**: User choice
**Method**:
- Disable all dynamic sections when enabled
- Minimal error handling (no NaN-boxing)
- Strip symbol tables
- No arena allocators (user handles malloc)
**Implementation**: Compile-time flag

### Target Achieved
21KB → **<1KB** for minimal programs
Enables competitive 64k demos

## Pattern Matching: 3 Improvements for Elegance

### Improvement 1: Jump Tables for Dense Matches
**Problem**: Current linear compare chain for value matches
**Solution**: Generate switch/jump table for 10+ consecutive integer cases
```c67
// Before (linear):
x { 0 => "zero", 1 => "one", 2 => "two", ... }
// Generates: if x==0 goto L0; if x==1 goto L1; ...

// After (jump table):
// Generates: jmp [table + x*8]
// table: [L0, L1, L2, ...]
```
**Benefit**: O(1) vs O(n) for dense integer switches

### Improvement 2: Range Patterns
**Problem**: No way to match ranges elegantly
**Solution**: Add range syntax in match arms
```c67
grade = score {
    0..59 => "F"
    60..69 => "D"
    70..79 => "C"
    80..89 => "B"
    90..100 => "A"
    ~> "Invalid"
}
```
**Benefit**: Clearer intent, compiler can optimize to range checks

### Improvement 3: Tuple Destructuring in Matches
**Problem**: Can't pattern match on structure
**Solution**: Allow destructuring in match patterns
```c67
point = (3, 4)
result = point {
    (0, 0) => "origin"
    (x, 0) => f"x-axis at {x}"
    (0, y) => f"y-axis at {y}"
    (x, y) => f"point ({x}, {y})"
}
```
**Benefit**: More expressive, less boilerplate

## Priority Order (What to Do Next)

### Week 1-2: Binary Size
1. Implement dead code elimination (biggest win)
2. Add `-tiny` flag
3. Strip dynamic linker
4. Test with Hello World → should be <2KB

### Week 3-4: ARM64
1. Fix lambda execution
2. Fix C FFI resolution
3. Test all examples on ARM64

### Week 5-6: Developer Tools
1. Add DWARF v5 debug info
2. Basic LSP (go-to-def only)
3. Simple formatter

### Month 2: Patterns + Perf
1. Jump table codegen
2. Range patterns in parser
3. Benchmark suite
4. Register allocator upgrade

## Why This Order?

1. **Binary size first**: Demoscene blocker, relatively easy wins
2. **ARM64 second**: Mobile/Mac deployment critical for gamedev
3. **Tools third**: Productivity multiplier, but workarounds exist
4. **Patterns fourth**: Nice-to-have, doesn't block real work

## Current Strengths (Don't Touch)

✅ x86_64 codegen is solid
✅ Error handling (NaN-boxing) is elegant
✅ Tail-call optimization works
✅ Memory model (arena allocators) is sound
✅ SDL3 integration proves FFI works

## Conclusion

C67 is **90% there** for demoscene/gamedev/osdev. The remaining 10% is:
- Size optimization (technical debt)
- ARM64 bugs (platform coverage)
- Tooling (developer experience)

All are solvable in 2-3 months of focused work.
