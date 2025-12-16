# Known Errors and Limitations

This document tracks known errors, limitations, and edge cases in the C67 compiler.

## Language Limitations

### 1. Doubly-Nested Recursive Calls with Multiple Arguments

**Status**: Known Issue  
**Severity**: Medium

**Description**: Functions with multiple arguments that make recursive calls where one argument is itself a recursive call do not work correctly.

**Example**:
```c67
// Ackermann function - DOES NOT WORK CORRECTLY
ack = (m, n) {
    | m == 0 => n + 1
    | n == 0 => ack(m - 1, 1)
    ~> ack(m - 1, ack(m, n - 1))  // This pattern fails
}

ack(1, 1)  // Returns 2 instead of 3
ack(2, 2)  // Returns 2 instead of 7
```

**Workaround**: Use intermediate variables or refactor to avoid nested recursive calls in arguments.

**Works**:
```c67
// Simple recursive two-arg functions work fine
power = (base, exp) {
    | exp == 0 => 1
    ~> base * power(base, exp - 1)  // Single recursive call works
}

// Nested calls to different functions work
add(mul(2, 3), 4)  // Works: 10
```

**Root Cause**: Likely related to memoization or argument evaluation order in multi-argument recursive contexts.

---

### 2. List Building with Recursive Concatenation

**Status**: Known Issue  
**Severity**: Medium

**Description**: Building lists recursively using concatenation (`acc + [item]`) returns 0 instead of the expected list.

**Example**:
```c67
// List building - DOES NOT WORK CORRECTLY
build_list_helper = (current, limit, acc) {
    | current > limit => acc
    ~> build_list_helper(current + 1, limit, acc + [current])
}

build_list = (n) {
    build_list_helper(1, n, [])
}

build_list(5)  // Returns 0 instead of [1, 2, 3, 4, 5]
```

**Workaround**: Use counting or other accumulator patterns instead of list building.

**Works**:
```c67
// Counting instead of list building works fine
count_helper = (current, limit, count) {
    | current > limit => count
    ~> count_helper(current + 1, limit, count + 1)
}
```

**Root Cause**: Issue with list concatenation in recursive contexts or accumulator handling.

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
