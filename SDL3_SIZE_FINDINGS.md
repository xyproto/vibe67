# SDL3 Binary Size Investigation - Findings

## Current State
- **SDL3 example binary**: 22KB
- **GCC equivalent**: 15KB  
- **Overhead**: 7KB (47%)

## Root Causes Identified

### 1. Arena Runtime Included Unnecessarily (2-3KB) ✓ CONFIRMED
**Problem**: Arena allocation code is generated even though SDL example doesn't use `alloc()` or string concat

**Evidence**:
```
DEBUG MarkLabel: _vibe67_arena_create at offset 2413
DEBUG MarkLabel: _vibe67_arena_alloc at offset 2603
DEBUG MarkLabel: _arena_alloc_grow at offset 2730
... (7 arena functions, ~2.5KB)
```

**Root Cause**: SDL_GetError() returns `const char*`, which gets converted to Vibe67 string format using `_vibe67_cstr_to_string()`. This function calls `fc.callArenaAlloc()` (line 8372 in codegen.go), which sets `usesArenas = true`.

**Fix Strategy**:
- Don't convert C strings to Vibe67 format if they're only used in format strings (%s)
- Pass `char*` directly to printf/exitf instead of converting
- Only convert when C string is stored in a Vibe67 variable

**Code Location**: codegen.go:8336-8430 (_vibe67_cstr_to_string generation)

**Estimated Savings**: 2-3KB

---

### 2. exitf() Uses libc dprintf (2-4KB) ✓ CONFIRMED  
**Problem**: `exitf()` calls libc's `dprintf` or `fprintf`, pulling in printf formatting code

**Evidence**:
```bash
nm -D sdl3example | grep dprintf
U dprintf  # Undefined symbol, needs libc
```

**Root Cause**: Line 14151-14305 in codegen.go uses dprintf for stderr output

**Fix Strategy**:
- Implement `compileExitfSyscall()` similar to `compilePrintfSyscall()`
- Write to fd=2 (stderr) instead of fd=1 (stdout)
- Reuse compile-time format string parsing
- This eliminates libc dependency for error messages

**Estimated Savings**: 2-4KB

---

### 3. Duplicate Symbol Generation (300-500 bytes) ✓ CONFIRMED
**Problem**: Each SDL function appears twice in symbol table

**Evidence**:
```
U SDL_Init
U sdl.SDL_Init
U SDL_CreateWindow
U sdl.SDL_CreateWindow
... (40+ symbols for 20 functions)
```

**Impact**: 20 functions × 24 bytes (PLT+GOT+reloc) = ~480 bytes

**Fix Strategy**: Use only unqualified name in PLT/GOT generation

---

### 4. CPUID Checks Always Included (80-150 bytes) ✓ CONFIRMED
**Problem**: Entry point includes FMA/AVX detection code even when not used

**Fix Strategy**: Only emit CPUID when `fc.usesSIMD || fc.usesFMA`

---

## Implementation Status

### Attempted:
- ✅ Identified all root causes
- ✅ Located exact code locations
- ⏸️ Started implementing exitf syscall (reverted due to syntax issues with heredoc)

### Next Steps:
1. **Fix arena conversion** (HIGHEST IMPACT):
   - Modify C FFI call handling to NOT convert char* to Vibe67 string when used in format strings
   - Check if result is used in %s context before calling _vibe67_cstr_to_string
   
2. **Implement exitf syscall** (HIGH IMPACT):
   - Create compileExitfSyscall() function properly (without heredoc issues)
   - Reuse compilePrintfSyscall logic with fd=2
   
3. **Remove duplicate symbols** (EASY WIN):
   - Strip namespace prefix in symbol generation

4. **Conditional CPUID** (EASY WIN):
   - Add `if fc.usesSIMD || fc.usesFMA` check before emitting CPUID

## Expected Results
- After fix #1: 22KB → 20KB (arena removal)
- After fix #2: 20KB → 16KB (exitf syscall)
- After fix #3+4: 16KB → 15.5KB (polish)

**Final target**: 15-16KB (matching or beating GCC!)

## Key Insight
The main bloat is NOT from the compiler's code generation, but from:
1. **Unnecessary runtime inclusion** (arenas when not needed)
2. **libc dependencies** (dprintf for exitf)
3. **Defensive conversions** (C string → Vibe67 string even when unnecessary)

All three are fixable with targeted changes to reduce "just in case" overhead.
