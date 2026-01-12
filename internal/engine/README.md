# Engine Package

## Purpose

This package contains the **stable, well-tested, high-confidence** components of the Vibe67 compiler. Code is moved here only when it meets these criteria:

1. **Completion Status**: Marked as 95-100% complete
2. **Test Coverage**: Has comprehensive tests that pass
3. **Low Complexity**: Self-contained, minimal dependencies
4. **High Stability**: No known bugs, production-ready

## Design Principles

### What Goes in Engine

✅ **Core data structures** (AST nodes, types, tokens)
✅ **Utility modules** (arch, platform, errors, string helpers)
✅ **High-level interfaces** (lexer, parser abstractions)
✅ **Import/module resolution** (stable, well-tested)
✅ **Type system** (complete, foundational)

### What Stays in Main Package (For Now)

❌ **Machine code emission** - Error-prone, needs extra care
❌ **Code generation** - Complex, architecture-specific
❌ **Instruction encoders** - Low-level, format-sensitive
❌ **Binary formats** - ELF/Mach-O/PE writers (complex)
❌ **Optimization passes** - Still being refined

## Migration Strategy

### Phase 1: Foundation (Current)
- [x] arch.go - Platform and architecture types
- [x] types.go - Type system
- [x] errors.go - Error handling utilities
- [x] utils.go - String and utility functions

### Phase 2: Frontend (Next)
- [ ] lexer.go - Tokenization (95% complete)
- [ ] ast.go - AST data structures (98% complete)
- [ ] parser.go - Parsing logic (95% complete)

### Phase 3: Module System
- [ ] import_resolver.go - Import resolution (100% complete)
- [ ] dependencies.go - Dependency management

### Phase 4: Backend (Future, with caution)
- [ ] Register allocators (when stable)
- [ ] Instruction encoders (after thorough testing)
- [ ] Binary writers (after validation)

## Testing

All engine components must:
1. Have corresponding test files
2. Pass all tests before migration
3. Maintain test coverage after migration
4. Not break any existing functionality

Run tests: `go test ./internal/engine/...`

## Current Status

### Migrated (Stable)
- ✅ `arch.go` - Platform types (100% complete)
- ✅ `types.go` - Type system (100% complete)
- ✅ `errors.go` - Error handling (100% complete)
- ✅ `utils.go` - Utilities (100% complete)

### Pending (Need Review)
- 🔄 Lexer - 95% complete, needs careful migration
- 🔄 AST - 98% complete, large but stable
- 🔄 Parser - 95% complete, depends on lexer + AST

### Not Yet (Complex/Unstable)
- ⏸️ Code generation - Too complex, error-prone
- ⏸️ Instruction encoders - Machine code sensitive
- ⏸️ Binary formats - Platform-specific complexity

## Guidelines for Adding Code

Before moving code to engine:

1. **Check completion status** - Must be 95%+ complete
2. **Run tests** - All related tests must pass
3. **Check dependencies** - Minimal coupling to main package
4. **Document exports** - Add comments for public APIs
5. **Test isolation** - Can be tested independently

## Notes

- Machine code emission is **intentionally excluded** due to error-prone nature
- Focus on high-level, algorithmic code first
- Binary format writers need extensive testing before migration
- When in doubt, leave it in main package
