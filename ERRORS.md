# Known Errors and Limitations

This document tracks known errors, limitations, and edge cases in the C67 compiler.

## Language Limitations

### 1. Doubly-Nested Recursive Calls with Multiple Arguments

**Status**: FIXED ✅  
**Date Fixed**: 2025-12-16

**Description**: Functions with multiple arguments that make recursive calls where one argument is itself a recursive call now work correctly.

**Example**:
```c67
// Ackermann function - NOW WORKS CORRECTLY
ack = (m, n) {
    | m == 0 => n + 1
    | n == 0 => ack(m - 1, 1)
    ~> ack(m - 1, ack(m, n - 1))
}

ack(3, 3)  // Returns 61 (correct!)
ack(3, 4)  // Returns 125 (correct!)
```

**Fix**: Register allocation for nested function calls was corrected to properly save/restore registers.

---

### 2. List Display in Strings

**Status**: Partially Implemented
**Date**: 2025-12-16

**Description**: Lists can be created and manipulated, but when converted to strings (e.g., in f-strings or println), they show as a placeholder "[...]" instead of their actual contents.

**Example**:
```c67
x = [1, 2, 3]
println(f"x = {x}")  // Prints: x = [...]
```

**Status**: Lists work internally but need proper string conversion implementation.

**Note**: List concatenation and other list operations work correctly. This only affects display/printing.

---

## Reserved Keywords

The following are reserved keywords and cannot be used as variable names:

- `max` - Reserved keyword
- `min` - Reserved keyword (likely)

**Error Example**:
```c67
// ERROR: max is a reserved keyword
primes_helper = (current, max, acc) {  // Syntax error
    ...
}
```

**Fix**: Use alternative names like `limit`, `maximum`, `upper_bound`, etc.

---

## Notes for Developers

- Memoization is automatically applied to pure single-argument functions
- Multi-argument functions do not currently benefit from memoization
- Tail-call optimization works for properly tail-recursive functions
- Forward references work via automatic function reordering

---

## Testing

The following benchmark programs demonstrate working and problematic patterns:

### Working Benchmarks:
- `factorial.c67` - Recursive and tail-recursive factorial (both work perfectly)
- `fib.c67` / `fib_bench.c67` - Fibonacci with automatic memoization
- `primes.c67` - Prime counting (modified to avoid list building)

### Problematic Examples:
- `ackermann.c67` - Demonstrates the doubly-nested recursive call issue
