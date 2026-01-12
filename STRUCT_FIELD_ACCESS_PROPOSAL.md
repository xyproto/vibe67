# Language Improvement Proposal: Type-Aware Struct Field Access

## Problem

Currently, `event.type` fails when `event` is a raw `c.malloc()` pointer because:

1. Compiler doesn't track that `event` points to `SDL_Event`
2. FieldAccessExpr falls back to C67 map lookup
3. Raw memory bytes ≠ C67 map → segfault

## Current Workarounds

### 1. Manual Memory Reading (peek32/peek8)
```c67
event = c.malloc(192)
event_type = peek32(event, 0)  // Read uint32 at offset 0
```
**Status**: Implemented but has bugs in codegen

### 2. CStruct Declaration
```c67
cstruct SDL_Event {
    type as uint32
}
event = SDL_Event()  // Allocates typed struct
event_type = event.type  // Works! Compiler knows layout
```
**Status**: Partially works, but SDL_Event() constructor not implemented

## Proposed Solution: Type Casting with `as` Keyword

Add type information using the existing `as` keyword for casting:

```c67
// C67 idiomatic way: Arena allocation with type cast
arena {
    event := alloc(192) as SDL_Event
    event_type = event.type  // Compiler knows event is SDL_Event*, can read at offset 0
}

// Also works with c.malloc() for when you need manual control
event := c.malloc(192) as SDL_Event
event_type = event.type
c.free(event)

// Cast can also be used on existing pointers
raw_ptr = c.malloc(192)
event := raw_ptr as SDL_Event
event_type = event.type
```

### Implementation Plan

1. **Parser**: Accept `as TypeName` after expressions
   ```c67
   name := expression as TypeName
   ```

2. **AST**: Add TypeCast expression node
   ```go
   type TypeCast struct {
       Expression Expression
       TypeName   string
   }
   ```

3. **Type Tracking**: Store variable→type mapping in compiler
   ```go
   fc.varTypes map[string]string  // "event" → "SDL_Event"
   ```

4. **FieldAccessExpr Codegen**: Check if object has known struct type
   ```go
   if varType, ok := fc.varTypes[varName]; ok {
       if cstruct, exists := fc.cstructs[varType]; exists {
           // Use direct memory access at known offset
       }
   }
   ```

5. **CStruct Lookup**: Use existing `fc.cstructs` map for field offsets

### Benefits

- **Zero-cost**: No runtime overhead, just metadata for compiler
- **Type-safe**: Compiler validates field names against cstruct
- **Natural syntax**: Reuses existing `as` keyword for type casting
- **Consistent**: Works like other casts in C67
- **Arena-friendly**: Works with C67's arena allocation model
- **Backward compatible**: Untyped pointers still work
- **Reuses existing infra**: cstruct, FieldAccessExpr, `as` keyword, arena blocks

### Example Usage

```c67
import sdl3 as sdl

cstruct SDL_Event {
    type as uint32,
    timestamp as uint64
}

// C67 idiomatic way: Arena allocation with type cast
arena {
    event := alloc(192) as SDL_Event
    sdl.SDL_PollEvent(event)

    // Now this works!
    event_type = event.type  // Read at offset 0
    timestamp = event.timestamp  // Read at offset 4
    
    // Arena auto-cleans up when block ends
}
```

### Complete SDL Example

```c67
import sdl3 as sdl

cstruct SDL_Event {
    type as uint32,
    timestamp as uint64
    // ... more fields
}

// SDL event loop with arena allocation
running := 1
@ running > 0 max inf {
    arena {
        // Allocate typed event buffer in arena
        event := alloc(192) as SDL_Event
        
        has_event := sdl.SDL_PollEvent(event)
        
        has_event {
            0 => {}  // No events
            ~> {
                // Direct field access!
                event.type {
                    256 => { running = 0 }  // SDL_EVENT_QUIT
                    768 => {                 // SDL_EVENT_KEY_DOWN
                        // Read nested struct fields
                        scancode := peek32(event, 16)
                        scancode {
                            41 => { running = 0 }  // ESC
                            20 => { running = 0 }  // Q
                        }
                    }
                }
            }
        }
        
        // Arena automatically frees event at end of block
    }
    
    // ... render ...
}
```

### For Long-Lived Objects: Use Manual Allocation

```c67
// When you need control over lifetime (e.g., persistent event buffer)
event := c.malloc(192) as SDL_Event

@ running > 0 max inf {
    sdl.SDL_PollEvent(event)
    event.type {
        256 => { running = 0 }
    }
}

c.free(event)
```

### Alternative: Infer Type from cstruct Constructor

```c67
cstruct Point { x as int32, y as int32 }

// Auto-generate constructor that returns typed pointer
p = Point()  // Returns typed pointer, size from Point.size
p.x = 10     // Works! Compiler knows p is Point*
```

## Recommendation

Implement **Type Casting with `as` keyword** because:
- Reuses existing `as` keyword and semantics
- Most natural for C67 (consistent with existing conversions)
- Works with arena blocks (idiomatic C67)
- Also works with `c.malloc()` when needed
- Clear and explicit at point of allocation
- Can be used to recast existing pointers
- Aligns with := assignment style

The syntax `event := alloc(192) as SDL_Event` is:
- Concise
- Idiomatic C67 (uses arena)
- Self-documenting
- Zero runtime cost
- Memory-safe (arena auto-cleanup)

This would make C67's C FFI integration seamless while maintaining zero-cost abstractions, idiomatic syntax, and the arena allocation model that makes C67 memory-safe by default.
