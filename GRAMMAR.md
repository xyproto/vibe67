# C67 Grammar Specification

**Version:** 1.5.0
**Date:** 2025-12-03
**Status:** Canonical Grammar Reference for C67 3.0 Release

This document defines the complete formal grammar of the C67 programming language using Extended Backus-Naur Form (EBNF).

## ⚠️ CRITICAL: The Universal Type

C67 has exactly ONE runtime type: `map[uint64]float64`, an ordered map.

Not "represented as" or "backed by" — every value IS this map:

```c67
42              // {0: 42.0}
"Hello"         // {0: 72.0, 1: 101.0, 2: 108.0, 3: 108.0, 4: 111.0}
[1, 2, 3]       // {0: 1.0, 1: 2.0, 2: 3.0}
{x: 10}         // {hash("x"): 10.0}
[]              // {}
{}              // {}
```

**Even C foreign types are stored as maps:**

```c67
// C pointer (0x7fff1234) stored as float64 bits
ptr: cptr = sdl.SDL_CreateWindow(...)  // {0: <pointer_as_float64>}

// C string pointer
err: cstring = sdl.SDL_GetError()      // {0: <char*_as_float64>}

// C int
result: cint = sdl.SDL_Init(...)       // {0: 1.0} or {0: 0.0}
```

There are NO special types, NO primitives, NO exceptions.
Everything is a map from uint64 to float64.

This is not an implementation detail — this IS C67.

## Type Annotations

Type annotations are **metadata** that specify:
1. **Semantic intent** - what does this map represent?
2. **FFI conversions** - how to marshal at C boundaries
3. **Optimization hints** - compiler optimizations

They do NOT change the runtime representation (always `map[uint64]float64`).

### Native C67 Types
- `num` - number (default type)
- `str` - string (map of char codes)
- `list` - list (map with integer keys)
- `map` - explicit map

### Foreign C Types
- `cstring` - C `char*` (pointer stored as `{0: <ptr>}`)
- `cptr` - C pointer (e.g., `SDL_Window*`)
- `cint` - C `int`/`int32_t`
- `clong` - C `int64_t`/`long`
- `cfloat` - C `float`
- `cdouble` - C `double`
- `cbool` - C `bool`/`_Bool`
- `cvoid` - C `void` (return type only)

Foreign types are used at FFI boundaries to guide marshalling.

## Table of Contents

- [Grammar Notation](#grammar-notation)
- [Block Disambiguation Rules](#block-disambiguation-rules)
- [Shadow Keyword](#shadow-keyword)
- [Complete Grammar](#complete-grammar)
- [Lexical Elements](#lexical-elements)
- [Keywords](#keywords)
- [Operators](#operators)
- [Operator Precedence](#operator-precedence)

## Grammar Notation

The grammar uses Extended Backus-Naur Form (EBNF):

| Notation          | Meaning                   |
|-------------------|---------------------------|
| `=`               | Definition                |
| `;`               | Termination               |
| `\`               | Alternation               |
| `[ ... ]`         | Optional (zero or one)    |
| `{ ... }`         | Repetition (zero or more) |
| `( ... )`         | Grouping                  |
| `"..."`           | Terminal string           |
| `letter`, `digit` | Character classes         |

## Block Disambiguation Rules

When the parser encounters `{`, it determines the block type by examining contents:

### Rule 1: Map Literal
**Condition:** First element contains `:` (before any `=>` or `~>`)

```c67
config = { port: 8080, host: "localhost" }
settings = { "key": value, "other": 42 }
```

### Rule 2: Match Block
**Condition:** Contains `=>` or `~>` in the block's scope

There are TWO forms:

#### Form A: Value Match (with expression before `{`)
Evaluates expression, then matches its result against patterns:

```c67
// Match on literal values
x {
    0 => "zero"
    5 => "five"
    ~> "other"
}

// Boolean match
x > 0 {
    1 => "positive"    // true = 1
    0 => "zero"        // false = 0
}
```

#### Form B: Guard Match (no expression, uses `|` at line start)
Each branch evaluates its own condition independently:

```c67
// Guard branches with | at line start
{
    | x == 0 => "zero"
    | x > 0 => "positive"
    | x < 0 => "negative"
    ~> "unknown"  // optional default
}
```

**Important:** The `|` is only a guard marker when at the start of a line/clause.
Otherwise `|` is the pipe operator: `data | transform | filter`

### Rule 3: Statement Block
**Condition:** No `=>` or `~>` in scope, not a map

```c67
compute = x -> {
    temp = x * 2
    result = temp + 10
    result    // Last expression returned
}
```

**Disambiguation order:**
1. Check for `:` → Map literal
2. Check for `=>` or `~>` → Match block
3. Otherwise → Statement block

**Match block type:**
- Has expression before `{` → Value match
- No expression, has `|` at line start → Guard match

## Shadow Keyword

The `shadow` keyword is used to explicitly declare that a variable shadows (hides) an existing variable from an outer scope. This prevents accidental shadowing bugs while allowing intentional shadowing when needed.

### Syntax

```c67
shadow identifier [: type] = expression
shadow identifier [: type] := expression
```

### Rules

1. **Shadow is required** when declaring a variable that would shadow:
   - A module-level constant or variable
   - A variable from an outer function scope
   - A parameter from an outer lambda

2. **Shadow is forbidden** for:
   - Module-level declarations (nothing to shadow at top level)
   - First declaration of a name in a scope (nothing being shadowed)

3. **Without shadow**: Attempting to declare a variable with a name that exists in an outer scope is a compilation error

### Examples

```c67
// Module level
PORT = 8080
config = { host: "localhost" }

// Function that needs to use same name
main = {
    shadow PORT = 9000        // ✓ OK: explicitly shadows module PORT
    shadow config = {}        // ✓ OK: explicitly shadows module config
    println(PORT)             // Prints 9000
}

// Nested scopes
process = x -> {
    shadow x = x * 2          // ✓ OK: shadows parameter x
    inner = y -> {
        shadow x = x + y      // ✓ OK: shadows outer x
        x
    }
    inner(10)
}

// Error cases
main = {
    x = 42                    // ✓ OK: first declaration
    x = 100                   // ✗ ERROR: immutable, can't reassign
    shadow x = 100            // ✗ ERROR: shadow not needed, x is local
}

X = 100                       // Module level
test = {
    x = 42                    // ✗ ERROR: would shadow X (case-insensitive check)
    shadow x = 42             // ✓ OK: explicitly shadows
}
```

### Rationale

**Why require explicit shadowing?**
1. **Prevents bugs**: Accidental shadowing is a common source of errors
2. **Makes intent clear**: Reader knows shadowing is intentional
3. **Helps refactoring**: Renaming outer variables won't silently break inner scopes
4. **No ALL_UPPERCASE rule needed**: Variables can use natural naming in all scopes

**Case sensitivity:**
- Variable names are case-sensitive for lookup
- Shadow checking is case-insensitive to catch `x` shadowing `X`

## Import and Export System

C67's import system provides a unified way to import libraries, git repositories, and local directories. The export system controls which functions are available to importers and whether they require namespace prefixes.

### Export Statements

The `export` statement controls which functions are available to importers:

**Three export modes:**

1. **`export *`** - Export all functions into global namespace (no prefix required)
   ```c67
   export *

   hello = { println("Hello from this module!") }
   goodbye = { println("Goodbye!") }
   ```
   When imported:
   ```c67
   import "github.com/user/greetings" as greet
   hello()      // Works - no prefix needed
   goodbye()    // Works - no prefix needed
   ```

2. **`export func1 func2 ...`** - Export only listed functions (prefix required)
   ```c67
   export hello goodbye

   hello = { println("Hello!") }
   goodbye = { println("Goodbye!") }
   internal_helper = { println("Internal") }  // Not exported
   ```
   When imported:
   ```c67
   import "github.com/user/greetings" as greet
   greet.hello()         // Works - prefix required
   greet.goodbye()       // Works - prefix required
   greet.internal_helper()  // Error - not exported
   ```

3. **No export statement** - All functions available (prefix required)
   ```c67
   // No export statement

   hello = { println("Hello!") }
   goodbye = { println("Goodbye!") }
   ```
   When imported:
   ```c67
   import "github.com/user/greetings" as greet
   greet.hello()    // Works - prefix required
   greet.goodbye()  // Works - prefix required
   ```

**Design rationale:**
- `export *` is for beginner-friendly libraries (e.g., frameworks that provide a simplified API)
- `export func1 func2` is for controlled APIs with selective exposure
- No export is for general libraries where namespace pollution is a concern

**Example: Beginner-friendly library**
```c67
// simplelib/main.c67
export *

// Library initialization
init_window = (width, height, title) -> { ... }

// Drawing functions
draw_rect = (x, y, w, h, color) -> { ... }
draw_circle = (x, y, radius, color) -> { ... }
clear_screen = color -> { ... }

// Usage:
import "github.com/user/simplelib" as lib

// No prefixes needed - feels like built-in functions!
init_window(800, 600, "My App")
@ {
    clear_screen(0)
    draw_rect(100, 100, 50, 50, 0xFF0000)
    draw_circle(400, 300, 30, 0x00FF00)
}
```

### Import Resolution Priority

1. **Libraries** (highest priority)
   - System libraries via pkg-config (Linux/macOS)
   - .dll files in current directory or system paths (Windows)
   - Headers in standard include paths

2. **Git Repositories**
   - GitHub, GitLab, Bitbucket
   - SSH or HTTPS URLs
   - Optional version specifiers

3. **Local Directories** (lowest priority)
   - Relative or absolute paths
   - Current directory with `.`

### Import Syntax

```c67
// Library import (uses pkg-config or finds .dll)
import sdl3 as sdl
import raylib as rl

// Git repository import
import github.com/xyproto/c67-math as math
import github.com/xyproto/c67-math@v1.0.0 as math
import github.com/xyproto/c67-math@latest as math
import github.com/xyproto/c67-math@main as math
import git@github.com:xyproto/c67-math.git as math

// Directory import
import . as local                    // Current directory
import ./subdir as sub              // Relative path
import /absolute/path as abs        // Absolute path

// C library file import
import /path/to/libmylib.so as mylib
import SDL3.dll as sdl
```

### Import Behavior

- **Libraries**: Searches for library files and headers, parses C headers for FFI
- **Git Repos**: Clones to `~/.cache/c67/` (respects `XDG_CACHE_HOME`), imports all top-level `.c67` files
- **Directories**: Imports all top-level `.c67` files from the directory
- **Version Specifiers**:
  - `@v1.0.0` - Specific tag
  - `@main` or `@master` - Specific branch
  - `@latest` - Latest tag (or default branch if no tags)
  - No `@` - Uses default branch

### Namespace Rules

When importing a C67 module:

1. **If module has `export *`**: Functions available without prefix
   ```c67
   import "github.com/user/simplelib" as lib
   init_window()  // No prefix needed
   ```

2. **If module has `export func1 func2`**: Only listed functions available, prefix required
   ```c67
   import "github.com/user/api" as api
   api.exported_func()  // Prefix required
   api.internal_func()  // Error - not exported
   ```

3. **If module has no export**: All functions available, prefix required
   ```c67
   import "github.com/user/utils" as utils
   utils.helper()  // Prefix required
   ```

## Program Execution Model

C67 programs can be structured in three ways:

### 1. Main Function
When a `main` function is defined, it becomes the program entry point:

```c67
main = { println("Hello!") }     // A lambda that returns the value returned from println (true/1.0)
main = 42                        // A C67 number {0: 42.0}
main = () -> { 100 }             // A lambda that returns 100
main = { 100 }                   // A lambda that returns 100
```

**Return value rules:**
- If `main` is set to a number, it is converted to int32 for the exit code
- If `main` returns an empty map `{}` or empty list `[]` or true: exit code 0
- If `main` is callable (function): called, result becomes exit code
- Return values are implicitly cast to int32 for `_start`

### 2. Main Variable
When a `main` variable (not a function) is defined without top-level code:

```c67
main = 42        // Exit with code 42
main = {}        // Exit with code 0 (empty map)
main = []        // Exit with code 0 (empty list)
```

**Evaluation:**
- The value of `main` becomes the program's exit code
- Non-callable values are used directly

### 3. Top-Level Code
When there's no `main` function or variable, top-level code executes:

```c67
println("Hello!")
x := 42
println(x)
// Last expression or ret determines exit code
```

**Exit code:**
- Last expression value becomes exit code
- `ret` keyword sets explicit exit code
- No explicit return: returns true (1.0), exit code 0

### Mixed Cases

**Top-level code + main function:**
- Top-level code executes first
- It's the responsibility of top-level code to call `main()`
- If top-level doesn't call `main()`, `main()` is never executed
- Last expression in top-level code provides exit code

```c67
// Top-level setup
x := 100

main = { println(x); 42 }

// main is defined but not called - exit code is 0
// To call: main() must appear in top-level code
```

**Top-level code + main variable:**
- Top-level code executes
- `main` variable is accessible but not special
- Last top-level expression provides exit code

```c67
main = 99

println("Setup")
42  // Exit code is 42, not 99
```

## Complete Grammar

```ebnf
program         = { statement { newline } } ;

statement       = assignment
                | expression_statement
                | loop_statement
                | unsafe_statement
                | arena_statement
                | parallel_statement
                | cstruct_decl
                | class_decl
                | return_statement
                | defer_statement
                | import_statement
                | export_statement ;

return_statement = "ret" [ "@" [ integer ] ] [ expression ] ;

defer_statement  = "defer" expression ;

import_statement = "import" import_source [ "as" identifier ] ;

export_statement = "export" ( "*" | identifier { identifier } ) ;

import_source   = string_literal           (* library name, file path, or directory, unquoted string *)
                | git_url [ "@" version_spec ] ; (* git repository with optional version *)

git_url         = identifier { "." identifier } { "/" identifier }  (* github.com/user/repo *)
                | "git@" identifier ":" identifier "/" identifier ".git" ; (* git@github.com:user/repo.git *)

version_spec    = identifier              (* tag, branch, "latest", or semver like "v1.0.0" *)
                | "latest" ;

cstruct_decl    = "cstruct" identifier "{" { field_decl } "}" ;

field_decl      = identifier "as" c_type [ "," ] ;

class_decl      = "class" identifier [ extend_clause ] "{" { class_member } "}" ;

extend_clause   = { "<>" identifier } ;

class_member    = class_field_decl
                | method_decl ;

class_field_decl = identifier "." identifier "=" expression ;

method_decl     = identifier "=" lambda_expr ;

c_type          = "int8" | "int16" | "int32" | "int64"
                | "uint8" | "uint16" | "uint32" | "uint64"
                | "float32" | "float64"
                | "cptr" | "cstring" ;

arena_statement = "arena" block ;

loop_statement  = "@" block
                | "@" identifier "in" expression [ "max" expression ] block
                | "@" expression [ "max" expression ] block ;

parallel_statement = "||" identifier "in" expression block ;

unsafe_statement = "unsafe" type_cast block [ block ] [ block ] ;

type_cast       = "int8" | "int16" | "int32" | "int64"
                | "uint8" | "uint16" | "uint32" | "uint64"
                | "float32" | "float64"
                | "number" | "string" | "list" | "address"
                | "packed" | "aligned" ;

assignment      = identifier [ ":" type_annotation ] ("=" | ":=" | "<-") expression
                | identifier ("+=" | "-=" | "*=" | "/=" | "%=" | "**=") expression
                | indexed_expr "<-" expression
                | identifier_list ("=" | ":=" | "<-") expression ;  // Multiple assignment

identifier_list = identifier { "," identifier } ;

// Module-level naming constraint (enforced by parser):
// All assignments at module level (outside functions/lambdas) MUST use UPPERCASE identifiers.
// This prevents shadowing and makes globals visually distinct from locals.

type_annotation = native_type | foreign_type ;

native_type     = "num" | "str" | "list" | "map" ;

foreign_type    = "cstring" | "cptr" | "cint" | "clong"
                | "cfloat" | "cdouble" | "cbool" | "cvoid" ;

indexed_expr    = identifier "[" expression "]" ;

expression_statement = expression [ match_block ] ;

match_block     = "{" ( default_arm
                      | match_clause { match_clause } [ default_arm ]
                      | guard_clause { guard_clause } [ default_arm ] ) "}" ;

match_clause    = expression [ "=>" match_target ] ;

guard_clause    = "|" expression "=>" match_target ;  // | must be at start of line

default_arm     = ( "~>" | "_" "=>" ) match_target ;

match_target    = jump_target | expression ;

jump_target     = integer ;

block           = "{" { statement { newline } } [ expression ] "}" ;

expression      = pipe_expr ;

pipe_expr       = reduce_expr { ( "|" | "||" ) reduce_expr } ;

reduce_expr     = receive_expr ;

receive_expr    = "<=" pipe_expr | or_bang_expr ;

or_bang_expr    = send_expr { "or!" send_expr } ;

send_expr       = or_expr { "<-" or_expr } ;

or_expr         = and_expr { "or" and_expr } ;

xor_expr        = and_expr { "xor" and_expr } ;

and_expr        = comparison_expr { "and" comparison_expr } ;

comparison_expr = bitwise_or_expr { comparison_op bitwise_or_expr } ;

comparison_op   = "==" | "!=" | "<" | "<=" | ">" | ">=" ;

bitwise_or_expr = bitwise_xor_expr { "|b" bitwise_xor_expr } ;

bitwise_xor_expr = bitwise_and_expr { "^b" bitwise_and_expr } ;

bitwise_and_expr = shift_expr { "&b" shift_expr } ;

shift_expr      = additive_expr { shift_op additive_expr } ;

shift_op        = "<<b" | ">>b" | "<<<b" | ">>>b" ;

additive_expr   = multiplicative_expr { ("+" | "-") multiplicative_expr } ;

multiplicative_expr = power_expr { ("*" | "/" | "%") power_expr } ;

power_expr      = unary_expr { ( "**" | "^" ) unary_expr } ;

unary_expr      = ( "-" | "not" | "!b" | "~b" | "#" | "µ" ) unary_expr
                | postfix_expr ;

postfix_expr    = primary_expr { postfix_op } ;

postfix_op      = "[" expression "]"
                | "." ( identifier | integer )
                | "(" [ argument_list ] ")"
                | "#"
                | match_block ;

primary_expr    = identifier
                | number
                | string
                | fstring
                | list_literal
                | map_literal
                | lambda_expr
                | enet_address
                | address_value
                | instance_field
                | this_expr
                | "(" expression ")"
                | "??"
                | unsafe_expr
                | arena_expr ;

instance_field  = "." identifier ;

this_expr       = "." [ " " | newline ] ;  // Dot followed by space or newline means "this"

enet_address    = "&" port_or_host_port ;

port_or_host_port = port | [ hostname ":" ] port ;

address_value   = "$" expression ;

port            = digit { digit } ;

hostname        = identifier | ip_address ;

ip_address      = digit { digit } "." digit { digit } "." digit { digit } "." digit { digit } ;

arena_expr      = "arena" "{" { statement { newline } } [ expression ] "}" ;

unsafe_expr     = "unsafe" "{" { statement { newline } } [ expression ] "}"
                  [ "{" { statement { newline } } [ expression ] "}" ]
                  [ "{" { statement { newline } } [ expression ] "}" ]
                  [ "as" type_cast ] ;

lambda_expr     = [ parameter_list ] "->" lambda_body
                | parameter_list block  // Arrow optional with parenthesized params + block body
                | block ;  // Inferred lambda with no parameters in assignment context

parameter_list  = variadic_params
                | identifier { "," identifier }
                | "(" [ param_decl_list ] ")" ;

param_decl_list = param_decl { "," param_decl } ;

param_decl      = identifier [ ":" type_annotation ] [ "..." ] ;

variadic_params = "(" identifier [ ":" type_annotation ] { "," identifier [ ":" type_annotation ] } "," identifier [ ":" type_annotation ] "..." ")" ;

lambda_body     = [ "->" type_annotation ] ( block | expression [ match_block ] ) ;

// Lambda Syntax Rules:
//
// Explicit lambda syntax (always works):
//   x -> x * 2                                    // One parameter, expression body
//   (x, y) -> x + y                               // Multiple parameters (parens required)
//   (x, y, rest...) -> sum(rest)                  // Variadic parameters (last param with ...)
//   -> println("hi")                              // No parameters (explicit ->)
//   x -> { temp = x * 2; temp }                   // Single param with block body
//   (n) -> { n * n }                              // Parenthesized param with block body
//   (n) { n * n }                                 // Arrow optional when params in parens + block
//   (a, b) { a + b }                              // Multiple params, arrow optional with block
//
// With type annotations:
//   (x: num, y: num) -> num { x + y }             // Parameter and return types
//   (name: str) -> str { upper(name) }            // String function
//   (ptr: cptr) -> cint { sdl.SDL_DoSomething(ptr) }  // C types
//   greet(name: str) -> str { f"Hello, {name}!" } // Function definition with types
//
// Inferred lambda syntax (works ONLY in assignment context):
//   main = { println("hello") }                   // Inferred: main = -> { println("hello") }
//   handler = { | x > 0 => "pos" }                // Inferred: handler = -> { | x > 0 => "pos" }
//
// When `->` can be omitted:
//   1. Parameters in parentheses with block body: `(n) { n * 2 }` or `(a, b) { a + b }`
//   2. In assignment context with just block: `name = { ... }`
//   3. Block contains statements or guard match (| at line start)
//
// When `->` is REQUIRED:
//   1. Single parameter without parens: `x -> x * 2` (arrow distinguishes from identifier)
//   2. Lambda body is an expression, not block: `-> 42` or `n -> n * 2`
//   3. Lambda is NOT being assigned: `[1, 2, 3] | x -> x * 2`
//   4. Lambda is a function argument: `map(data, x -> x * 2)`
//
// Parentheses rules:
//   - Single parameter: `x -> x * 2` (no parens) OR `(x) -> x * 2` (with parens)
//   - Single parameter + block: `x -> { ... }` OR `(x) { ... }` (arrow optional with parens)
//   - Multiple parameters: `(x, y) -> x + y` (parens required)
//   - Multiple parameters + block: `(x, y) { x + y }` (arrow optional)
//   - Type annotations: `(x: num) -> num { x * 2 }` (parens required)
//   - No parameters with explicit ->: `-> println("hi")` (no parens needed)
//   - No parameters inferred from block: `main = { ... }` (no parens needed)
//
// Block type determination:
//   { x: 10 }                         // Map literal (has `:` before any `=>`)
//   { | x > 0 => "pos" }              // Guard match block (has `|` at line start)
//   { temp = x * 2; temp }            // Statement block (no `:`, no `=>` or `~>`)
//   { stmt1; stmt2; | guard => result } // Mixed block (statements + guards)
//   x { 0 => "zero" ~> "other" }      // Value match (expression before `{`)
//
// Examples:
//   // Function definitions (inferred lambda)
//   main = { println("Hello!") }
//   process = { | x > 0 => "pos" | x < 0 => "neg" }
//
//   // Lambdas with parameters (explicit)
//   square = x -> x * x
//   add = (x, y) -> x + y
//   map_fn = f -> data | f
//
//   // Method definitions in classes (always use `=`)
//   class Point {
//       distance = other -> sqrt((other.x - .x) ** 2 + (other.y - .y) ** 2)
//   }

argument_list   = expression { "," expression } ;

list_literal    = "[" [ expression { "," expression } ] "]" ;

map_literal     = "{" [ map_entry { "," map_entry } ] "}" ;

map_entry       = ( identifier | string ) ":" expression ;

identifier      = letter { letter | digit | "_" } ;

number          = [ "-" ] digit { digit } [ "." digit { digit } ] ;

string          = '"' { character } '"' ;

fstring         = 'f"' { character | "{" expression "}" } '"' ;
```

## Lexical Elements

### Identifiers

Identifiers start with a letter and contain letters, digits, or underscores:

```ebnf
identifier = letter { letter | digit | "_" } ;
letter     = "a" | "b" | ... | "z" | "A" | "B" | ... | "Z" ;
digit      = "0" | "1" | ... | "9" ;
```

**Rules:**
- Case-sensitive
- Can start with letter only (not digit or underscore)
- No length limit
- Can include Unicode letters

**Valid examples:**
```c67
x, count, user_name, myVar, value2, Temperature, λ
```

**Invalid:**
```c67
2count     // starts with digit
_private   // starts with underscore
my-var     // contains hyphen
```

### Numbers

Numbers are `map[uint64]float64` with a single entry at key 0:

```ebnf
number = [ "-" ] digit { digit } [ "." digit { digit } ] ;
```

**Examples:**
```c67
42              // {0: 42.0}
3.14159         // {0: 3.14159}
-17             // {0: -17.0}
0.001           // {0: 0.001}
1000000         // {0: 1000000.0}
-273.15         // {0: -273.15}
```

**Special values:**
- `??` - cryptographically secure random number [0, 1) → `{0: random_value}`
- Result of `0/0` - NaN (used for error encoding) → `{0: NaN}`

**Note:** While the values stored happen to be IEEE 754 doubles, this is an implementation detail. Numbers ARE maps, not primitives.

### Strings

Strings are `map[uint64]float64` where keys are indices and values are character codes:

```ebnf
string = '"' { character } '"' ;
```

**Examples:**
```c67
"Hello"         // {0: 72.0, 1: 101.0, 2: 108.0, 3: 108.0, 4: 111.0}
"A"             // {0: 65.0}
""              // {} (empty map)
```

**Escape sequences:**
- `\n` - newline (character code 10)
- `\t` - tab (character code 9)
- `\r` - carriage return (character code 13)
- `\\` - backslash
- `\"` - quote
- `\xHH` - hex byte
- `\uHHHH` - Unicode code point

**String operations:**
- `.bytes` - get byte array
- `.runes` - get Unicode code point array
- `+` - concatenation
- `[n]` - access byte at index

### F-Strings (Interpolated Strings)

F-strings allow embedded expressions:

```ebnf
fstring = 'f"' { character | "{" expression "}" } '"' ;
```

**Examples:**
```c67
name = "World"
greeting = f"Hello, {name}!"
result = f"2 + 2 = {2 + 2}"
```

### Comments

```c67
// Single-line comment (C++ style)
```

No multi-line comments.

## Keywords

### Reserved Keywords

```
ret arena unsafe cstruct class as max this defer spawn import shadow
```

**Note:** In C67, lambda definitions use `->` (thin arrow) and match arms use `=>` (fat arrow), similar to Rust syntax, except that `~>` is used for the default case.

**No-argument lambdas** can be written as `-> expr` or inferred from context in assignments: `name = { ... }`

The `shadow` keyword is required when declaring a variable that would shadow an outer scope variable (see Shadow Keyword section above).

### Type Keywords

Type annotations use these keywords (context-dependent):

**Native C67 types:**
```
num str list map
```

**Foreign C types:**
```
cstring cptr cint clong cfloat cdouble cbool cvoid
```

**Legacy type cast keywords (for `unsafe` blocks and `cstruct`):**
```
int8 int16 int32 int64 uint8 uint16 uint32 uint64 float32 float64
cptr cstring number string address packed aligned
```

**Usage:**
```c67
// Type annotations (preferred)
x: num = 42
name: str = "Alice"
ptr: cptr = sdl.SDL_CreateWindow(...)

// Type casts in unsafe blocks (legacy)
value = unsafe int32 { ... }
```

Type keywords are contextual - you can still use them as variable names in most contexts:

```c67
num = 100              // OK - variable named num
x: num = num * 2       // OK - type annotation vs variable
```

## Memory Management and Builtins

**CRITICAL DESIGN PRINCIPLE:** C67 keeps builtin functions to an ABSOLUTE MINIMUM.

**Memory allocation:**
- NO `malloc`, `free`, `realloc`, or `calloc` as builtins
- Use arena allocators: `allocate()` within `arena {}` blocks (recommended)
- Or use C FFI: `c.malloc`, `c.free`, `c.realloc`, `c.calloc` (explicit)

```c67
// Recommended: arena allocator
result = arena {
    data = allocate(1024)
    process(data)
}

// Alternative: explicit C FFI
ptr := c.malloc(1024)
defer c.free(ptr)
```

**List operations:**
- Use builtin functions: `head(xs)` for first element, `tail(xs)` for remaining elements
- Use `#` length operator (prefix or postfix)

**Why minimal builtins?**
1. **Simplicity:** Less to learn and remember
2. **Orthogonality:** One concept, one way
3. **Extensibility:** Users can define their own functions
4. **Predictability:** No hidden magic

**What IS builtin:**
- Operators: `#`, arithmetic, logic, bitwise
- Control flow: `@`, match blocks, `ret`
- Core I/O: `print`, `println`, `printf` (and error/exit variants)
- List operations: `head()`, `tail()`
- Keywords: `arena`, `unsafe`, `cstruct`, `class`, `defer`, etc.

**Everything else via:**
1. **Operators** for common operations (`#xs` for length)
2. **Builtin functions** for core operations (`head(xs)`, `tail(xs)`)
3. **C FFI** for system functionality (`c.sin`, `c.malloc`, etc.)
4. **User-defined functions** for application logic

## Operators

### Arithmetic Operators

```
+    Addition
-    Subtraction (binary) or negation (unary)
*    Multiplication
/    Division
%    Modulo
**   Exponentiation
^    Exponentiation (alias for **)
```

### Comparison Operators

```
==   Equal
!=   Not equal
<    Less than
<=   Less than or equal
>    Greater than
>=   Greater than or equal
```

### Logical Operators

```
and  Logical AND (short-circuit)
or   Logical OR (short-circuit)
xor  Logical XOR
not  Logical NOT
```

### Bitwise Operators

All bitwise operators use `b` suffix:

```
&b    Bitwise AND
|b    Bitwise OR
^b    Bitwise XOR
!b    Bitwise NOT (unary)
~b    Bitwise NOT (alias for !b)
<<b   Left shift
>>b   Arithmetic right shift
<<<b  Rotate left
>>>b  Rotate right
?b    Bit test (tests if bit at position is set, returns 1 or 0)
```

Example of bit test:
```c67
x = 0b10110  // Binary 22
bit2 = x ?b 2  // Returns 1 (bit 2 is set)
bit3 = x ?b 3  // Returns 0 (bit 3 is not set)
```

### Assignment Operators

```
=     Immutable assignment (cannot reassign variable or modify value)
:=    Mutable assignment (can reassign variable and modify value)
<-    Update/reassignment (for mutable vars)

+=    Add and assign (for lists: append element)
-=    Subtract and assign
*=    Multiply and assign
/=    Divide and assign
%=    Modulo and assign
**=   Exponentiate and assign
```

**Arrow Operator Summary:**

| Operator | Context           | Meaning                            | Example                            |
|----------|-------------------|------------------------------------|------------------------------------|
| `->`     | Lambda definition | Lambda arrow                       | `x -> x * 2` or `-> println("hi")` |
| `=>`     | Match block       | Match arm                          | `x { 0 => "zero" ~> "other" }`     |
| `~>`     | Match block       | Default match arm                  | `x { 0 => "zero" ~> "other" }`     |
| `_ =>`   | Match block       | Default match arm (alias for ~>)   | `x { 0 => "zero" _ => "other" }`   |
| `=`      | Variable binding  | Immutable assignment               | `x = 42` (standard for functions)  |
| `:=`     | Variable binding  | Mutable assignment                 | `x := 42` (can reassign later)     |
| `<-`     | Update/Send       | Update mutable var OR send to ENet | `x <- 99` or `&8080 <- msg`        |
| `<=`     | Comparison        | Less than or equal                 | `x <= 10`                          |
| `>=`     | Comparison        | Greater than or equal              | `x >= 10`                          |

**Important Conventions:**
- **Functions/methods** should use `=` (immutable), not `:=`, since they rarely need reassignment
- **Lambda syntax**: `->` always defines a lambda, `=>` always defines a match arm
- **Update operator** `<-` is for updating existing mutable variables or sending to ENet channels
- **Comparison** operators `<=` and `>=` are for comparisons, not assignment or arrows

**Multiple Assignment (Tuple Unpacking):**

```c67
// Functions can return multiple values as a list
a, b = some_function()  // Unpack first two elements
x, y, z := [1, 2, 3]    // Unpack list literal

// Practical example with pop()
new_list, popped_value = pop(old_list)
```

When a function returns a list, multiple assignment unpacks the elements:
- Right side must evaluate to a list/map
- Left side specifies variable names separated by commas
- Variables are assigned elements at indices 0, 1, 2, etc.
- If list has fewer elements than variables, remaining variables get 0
- If list has more elements, extra elements are ignored

### Collection Operators

```
#     Length operator (prefix or postfix)
```

### Other Operators

```
->    Lambda arrow (can be omitted in assignment context with blocks)
=>    Match arm
~>    Default match arm
|     Pipe operator
||    Parallel map
<>    Function composition (f <> g creates a new function that applies g then f)
<-    Update/Send (update mutable var OR send to ENet)
<=    Receive (ENet, prefix) OR less-than-or-equal comparison
µ     Memory ownership/movement operator (prefix)
.     Field access
[]    Indexing
()    Function call (parentheses optional for zero or one argument in some contexts)
@     Loop
&     ENet address (network endpoints)
$     Address value (memory addresses)
??    Random number (cryptographically safe)
or!   Error/null handler (executes right side if left is error or null pointer)
```

## Operator Precedence

From highest to lowest precedence:

1. **Primary**: `()` `[]` `.` function call, postfix `#`
2. **Unary**: `-` `not` `!b` `#` `µ`
3. **Power**: `**`
4. **Multiplicative**: `*` `/` `%`
5. **Additive**: `+` `-`
6. **Shift**: `<<b` `>>b` `<<<b` `>>>b`
7. **Bitwise AND**: `&b`
8. **Bitwise XOR**: `^b`
9. **Bitwise OR**: `|b`
10. **Comparison**: `==` `!=` `<` `<=` `>` `>=`
11. **Logical AND**: `and`
12. **Logical OR**: `or`
13. **Or-bang**: `or!`
14. **Function Composition**: `<>`
15. **Send**: `<-`
16. **Receive**: `<=`
17. **Pipe**: `|` `||`
18. **Match**: `{ }` (postfix)
19. **Assignment**: `=` `:=` `<-` `+=` `-=` `*=` `/=` `%=` `**=`

**Associativity:**
- Left-associative: All binary operators except `**` and assignments
- Right-associative: `**`, all assignments
- Non-associative: Comparison operators (can't chain)

## Parsing Rules

### Minimal Parentheses Philosophy

C67 minimizes parenthesis usage. Use parentheses only when:

1. **Precedence override needed:**
   ```c67
   (x + y) * z      // Override precedence
   ```

2. **Complex condition grouping:**
   ```c67
   (x > 0 && y < 10) { ... }  // Group condition
   ```

3. **Multiple lambda parameters:**
   ```c67
   (x, y) -> x + y  // Multiple params
   ```

**Not needed:**
```c67
// Good: no unnecessary parens
x > 0 { => "positive" ~> "negative" }
result = x + y * z
classify = x -> x { 0 => "zero" ~> "other" }

// Bad: unnecessary parens
result = x > 0 { => ("positive") ~> ("negative") }
compute = (x) -> (x * 2)
```

### Statement Termination

Statements are terminated by newlines:

```c67
x = 10
y = 20
z = x + y
```

Multiple statements on one line require explicit semicolons:

```c67
x = 10; y = 20; z = x + y
```

### Whitespace Rules

- **Significant newlines**: End statements
- **Insignificant whitespace**: Spaces, tabs (except in strings)
- **Indentation**: Not significant (unlike Python)

### Edge Cases

#### Pipe vs Guard

The `|` character is context-dependent:

```c67
// Pipe operator (| not at line start)
result = data | transform | filter

// Guard marker (| at line start)
classify = x -> {
    | x > 0 => "positive"
    | x < 0 => "negative"
    ~> "zero"
}
```

**Rule:** `|` at the start of a line/clause (after `{` or newline) is a guard marker. Otherwise it's the pipe operator.

#### Arrow Disambiguation

```c67
=>   Match arm result
~>   Default match arm
->   Lambda or receive
```

Context determines meaning:

```c67
f = x -> x + 1             // Lambda with one arg
msg <= &8080               // Receive from channel
x { 0 => "zero" }          // Match arm
x { ~> "default" }         // Default arm
greet = { println("Hi") }  // No-arg lambda
```

#### No-Argument Lambdas

```c67
// Inferred lambda (in assignment context):
greet = { println("Hello!") }            // Inferred: greet = -> { println("Hello!") }
worker = { @ { process_forever() } }     // Inferred: worker = -> { @ { process_forever() } }

// Explicit no-argument lambda:
greet = -> println("Hello!")             // Explicit ->
handler = -> process_events()            // Explicit ->

// With block body:
worker = {                               // Inferred lambda
    @ { process_forever() }
}

// Common use cases:
init = { setup_resources() }             // Inferred (assignment context)
cleanup = { release_all() }              // Inferred (assignment context)
background = { @ { poll_events() } }     // Inferred (assignment context)

// When explicit -> is needed:
callbacks = [-> print("A"), -> print("B")]  // Not in assignment, need explicit ->
process(-> get_data())                      // Function argument, need explicit ->
```

#### Loop Forms

The `@` symbol introduces loops (one of three forms):

```c67
@ { ... }                  // Infinite loop
@ i in collection { ... }  // For-each loop
@ condition { ... }        // While loop
```

**Loop Control with `ret @` and Numbered Labels:**

Instead of `break`/`continue` keywords, C67 uses `ret @` with automatically numbered loop labels.

**Loop Numbering:** Loops are numbered from outermost to innermost:
- `@1` = outermost loop
- `@2` = second level (nested inside @1)
- `@3` = third level (nested inside @2)
- `@` = current/innermost loop

```c67
// Exit current loop
@ i in 0..<100 {
    i > 50 { ret @ }      // Exit current loop (same as ret @1 here)
    i == 42 { ret @ 42 }  // Exit loop with value 42
    println(i)
}

// Nested loops with numbered labels
@ i in 0..<10 {           // Loop @1 (outermost)
    @ j in 0..<10 {       // Loop @2 (inner)
        j == 5 { ret @ }         // Exit loop @2 (innermost)
        i == 5 { ret @1 }        // Exit loop @1 (outer)
        i == 3 and j == 7 { ret @1 42 }  // Exit loop @1 with value
        println(i, j)
    }
}

// ret without @ returns from function (not loop)
compute = n -> {
    @ i in 0..<100 {
        i == n { ret i }  // Return from function
        i == 50 { ret @ } // Exit loop only, continue function
    }
    ret 0
}
```

**Loop `max` Keyword:**

Loops with unknown bounds or modified counters require `max`:

```c67
// Counter modified - needs max
@ i in 0..<10 max 20 {
    i++  // Modified counter
}

// Unknown iterations - needs max
@ msg in read_channel() max inf {
    process(msg)
}
```

#### Defer Statement

The `defer` keyword schedules an expression to execute when the current scope exits (function return, block exit, or error). Deferred expressions execute in **LIFO (Last In, First Out)** order.

**Syntax:**
```ebnf
defer_statement = "defer" expression ;
```

**Examples:**
```c67
// Resource cleanup with defer
init_resources = () -> {
    file := open("data.txt") or! {
        println("Failed to open file")
        ret 0
    }
    defer close(file)  // Always closes when function returns

    buffer := c_malloc(1024) or! {
        println("Out of memory")
        ret 0
    }
    defer c_free(buffer)  // Frees before file closes (LIFO)

    process(file, buffer)
    ret 1
}

// C FFI with defer (SDL3 example)
sdl.SDL_Init(sdl.SDL_INIT_VIDEO) or! {
    println("SDL init failed")
    ret 1
}
defer sdl.SDL_Quit()  // Always called on return

window := sdl.SDL_CreateWindow("Title", 640, 480, 0) or! {
    println("Window creation failed")
    ret 1  // SDL_Quit still called via defer
}
defer sdl.SDL_DestroyWindow(window)  // Executes before SDL_Quit

// More resources...
```

**Execution Order:**
Deferred calls execute in reverse order of declaration (LIFO):
```c67
defer println("1")  // Executes third
defer println("2")  // Executes second
defer println("3")  // Executes first
// Output: 3, 2, 1
```

**When Defer Executes:**
- On function return (`ret`)
- On block exit (normal completion)
- On early return from error handling
- On loop exit with `ret @`

**Best Practices:**
1. Use `defer` immediately after resource acquisition
2. Combine with `or!` for railway-oriented error handling
3. Rely on LIFO order for proper cleanup sequence
4. Use `defer` for C FFI resources (files, sockets, SDL objects)
5. Return from error blocks instead of `exit()` - defer ensures cleanup

**Common Pattern:**
```c67
// Railway-oriented with defer
resource := acquire() or! {
    println("Acquisition failed")
    ret error("acq")
}
defer cleanup(resource)

// Work with resource...
// cleanup always happens, even on error
```

#### Address Operator

The `&` symbol creates ENet addresses (network endpoints):

```c67
&8080                      // Port only: & followed by digits
&localhost:8080            // Host:port: & followed by identifier/IP + :
&192.168.1.1:3000          // IP:port
```

**Examples:**
```c67
// Loops (statement context)
@ { println("Forever") }           // Infinite loop
@ i in [1, 2, 3] { println(i) }    // For-each loop
@ x < 10 { x = x + 1 }             // While loop

// Addresses (expression context)
server = @8080                      // Address literal
client = &localhost:9000            // Address with hostname
remote = &192.168.1.100:3000        // Address with IP

// Unambiguous in context
listen(&8080)                       // Function call with address
@ x > 0 { send(&8080, data) }      // Loop with address inside
```

#### Block vs Map vs Match

Disambiguated by contents (see Block Disambiguation Rules above):

```c67
{ x: 10 }                // Map: contains :
x { 0 -> "zero" }        // Match: contains ->
{ temp = x * 2; temp }   // Statement block: no : or ->
```

## Error Handling and Result Types

C67 uses a **Result type** for operations that can fail. A Result is still `map[uint64]float64`, but with special semantic meaning tracked by the compiler.

### Result Type Design

A Result is encoded as follows:

**Byte Layout:**
```
[type_byte][length][key][value][key][value]...[0x00]
```

**Type Bytes:**
```
0x01 - C67 Number (success)
0x02 - C67 String (success)
0x03 - C67 List (success)
0x04 - C67 Map (success)
0x05 - C67 Address (success)
0xE0 - Error (failure, followed by 4-char error code)
0x10 - C int8
0x11 - C int16
0x12 - C int32
0x13 - C int64
0x14 - C uint8
0x15 - C uint16
0x16 - C uint32
0x17 - C uint64
0x18 - C float32
0x19 - C float64
0x1A - C pointer
0x1B - C string pointer
```

**Success case:**
- Type byte indicates the C67 or C type
- Length field (uint64) indicates number of key-value pairs
- Key-value pairs follow (each pair is uint64 key, float64 value)
- Terminated with 0x00 byte

**Error case:**
- Type byte is 0xE0
- Followed by 4-byte error code (ASCII, space-padded)
- Terminated with 0x00 byte

### Standard Error Codes

```
"dv0 " - Division by zero
"idx " - Index out of bounds
"key " - Key not found
"typ " - Type mismatch
"nil " - Null pointer
"mem " - Out of memory
"arg " - Invalid argument
"io  " - I/O error
"net " - Network error
"prs " - Parse error
"ovf " - Overflow
"udf " - Undefined
```

**Note:** Error codes are 4 bytes, space-padded if shorter. The `.error` accessor strips trailing spaces on access.

### The `.error` Accessor

Every value has a `.error` accessor that:
- Returns `""` (empty string) for success values
- Returns the error code string (spaces stripped) for error values

```c67
x = 10 / 2              // Success: returns 5.0
x.error                 // Returns "" (empty)

y = 10 / 0              // Error: division by zero
y.error                 // Returns "dv0" (spaces stripped)

// Typical usage
result.error {
    "" => proceed(result)
    ~> handle_error(result.error)
}
```

### The `or!` Operator

The `or!` operator provides a default value or executes a block when the left side is an error or null:

```c67
// Handle errors
x = 10 / 0              // Error result
safe = x or! 99         // Returns 99 (error case)

y = 10 / 2              // Success result (value 5)
safe2 = y or! 99        // Returns 5 (success case)

// Handle null pointers from C FFI
window := sdl.SDL_CreateWindow("Title", 640, 480, 0) or! {
    println("Failed to create window!")
    sdl.SDL_Quit()
    exit(1)
}

// Inline null check with default
ptr := c_malloc(1024) or! 0  // Returns 0 if allocation failed
```

**Semantics:**
1. Evaluate left operand
2. Check if NaN (error value) OR if value equals 0.0 (null pointer)
3. If NaN or null:
   - And right side is a block: execute block, result in xmm0
   - And right side is an expression: evaluate right side, result in xmm0
4. Otherwise (value is valid): keep left operand value in xmm0
5. Right side is NOT evaluated unless left is NaN/null (lazy/short-circuit evaluation)

**Error Checking:**
- **NaN check**: Compares value with itself using UCOMISD (NaN != NaN)
- **Null check**: Compares value with 0.0 using UCOMISD

**When checking for null (C FFI pointers):**
- All values are float64, so pointer 0 is encoded as 0.0
- `or!` treats 0.0 (null pointer) as a failure case
- Enables railway-oriented programming for C interop
- Works with any C function that returns pointers

**Precedence:** Lower than logical OR, higher than send operator

### Error Propagation Patterns

```c67
// Check and early return
process = input -> {
    step1 = validate(input)
    step1.error { != "" => step1 }  // Return error

    step2 = transform(step1)
    step2.error { != "" => step2 }

    finalize(step2)
}

// Default values with or!
compute = input -> {
    x = parse(input) or! 0
    y = divide(100, x) or! -1
    y * 2
}

// Match on error code
result = risky()
result.error {
    "" => println("Success:", result)
    "dv0" => println("Division by zero")
    "mem" => println("Out of memory")
    ~> println("Unknown error:", result.error)
}
```

### Creating Custom Errors

Use the `error` function to create error Results:

```c67
// Create error with code
err = error("arg")  // Type byte 0xE0 + "arg "

// Or use division by zero for runtime errors
fail = 0 / 0        // Returns error "dv0"
```

### Compiler Type Tracking

The compiler tracks whether a value is a Result type:

```c67
// Compiler knows this returns Result
divide = (a, b) -> {
    b == 0 { ret error("dv0") }
    a / b
}

// Compiler propagates Result type
compute = x -> {
    y = divide(100, x)  // y has Result type
    y or! 0             // Handles potential error
}
```

See [TYPE_TRACKING.md](TYPE_TRACKING.md) for implementation details.

### Result Type Memory Layout

**Success value (number 42):**
```
Bytes: 01 01 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 40 45 00 00 00 00 00 00 00 00
       ↑  ↑----- length=1 ----↑  ↑------- key=0 -------↑  ↑------- value=42.0 ------↑  ↑ term
       type=01 (number)
```

**Error value (division by zero):**
```
Bytes: E0 64 76 30 20 00
       ↑  ↑----- error code "dv0 " -----↑  ↑ term
       type=E0 (error)
```

### `.error` Implementation

The `.error` accessor:
1. Checks type byte (first byte)
2. If 0xE0: extract next 4 bytes as error code string
3. Strip trailing spaces
4. Return error code string
5. Otherwise: return empty string ""

### `or!` Implementation

The `or!` operator:
1. Evaluates left operand
2. Checks type byte
3. If 0xE0: returns right operand
4. Otherwise: returns left operand value (strips type metadata)

## Classes and Object-Oriented Programming

C67 supports classes as syntactic sugar over maps and closures, providing a familiar OOP interface while maintaining the language's fundamental simplicity.

### Core Principles

- **Maps as objects:** Objects are `map[uint64]float64` with conventions
- **Closures as methods:** Methods are lambdas that close over instance data
- **Composition over inheritance:** Use `<>` to compose with behavior maps
- **Dot notation:** `.field` inside methods for instance fields
- **Minimal syntax:** Only one new keyword (`class`)
- **Desugars to regular C67:** Classes compile to maps and lambdas
- **`this` keyword:** Reference to current instance

### Class Declaration

```c67
class Point {
    // Constructor (implicit)
    init = (x, y) -> {
        .x = x
        .y = y
    }

    // Instance methods
    distance = other -> {
        dx := other.x - .x
        dy := other.y - .y
        sqrt(dx * dx + dy * dy)
    }

    move = (dx, dy) -> {
        .x <- .x + dx
        .y <- .y + dy
    }
}

// Usage
p1 := Point(10, 20)
p2 := Point(30, 40)
dist := p1.distance(p2)
p1.move(5, 5)
```

### Desugaring

Classes desugar to regular C67 code:

```c67
// class Point { ... } becomes:
Point := (x, y) -> {
    instance := {}
    instance["x"] = x
    instance["y"] = y

    instance["distance"] = other -> {
        dx := other["x"] - instance["x"]
        dy := other["y"] - instance["y"]
        sqrt(dx * dx + dy * dy)
    }

    instance["move"] = (dx, dy) -> {
        instance["x"] <- instance["x"] + dx
        instance["y"] <- instance["y"] + dy
    }

    ret instance
}
```

### Instance Fields

Use `.field` inside class methods to access instance state:

```c67
class Counter {
    init = start -> {
        .count = start
        .history = []
    }

    increment = () -> {
        .count <- .count + 1
        .history <- .history :: .count
    }

    get = () -> .count
}

c := Counter(0)
c.increment()
println(c.get())  // 1
```

### Class Fields (Static Members)

Use `ClassName.field` for class-level state:

```c67
class Entity {
    Entity.count = 0
    Entity.all = []

    init = name -> {
        .name = name
        .id = Entity.count
        Entity.count <- Entity.count + 1
        Entity.all <- Entity.all :: instance
    }
}

e1 := Entity("Alice")
e2 := Entity("Bob")
println(Entity.count)  // 2
```

### Composition with `<>`

Extend classes with behavior maps using `<>`:

```c67
Serializable = {
    to_json: {
        // Convert instance to JSON
    },
    from_json: json {
        // Parse JSON to instance
    }
}

class Point <> Serializable {
    init = (x, y) -> {
        .x = x
        .y = y
    }
}

p := Point(10, 20)
json := p.to_json()
```

**Multiple composition** - chain `<>` operators:

```c67
class User {
    <> Serializable
    <> Validatable
    <> Timestamped
    init = name -> {
        .name = name
        .created_at = now()
    }
}
```

### Instance Field Resolution

Inside class methods:
- `.field` → instance field access
- `ClassName.field` → class field access
- `other.field` → other instance field access

```c67
class Point {
    Point.origin = nil  // Class field

    init = (x, y) -> {
        .x = x           // Instance field (this instance)
        .y = y
    }

    distance_to_origin = -> {
        .distance(Point.origin)  // Class field access
    }

    distance = other -> {
        dx := other.x - .x       // Other instance field vs this instance field
        dy := other.y - .y
        sqrt(dx * dx + dy * dy)
    }
}

Point.origin = Point(0, 0)  // Initialize class field
```

### Private Methods Convention

Use underscore prefix for "private" methods (by convention):

```c67
class Account {
    init = balance -> {
        .balance = balance
    }

    _validate = amount -> {
        amount > 0 && amount <- .balance
    }

    withdraw = amount -> {
        ._ validate(amount) {
            .balance -= - amount
            ret 0
        }
        ret -1  // Error
    }
}
```

### Integration with CStruct

Combine classes with CStruct for performance:

```c67
cstruct Vec2Data {
    x as float64,
    y as float64
}

class Vec2 {
    init = (x, y) -> {
        .data = call("malloc", Vec2Data.size as uint64)
        unsafe float64 {
            rax <- .data as cptr
            [rax] <- x
            [rax + 8] <- y
        }
    }

    magnitude = () -> {
        unsafe float64 {
            rax <- .data as cptr
            xmm0 <- [rax]
            xmm1 <- [rax + 8]
            xmm0 <- xmm0 * xmm0
            xmm1 <- xmm1 * xmm1
            xmm0 <- xmm0 + xmm1
        } | result -> sqrt(result)
    }
}
```

### Operator Overloading via Methods

While C67 doesn't have operator overloading syntax, you can define methods with operator-like names:

```c67
class Complex {
    init = (real, imag) -> {
        .real = real
        .imag = imag
    }

    add = other -> Complex(.real + other.real, .imag + other.imag)
    mul = other -> Complex(
        .real * other.real - .imag * other.imag,
        .real * other.imag + .imag * other.real
    )
}

a := Complex(1, 2)
b := Complex(3, 4)
c := a.add(b)
```

### The `<>` Operator

The `<>` operator merges behavior maps into the class:

```ebnf
class_decl      = "class" identifier { "<>" identifier } "{" { class_member } "}" ;
```

Semantically:

```c67
class Point {
    <> Serializable
    <> Validatable
    // members
}

// Desugars to:
Point = (...) -> {
    instance := {}
    // Merge Serializable methods
    @ key in Serializable { instance[key] <- Serializable[key] }
    // Merge Validatable methods
    @ key in Validatable { instance[key] <- Validatable[key] }
    // Add Point-specific members
    // ...
    ret instance
}
```

### Method Chaining

Methods that return `. ` (this) enable chaining:

```c67
class Builder {
    init = () -> {
        .parts = []
    }

    add = part -> {
        .parts <- .parts :: part
        ret .  // Return this (self)
    }

    build = () -> .parts
}

result = Builder().add("A").add("B").add("C").build()
```

### No Inheritance

C67 deliberately avoids inheritance hierarchies. Use composition:

```c67
// Instead of inheritance
Drawable = {
    draw: { println("Drawing...") }
}

Movable := {
    move: (dx, dy) -> {
        .x <- .x + dx
        .y <- .y + dy
    }
}

class {
    <> Sprite
    <> Drawable
    <> Movable
    init = (x, y) -> {
        .x = x
        .y = y
    }
}
```

## Parsing Algorithm

### High-Level Flow

```
1. Tokenize (lexer.go)
   Source → Tokens

2. Parse (parser.go)
   Tokens → AST

3. Type Inference (optional, see TYPE_TRACKING.md)
   AST → AST with type annotations

4. Code Generation (x86_64_codegen.go, arm64_codegen.go, riscv64_codegen.go)
   AST → Machine code

5. Linking (elf.go, macho.go)
   Machine code → Executable
```

### Parser Implementation Notes

**Recursive Descent:**
- Hand-written recursive descent parser
- Operator precedence climbing for expressions
- Look-ahead for block disambiguation

**Error Recovery:**
- Continue parsing after errors when possible
- Collect multiple errors per pass
- Provide helpful error messages with line numbers

**Performance:**
- Single-pass parsing (no separate AST transformation)
- Minimal memory allocation
- Fast compilation (typically <100ms for small programs)

## Implementation Guidelines

**Memory Management:**
- **ALWAYS use arena allocation** instead of malloc/free when possible
- The arena allocator (`c67_arena_alloc`) provides fast bump allocation with automatic growth
- Arena memory is freed in bulk, avoiding fragmentation
- Only use malloc for external C library compatibility

**Register Management:**
- The compiler has a sophisticated register allocator (`RegisterAllocator` in register_allocator.go)
- Real-time register tracking via `RegisterTracker` (register_tracker.go)
- Register spilling when needed via `RegisterSpiller` (register_allocator.go)
- Use these systems instead of ad-hoc register assignment

**Code Generation:**
- Target-independent IR through `Out` abstraction layer
- Backend-specific optimizations in arm64_backend.go, riscv64_backend.go, x86_64_codegen.go
- SIMD operations for parallel loops (AVX-512 on x86_64)

## Type Annotations

Type annotations are **optional metadata** that specify semantic intent and guide FFI marshalling. They do NOT change the runtime representation (always `map[uint64]float64`).

### Syntax

**Variable declarations:**
```c67
x: num = 42                    // Number annotation
name: str = "Alice"            // String annotation
items: list = [1, 2, 3]        // List annotation
config: map = {port: 8080}     // Map annotation

// C types for FFI
ptr: cptr = sdl.SDL_CreateWindow("Hi", 640, 480, 0)
err: cstring = sdl.SDL_GetError()
result: cint = sdl.SDL_Init(sdl.SDL_INIT_VIDEO)
value: cdouble = 3.14159
```

**Function signatures:**
```c67
// Parameter and return types
add(x: num, y: num) -> num { x + y }

// String functions
greet(name: str) -> str { f"Hello, {name}!" }

// C FFI functions
create_window(title: str, w: cint, h: cint) -> cptr {
    sdl.SDL_CreateWindow(title, w, h, 0)
}

// Mixed types
format_error(code: cint) -> str {
    f"Error {code}: {sdl.SDL_GetError()}"
}
```

### Type Semantics

| Type      | Runtime Repr          | Purpose       | Example               |
|-----------|-----------------------|---------------|-----------------------|
| `num`     | `{0: 42.0}`           | Number intent | `x: num = 42`         |
| `str`     | `{0: 72.0, 1: 105.0}` | String intent | `name: str = "Hi"`    |
| `list`    | `{0: 1.0, 1: 2.0}`    | List intent   | `xs: list = [1, 2]`   |
| `map`     | `{hash("x"): 10.0}`   | Map intent    | `m: map = {x: 10}`    |
| `cstring` | `{0: <ptr>}`          | C `char*`     | `s: cstring = c.fn()` |
| `cptr`    | `{0: <ptr>}`          | C pointer     | `p: cptr = sdl.fn()`  |
| `cint`    | `{0: 42.0}`           | C `int`       | `n: cint = sdl.fn()`  |
| `clong`   | `{0: 42.0}`           | C `int64_t`   | `l: clong = c.time()` |
| `cfloat`  | `{0: 3.14}`           | C `float`     | `f: cfloat = 3.14`    |
| `cdouble` | `{0: 3.14}`           | C `double`    | `d: cdouble = c.fn()` |
| `cbool`   | `{0: 1.0}`            | C `bool`      | `ok: cbool = c.fn()`  |

### FFI Marshalling

Type annotations guide automatic conversions at C FFI boundaries:

**C67 → C conversions:**
```c67
// C67 string → C string (calls c67_string_to_cstr)
title: str = "Window"
window = sdl.SDL_CreateWindow(title, 640, 480, 0)  // title converted to char*

// C67 number → C int (extracts {0: value})
result: cint = sdl.SDL_Init(0x00000020)  // C67 num → C int
```

**C → C67 conversions:**
```c67
// C char* → cstring (stored as pointer in {0: <ptr>})
err: cstring = sdl.SDL_GetError()  // char* stored as-is

// When needed, convert cstring → str manually
err_str: str = str(err)  // Convert C string to C67 string
```

### Type Inference

When annotations are omitted, the compiler infers types:

```c67
x = 42              // Inferred: num
name = "Alice"      // Inferred: str
items = [1, 2, 3]   // Inferred: list
ptr = sdl.SDL_CreateWindow(...)  // Inferred: cptr (from FFI signature)
```

### When to Use Type Annotations

**Use annotations when:**
1. Clarifying intent (documentation)
2. Working with C FFI (marshalling guidance)
3. Catching type errors early
4. Enabling future optimizations

**Omit annotations when:**
1. Type is obvious from context
2. Writing quick scripts
3. Type doesn't matter for correctness

---

**Note:** This grammar is the canonical reference for C67 3.0. The compiler implementation (lexer.go, parser.go) must match this specification exactly.

**See also:**
- [LANGUAGESPEC.md](LANGUAGESPEC.md) - Complete language semantics
- [LIBERTIES.md](LIBERTIES.md) - Documentation accuracy guidelines
