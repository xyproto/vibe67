# Vibe67 Programming Language

[![Go CI](https://github.com/xyproto/vibe67/actions/workflows/ci.yml/badge.svg)](https://github.com/xyproto/vibe67/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/License-BSD_3--Clause-blue.svg)](LICENSE)

**Vibe67** is a high-performance systems programming language that compiles directly to machine code (x86_64) for Linux and Windows. It combines the minimalism of functional programming with the raw power of C.

## 🚀 Why Vibe67?

Vibe67 is designed for developers who value performance, simplicity, and transparency.

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
go install github.com/xyproto/vibe67@latest
```

Ensure `~/go/bin` is in your PATH.

## ⚡ Quick Start

Create `hello.vibe67`:

```vibe67
println("Hello, Vibe67!")
```

Compile and run:

```bash
vibe67 hello.vibe67 -o hello
./hello
```

## 📘 Language Tour

### 1. Everything is a Map
Vibe67 has one runtime type.
```vibe67
42              // {0: 42.0}
"Hi"            // {0: 72.0, 1: 105.0} (ASCII codes)
[1, 2]          // {0: 1.0, 1: 2.0}
{x: 10}         // {hash("x"): 10.0}
```

### 2. Variables & Functions
```vibe67
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

```vibe67
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

```vibe67
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

```vibe67
arena {
    data = allocate(1024)
    // ... use data ...
} // Automatically freed here

ptr := c.malloc(64)
defer c.free(ptr)
```

### 6. C Interop (FFI)
Call C libraries directly. Vibe67 parses headers and links dynamically.

```vibe67
import sdl3 as sdl

sdl.SDL_Init(sdl.SDL_INIT_VIDEO)
defer sdl.SDL_Quit()
```

## 🎮 Example: Displaying an image with SDL3

* Requires `img/grumpy-cat.bmp`.
* When compiling for Windows, this also requires `SDL3.dll` and the `include/SDL3` folder (with SDL3 header files).

```vibe67
// Import the SDL3 library (auto detect header files and library files with pkg-config on Linux, use SDL3.dll and include/* on Windows)
import sdl3 as sdl

// Set the window dimentions
width = 620
height = 387

// Initialize SDL with SDL_Init. Use the "or!" keyword to handle the case where SDL_Init returns nothing.
sdl.SDL_Init(sdl.SDL_INIT_VIDEO) or! {
    // Exitf is like printf, but writes to stderr and also quits the program with error code 1
    exitf("SDL_Init failed: %s\n", sdl.SDL_GetError())
}

// Call SDL_Quit when the program ends
defer sdl.SDL_Quit()

// Create window, or exit with an error
window = sdl.SDL_CreateWindow("Hello World!", width, height, sdl.SDL_WINDOW_RESIZABLE) or! {
    exitf("Failed to create window: %s\n", sdl.SDL_GetError())
}

// Call SDL_DestroyWindow when the program ends (before SDL_Quit)
defer sdl.SDL_DestroyWindow(window)

// Create renderer, or exit with an error
renderer = sdl.SDL_CreateRenderer(window, 0) or! {
    exitf("Failed to create renderer: %s\n", sdl.SDL_GetError())
}

// Call SDL_DestroyRenderer when the program ends (before SDL_DestroyWindow and SDL_Quit)
defer sdl.SDL_DestroyRenderer(renderer)

// Load BMP file, or exit with an error
file = sdl.SDL_IOFromFile("img/grumpy-cat.bmp", "rb") or! {
    exitf("Error reading file: %s\n", sdl.SDL_GetError())
}

// Load surface from file, or exit with an error
bmp = sdl.SDL_LoadBMP_IO(file, 1) or! {
    exitf("Error creating surface: %s\n", sdl.SDL_GetError())
}

// Clean up the surface when the program ends
defer sdl.SDL_DestroySurface(bmp)

// Create texture from surface, or exit with an error
tex = sdl.SDL_CreateTextureFromSurface(renderer, bmp) or! {
    exitf("Error creating texture: %s\n", sdl.SDL_GetError())
}

// Clean up the surface when the program ends
defer sdl.SDL_DestroyTexture(tex)

// Main rendering loop. Run for approximately 2 seconds (20 frames * 100ms = 2s)
@ frame in 0..<20 {

    // Clear screen
    sdl.SDL_RenderClear(renderer)

    // Render texture (fills entire window)
    sdl.SDL_RenderTexture(renderer, tex, 0, 0)

    // Present the rendered frame
    sdl.SDL_RenderPresent(renderer)

    // Delay to maintain framerate
    sdl.SDL_Delay(100)
}

// That's it
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

1.  Clone the repo: `git clone https://github.com/xyproto/vibe67`
2.  Run tests: `go test ./...`
3.  Submit a PR!

## 📄 Leneral info

* Version: 1.5.4
* License: BSD-3
