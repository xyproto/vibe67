# Dynamic Linking Optimization - Future Work

## Goal

Further reduce binary size by eliminating PLT/GOT overhead for programs that don't call external functions.

## Current State (21KB minimum)

### What We Achieved

1. **Runtime Feature Tracking**: ✅ Working
   - Detects CPU features, arenas, string/list operations
   - Conditionally emits runtime code
   - Minimal program (`x = 42`) has no CPU detection, no arena init

2. **Binary Size**: ✅ 21KB (down from 45KB = 53% reduction)
   - Text segment: 12KB (down from 32KB)
   - Actual code in minimal program: ~60 bytes
   - Rest is ELF format overhead

### Current 21KB Breakdown

- **ELF Headers**: ~4KB (program headers, etc.)
- **PLT/GOT**: ~4KB (even when empty, structure is there)
- **Dynamic sections**: ~4KB (dynsym, dynstr, hash, rela)
- **Text segment**: 12KB (includes padding)
  - Actual code: ~60 bytes
  - Padding: ~12KB (page alignment)
- **Data/rodata**: ~1KB

## What Still Needs Dynamic Linking

Currently, the compiler ALWAYS enables dynamic linking (`useDynamicLinking = true`) in `codegen_elf_writer.go:20`.

### Actually Needs Dynamic Linking

- ❌ **Minimal programs**: No external calls (only syscalls like `exit`)
- ✅ **Programs with printf**: If using libc printf (we use syscalls, so no)
- ✅ **Programs with malloc**: If using libc malloc (we use mmap syscalls, so no)  
- ✅ **C FFI**: SDL3, pthread, custom C libraries
- ✅ **Explicit imports**: `use c "libc.so"`

### Key Insight

**Our runtime uses syscalls, not libc!**
- `exit()` → syscall (not libc call)
- `mmap()` → syscall (for arenas)
- `write()` → syscall (for printf via syscall)

So a minimal c67 program needs **ZERO external function calls**.

## Implementation Plan (For Future)

### Phase 1: Track Dynamic Linking Needs

Add to `runtime_tracker.go`:
```go
const (
    FeatureDynamicLink RuntimeFeature = "dynamic_link"
    FeatureMalloc      RuntimeFeature = "malloc"  
    FeatureCFFI        RuntimeFeature = "cffi"
)

func (rf *RuntimeFeatures) needsDynamicLinking() bool {
    return rf.Uses(FeatureDynamicLink) || 
           rf.Uses(FeatureMalloc) || 
           rf.Uses(FeatureCFFI)
}
```

Mark features when:
- C imports detected: `fc.runtimeFeatures.Mark(FeatureCFFI)`
- malloc/free called: `fc.runtimeFeatures.Mark(FeatureMalloc)`
- External library functions called

### Phase 2: Conditional Dynamic Linking

In `codegen_elf_writer.go`:
```go
// Only enable if actually needed
if fc.runtimeFeatures.needsDynamicLinking() {
    fc.eb.useDynamicLinking = true
}
```

### Phase 3: Static ELF Support

Modify `WriteCompleteDynamicELF` or create `WriteCompleteStaticELF`:

**Static ELF needs**:
- 4 program headers (not 6): PHDR, LOAD(ro), LOAD(rx), LOAD(rw)
- No INTERP segment
- No DYNAMIC segment  
- No PLT/GOT
- No dynsym/dynstr/hash/rela sections

**Estimated savings**: ~8KB (from 21KB → ~13KB)

### Phase 4: Optimize Text Segment Padding

Current 12KB text segment for 60 bytes of code is wasteful.

Options:
1. Reduce page alignment (risky - may break some systems)
2. Better packing of code/data
3. Investigate smaller page sizes

**Estimated savings**: ~8KB (from 13KB → ~5KB)

### Final Target

**5KB minimal binary**:
- 4KB: ELF headers + minimal segments
- 1KB: Actual code + data

## Challenges

### 1. ELF Format Complexity

`elf_complete.go` is 700+ lines designed for dynamic linking.

Creating a static variant requires:
- Different memory layout calculation
- Different program header count
- Conditional section writing
- Patching/relocation handling

### 2. Risk of Breaking Things

The ELF writer is production code that works well.

Need to:
- Keep backward compatibility
- Test thoroughly on different Linux versions
- Handle edge cases (empty programs, no data, etc.)

### 3. Minimal Gains for Most Programs

Most real c67 programs will:
- Use strings (needs arenas = mmap = syscalls only, still no dynamic linking)
- Use printf (we use syscalls, still no dynamic linking)
- Eventually use C FFI (SDL3, etc. = needs dynamic linking)

Only trivial programs benefit from removing dynamic linking.

## Recommendation

### Short Term: Document Current State

We achieved 53% size reduction (45KB → 21KB). This is significant!

The infrastructure is in place:
- Runtime feature tracking works
- Conditional code emission works
- Type-aware analysis works

### Medium Term: Track But Don't Act

Add dynamic linking tracking to the feature system, but don't change ELF generation yet.

This will:
- Provide data on how often it's actually static
- Prepare for future optimization
- Document the intent

### Long Term: Static ELF When Justified

If benchmarks show many programs don't need dynamic linking:
1. Implement static ELF writer
2. Make it opt-in via flag first
3. Test extensively
4. Make it automatic based on feature detection

## Code Locations

- **Feature tracking**: `runtime_tracker.go`
- **Analysis**: `codegen.go:analyzeRuntimeFeatures()`
- **ELF writing**: `codegen_elf_writer.go`, `elf_complete.go`
- **PLT/GOT**: `plt_got.go`, `elf_dynamic.go`
- **Dynamic sections**: `dynlib.go`, `elf_sections.go`

## Testing Needed

If implementing static ELF:
1. Test on multiple Linux distros (Ubuntu, Arch, Fedora, Alpine)
2. Test on different kernels (5.x, 6.x)
3. Test with/without ASLR
4. Test with different security settings (SELinux, AppArmor)
5. Verify `ldd` output
6. Verify execution permissions
7. Test debuggability

## Current Achievement Summary

✅ **What Works Now**:
- 53% binary size reduction for minimal programs
- Conditional CPU detection
- Conditional arena initialization  
- Conditional runtime helper emission
- Type-aware feature analysis
- Infrastructure for future optimizations

📋 **Future Work**:
- Static ELF for zero external dependencies (~8KB savings)
- Better text segment packing (~8KB savings)
- Target: 5KB minimal binary (vs current 21KB)

The foundation is solid. Further optimization is possible but requires careful ELF format work.
