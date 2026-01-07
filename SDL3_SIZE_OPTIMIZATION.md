# SDL3 Binary Size Optimization: Low-Hanging Fruit

## Executive Summary
**Current**: C67 produces 22KB for SDL3 example vs GCC's 15KB (47% overhead)
**Target**: 16KB (matching GCC + reasonable C67 runtime overhead)
**Savings**: 6KB reduction (27% smaller)

---

## TOP 3 LOW-HANGING FRUIT (Easy Wins = 5-7KB!)

### 🍎 #1: Remove Unnecessary Arena Runtime (HIGHEST IMPACT)
**Current Problem**: Arena allocation code included even when not used
**Evidence**: 
```
DEBUG MarkLabel: _c67_arena_create at offset 2413
DEBUG MarkLabel: _c67_arena_alloc at offset 2603
DEBUG MarkLabel: _arena_alloc_fast at offset 2709
DEBUG MarkLabel: _arena_alloc_grow at offset 2730
DEBUG MarkLabel: _c67_arena_destroy at offset 2932
DEBUG MarkLabel: _c67_arena_reset at offset 2978
DEBUG MarkLabel: _c67_arena_ensure_capacity at offset 3460
```

**SDL3 example analysis**:
- Uses: `defer`, C FFI calls
- Does NOT use: `alloc()`, string concatenation, list operations
- Yet includes: Full arena runtime (create/alloc/grow/destroy/reset)

**Root Cause**: `usesArenas` flag set incorrectly or runtime always generated

**Solution**:
1. Check if `usesArenas` flag is true in SDL example (should be false!)
2. If true, find what's setting it (defer? string-to-C conversion?)
3. Add conditional generation in `generateRuntimeHelpers()`
```go
if fc.usesArenas {
    fc.generateArenaRuntime()
}
```

**Estimated Savings**: 2-3KB (arena code is 500-800 bytes, plus helper functions)

**Difficulty**: EASY (one conditional check)
**Code Location**: `codegen.go` runtime generation

---

### 🍎 #2: Replace exitf() with Syscall Version
**Current Problem**: `exitf()` uses libc's `dprintf/fprintf`
**Evidence**:
```c
nm -D /tmp/sdl3example | grep dprintf
U dprintf
```

**Impact**: Pulls in libc's printf formatting (~3KB of format parsing code)

**Solution**: Reuse `compilePrintfSyscall()` logic for exitf
```go
case "exitf":
    if fc.eb.target.OS() == OSLinux {
        // Compile-time format string parsing
        fc.compileExitfSyscall(call, strExpr)
        // Write to fd=2 (stderr) instead of fd=1 (stdout)
        // Then emit exit syscall
    } else {
        // Windows: use existing dprintf approach
    }
```

**Estimated Savings**: 2-4KB (depending on printf overhead)

**Difficulty**: EASY (copy-paste from printf_syscall.go)
**Code Location**: `codegen.go:14144-14300`

---

### 🍎 #3: Fix Duplicate Symbol Generation
**Current Problem**: Every SDL function appears twice in symbol table
**Evidence**:
```
U SDL_Init
U sdl.SDL_Init  
U SDL_CreateWindow
U sdl.SDL_CreateWindow
... (20+ functions x 2 = 40+ symbols!)
```

**Impact**: 
- Duplicate PLT entries (~16 bytes each)
- Duplicate GOT entries (8 bytes each)
- Duplicate relocations
- Total waste: 20 functions × 24 bytes = 480 bytes minimum

**Root Cause**: Import handling adds both qualified and unqualified names

**Solution**: Use only the unqualified name in PLT/GOT generation
```go
// When generating PLT entry for sdl.SDL_Init:
// Use "SDL_Init" not "sdl.SDL_Init"
```

**Estimated Savings**: 300-500 bytes

**Difficulty**: EASY (fix symbol name generation)
**Code Location**: Import handling in `elf_dynamic.go` or PLT generation

---

## Additional Quick Wins

### 🍏 #4: Remove Unused CPUID Code (80-150 bytes)
**Problem**: Entry point checks for FMA/AVX even when not used
**Solution**: Only emit CPUID when SIMD actually used
```go
if fc.usesSIMD || fc.usesFMA {
    fc.emitCPUIDChecks()
}
```

### 🍏 #5: Optimize String-to-C Conversions (1-2KB)
**Problem**: Each SDL string arg might inline conversion code
**Solution**: Generate single `_c67_string_to_cstr` helper, call it
**Note**: May already be implemented, needs verification

---

## Implementation Priority

### Phase 1: Quick Wins (1 hour) = 5-7KB savings
1. ✅ Remove arena runtime when not used (2-3KB)
2. ✅ Convert exitf to syscall version (2-4KB)  
3. ✅ Fix duplicate symbols (300-500 bytes)
4. ✅ Remove unused CPUID (80-150 bytes)

**Expected result**: 22KB → 16KB ✨

### Phase 2: Polish (2-4 hours) = additional 1-2KB
5. Optimize string conversion helpers
6. Shorter PLT stubs
7. Peephole optimizations

**Expected result**: 16KB → 14KB (better than GCC!)

---

## Windows PE Considerations

Same optimizations apply:
1. exitf uses `fprintf` on Windows too
2. PE also has import table overhead (similar to PLT/GOT)
3. Arena runtime bloat is platform-independent
4. CPUID checks still present

**Expected PE savings**: Similar 5-7KB reduction

---

## Verification Commands

```bash
# Check if arena is used:
./c67 -v examples/sdl3example.c67 2>&1 | grep -c arena

# Check symbol duplication:
nm -D sdl3example | grep SDL | sort | uniq -d

# Measure before/after:
ls -lh sdl3example
```

---

## Next Steps

1. **Start with arena removal** (biggest impact, easiest fix)
2. **Then exitf syscall** (high impact, proven technique)
3. **Fix duplicate symbols** (easy fix, measurable win)
4. Test all changes with `go test`
5. Document savings in TODO.md

**Estimated total time**: 1-2 hours for 5-7KB reduction! 🚀
