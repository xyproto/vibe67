# TODO

## Priority 0: Binary Size Reduction (21KB → <1KB for demos)

### Critical for demoscene - current 21KB blocks 64k intros

1. [ ] Implement dead code elimination at function level
2. [ ] Strip dynamic linker if no C FFI used (auto-detect libc dependency)
3. [ ] Merge all segments into single RWX segment with custom ELF header (saves ~8KB)
4. [ ] Use smallest ELF header (overlap PHDR with ELF header, 52 bytes minimum)
5. [ ] Add verbose output showing which dynamic libs/functions are used

## Priority 1: ARM64 Backend Fixes

1. [ ] Fix lambda execution on ARM64 (empty output bug)
2. [ ] Fix C FFI calls on ARM64 (sin, cos, malloc treated as undefined)
3. [ ] Implement bit test operator `?b` for ARM64
4. [ ] Test division-by-zero protection on ARM64

## Priority 2: Pattern Matching Improvements

1. [ ] Generate jump tables for dense integer matches (10+ consecutive cases)
2. [ ] Add range patterns: `x { 0..10 => "small", 11..100 => "medium" }`
3. [ ] Add tuple destructuring: `point { (0, 0) => "origin", (x, y) => ... }`

## Priority 3: Developer Experience

1. [ ] Generate DWARF v5 debug info (enable GDB/LLDB debugging)
2. [ ] Implement basic LSP (go-to-definition, completions)
3. [ ] Add `c67 fmt` formatter
4. [ ] Fuzz test parser to prevent crashes

## Priority 4: Platform Support

1. [ ] Complete Mach-O writer for macOS/ARM64
2. [ ] Fix PE header generation for Windows (small executables)
3. [ ] Test RISC-V backend

## Priority 5: Performance

1. [ ] Benchmark suite vs C (gcc -O2) and Go
2. [ ] Upgrade register allocator to linear scan
3. [ ] Optimize O(n²) string iteration to O(n)
