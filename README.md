# C67 Programming Language

[![Go CI](https://github.com/xyproto/c67/actions/workflows/ci.yml/badge.svg)](https://github.com/xyproto/c67/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/License-BSD_3--Clause-blue.svg)](LICENSE)

**C67** is a high-performance systems programming language that compiles directly to machine code (x86_64) for Linux and Windows. It combines the minimalism of functional programming with the raw power of C.

## 🚀 Why C67?

C67 is designed for developers who value performance, simplicity, and transparency.

*   **Blazing Fast Compilation**: Compiles directly to machine code. No LLVM, no intermediate steps, sub-second build times.
*   **Highly Optimized**:
    *   **Tail Call Optimization (TCO)**: Recursion is safe and efficient.
    *   **Pure Function Memoization**: Automatic caching for pure functions.
    *   **SIMD & FMA**: Automatic use of Fused Multiply-Add and AVX-512 instructions where available.
*   **Minimalist Syntax**:
    *   Unified syntax for functions, lambdas, and pattern matching.
    *   The `@` symbol handles all loops (range, while, infinite, for-each).
*   **Universal Type System**: Everything is a `map[uint64]float64` at runtime. This radical simplicity eliminates type erasure issues and simplifies serialization, while compile-time annotations (`: num`, `: str`) preserve safety.
*   **Compact & Standalone**: "Hello World" is ~21KB on Linux. No `libc` dependency on Linux (uses direct syscalls).
*   **Manual Memory Management w/ Safety**: First-class **Arena** allocators for bulk deallocation and `defer` for resource cleanup. No Garbage Collector pauses.

## 📦 Installation

```bash
go install github.com/xyproto/c67@latest
```

Ensure `~/go/bin` is in your PATH.

## ⚡ Quick Start

Create `hello.c67`:

```c67
println("Hello, C67!")
```

Compile and run:

```bash
c67 hello.c67 -o hello
./hello
```

## 📘 Language Tour

### 1. Everything is a Map
C67 has one runtime type.
```c67
42              // {0: 42.0}
"Hi"            // {0: 72.0, 1: 105.0} (ASCII codes)
[1, 2]          // {0: 1.0, 1: 2.0}
{x: 10}         // {hash("x"): 10.0}
```

### 2. Variables & Functions
```c67
// Immutable (default)
x = 42
add = (a, b) -> a + b

// Mutable
count := 0
count <- count + 1

// Implicit lambda in assignment
run = { println("Running...") }
```

### 3. Unified Control Flow
Pattern matching and lambdas share syntax.

```c67
// Value matching
sign = x {
    | x > 0 => "positive"
    | x < 0 => "negative"
    ~> "zero"
}

// Range Loop
@ i in 0..<10 {
    println(i)
}

// While Loop
@ count > 0 {
    count <- count - 1
}
```

### 4. Error Handling (`or!`)
The `or!` operator handles errors (encoded as NaN) or nulls (0.0).

```c67
// Returns 42 if risky() fails
val = risky() or! 42

// Executes block on error
file = open("data.txt") or! {
    println("Failed to open file")
    ret 1
}
```

### 5. Memory & Resources
Use `defer` for LIFO cleanup and `arena` for high-performance temporary allocations.

```c67
arena {
    data = allocate(1024)
    // ... use data ...
} // Automatically freed here

ptr := c.malloc(64)
defer c.free(ptr)
```

### 6. C Interop (FFI)
Call C libraries directly. C67 parses headers and links dynamically.

```c67
import sdl3 as sdl

sdl.SDL_Init(sdl.SDL_INIT_VIDEO)
defer sdl.SDL_Quit()
```

## 🎮 Example: SDL3 Pong
A complete, no-dependency graphical application in ~60 lines.

```c67
import sdl3 as sdl

WIDTH := 800
HEIGHT := 600

sdl.SDL_Init(sdl.SDL_INIT_VIDEO) or! exitln("Init failed")
defer sdl.SDL_Quit()

window := sdl.SDL_CreateWindow("Pong", WIDTH, HEIGHT, 0) or! exitln("Window failed")
defer sdl.SDL_DestroyWindow(window)

renderer := sdl.SDL_CreateRenderer(window, 0) or! exitln("Renderer failed")
defer sdl.SDL_DestroyRenderer(renderer)

ball_y := 300.0
speed := 4.0
running := 1

@ { // Infinite loop
    // Event handling
    @ {
        e := sdl.SDL_PollEvent(0)
        | e == 0 => ret @ // break inner loop
        | sdl.SDL_EventType(e) == sdl.SDL_EVENT_QUIT => { running = 0; ret @ }
    }
    | running == 0 => ret @ // break outer loop

    // Logic
    ball_y += speed
    | ball_y > 600 or ball_y < 0 => speed = -speed

    // Render
    sdl.SDL_SetRenderDrawColor(renderer, 0, 0, 0, 255)
    sdl.SDL_RenderClear(renderer)
    sdl.SDL_SetRenderDrawColor(renderer, 255, 255, 255, 255)
    sdl.SDL_RenderFillRect(renderer, 400, ball_y, 20, 20)
    sdl.SDL_RenderPresent(renderer)
    sdl.SDL_Delay(16)
}
```

## 🔧 Platform Support

| Platform | Arch | Status | Notes |
|----------|------|--------|-------|
| Linux | x86_64 | ✅ | Primary target, no libc required |
| Windows | x86_64 | ✅ | Native PE generation |
| Linux | ARM64 | ✅ | Raspberry Pi / Apple M1 (Linux) |
| Linux | RISC-V | ✅ | SiFive / StarFive |
| macOS | x86_64 | 🚧 | In progress |

## 🤝 Contributing

We welcome contributions from the open source community!

1.  Clone the repo: `git clone https://github.com/xyproto/c67`
2.  Run tests: `go test ./...`
3.  Submit a PR!

## 📄 License

BSD 3-Clause. See [LICENSE](LICENSE).