// Completion: 98% - All AST nodes implemented, comprehensive coverage
package main

import (
	"fmt"
	"strings"
)

// AST Nodes
type Node interface {
	String() string
}

type Program struct {
	Statements         []Statement
	ExportMode         string                  // "*" for export all without prefix, "" for require prefix
	ExportedFuncs      []string                // Specific functions to export (only if ExportMode is not "*")
	FunctionNamespaces map[string]string       // function name -> namespace (for imports)
	CStructs           map[string]*CStructDecl // cstruct name -> declaration
}

func (p *Program) String() string {
	var out strings.Builder
	for _, stmt := range p.Statements {
		out.WriteString(stmt.String())
		out.WriteString("\n")
	}
	return out.String()
}

type Statement interface {
	Node
	statementNode()
}

type AssignStmt struct {
	Name           string
	Value          Expression
	Mutable        bool        // true for := or <-, false for =
	IsUpdate       bool        // true for <-, false for = and :=
	IsReuseMutable bool        // true when = is used to update existing mutable variable
	Precision      string      // Legacy type annotation: "b64", "f32", etc. (empty if none)
	TypeAnnotation *Vibe67Type // Type annotation: num, str, cstring, cptr, etc. (nil if none)
}

type MultipleAssignStmt struct {
	Names    []string   // Variable names (left side)
	Value    Expression // Expression that should evaluate to a list (right side)
	Mutable  bool       // true for := or <-, false for =
	IsUpdate bool       // true for <-, false for = and :=
}

func (m *MultipleAssignStmt) String() string {
	op := "="
	if m.IsUpdate {
		op = "<-"
	} else if m.Mutable {
		op = ":="
	}
	names := strings.Join(m.Names, ", ")
	return names + " " + op + " " + m.Value.String()
}

func (m *MultipleAssignStmt) statementNode() {}

func (a *AssignStmt) String() string {
	op := "="
	if a.IsUpdate {
		op = "<-"
	} else if a.Mutable {
		op = ":="
	}
	result := a.Name
	if a.Precision != "" {
		result += ":" + a.Precision
	}
	if a.Value == nil {
		return result + " " + op + " <nil>"
	}
	return result + " " + op + " " + a.Value.String()
}
func (a *AssignStmt) statementNode() {}

type MapUpdateStmt struct {
	MapName string     // Name of the map/list variable
	Index   Expression // Index expression
	Value   Expression // New value
}

func (m *MapUpdateStmt) String() string {
	return fmt.Sprintf("%s[%s] <- %s", m.MapName, m.Index.String(), m.Value.String())
}
func (m *MapUpdateStmt) statementNode() {}

type UseStmt struct {
	Path string // Import path: "./file.vibe67" or "package_name"
}

func (u *UseStmt) String() string { return "use " + u.Path }
func (u *UseStmt) statementNode() {}

type ExportStmt struct {
	Mode      string   // "*" for export all, "" for export specific functions
	Functions []string // Function names to export (only if Mode != "*")
}

func (e *ExportStmt) String() string {
	if e.Mode == "*" {
		return "export *"
	}
	return "export " + strings.Join(e.Functions, " ")
}
func (e *ExportStmt) statementNode() {}

type ImportStmt struct {
	URL     string // Git URL: "github.com/owner/repo"
	Version string // Git ref: "v1.0.0", "HEAD", "latest", "commit-hash", or "" for latest
	Alias   string // Namespace alias: "xmath" or "*" for wildcard
}

func (i *ImportStmt) String() string {
	url := i.URL
	if i.Version != "" {
		url += "@" + i.Version
	}
	return "import " + url + " as " + i.Alias
}
func (i *ImportStmt) statementNode() {}

type CImportStmt struct {
	Library string // C library name: "sdl3", "raylib", "sqlite3", or .so filename: "libmylib.so"
	Alias   string // Namespace alias: "sdl", "rl", "sql"
	SoPath  string // Optional: full path to .so file for custom libraries (e.g., "/tmp/libmylib.so")
}

func (c *CImportStmt) String() string {
	if c.SoPath != "" {
		return "import \"" + c.SoPath + "\" as " + c.Alias
	}
	return "import " + c.Library + " from C as " + c.Alias
}
func (c *CImportStmt) statementNode() {}

// CStructField represents a field in a C struct
type CStructField struct {
	Name   string // Field name
	Type   string // C type (i8, i16, i32, i64, u8, u16, u32, u64, f32, f64, cstr, ptr)
	Offset int    // Byte offset from struct start (calculated)
	Size   int    // Size in bytes (calculated)
}

// CStructDecl represents a C-compatible struct definition
type CStructDecl struct {
	Name   string         // Struct name
	Fields []CStructField // Struct fields
	Packed bool           // true if #[packed] - no padding
	Align  int            // Custom alignment (0 = natural alignment)
	Size   int            // Total struct size in bytes (calculated)
}

func (c *CStructDecl) String() string {
	return fmt.Sprintf("cstruct %s { ... }", c.Name)
}
func (c *CStructDecl) statementNode() {}

// GetCTypeSize returns the size in bytes for a C type string
func GetCTypeSize(ctype string) int {
	switch ctype {
	case "int8", "uint8":
		return 1
	case "int16", "uint16":
		return 2
	case "int32", "uint32", "float32":
		return 4
	case "int64", "uint64", "float64", "ptr", "cstr":
		return 8
	default:
		return 0 // Unknown type
	}
}

// GetCTypeAlignment returns the natural alignment in bytes for a C type string
func GetCTypeAlignment(ctype string) int {
	// Natural alignment is the same as size for primitives
	return GetCTypeSize(ctype)
}

// CalculateStructLayout calculates field offsets and total size for a C struct
// Returns the total size of the struct
func (c *CStructDecl) CalculateStructLayout() {
	if len(c.Fields) == 0 {
		c.Size = 0
		return
	}

	currentOffset := 0
	maxAlign := 1

	for i := range c.Fields {
		field := &c.Fields[i]
		field.Size = GetCTypeSize(field.Type)

		if field.Size == 0 {
			// Unknown type - this will be caught during compilation
			continue
		}

		align := GetCTypeAlignment(field.Type)
		if c.Packed {
			// Packed: no padding between fields
			field.Offset = currentOffset
		} else {
			// Add padding to align field
			padding := (align - (currentOffset % align)) % align
			field.Offset = currentOffset + padding
		}

		currentOffset = field.Offset + field.Size

		// Track maximum alignment for struct alignment
		if align > maxAlign {
			maxAlign = align
		}
	}

	// Custom alignment override
	if c.Align > 0 {
		maxAlign = c.Align
	}

	// Add padding at end to align to struct's alignment
	// If packed, only add padding if custom alignment is specified
	// If not packed, always add padding to natural alignment
	if maxAlign > 0 && (!c.Packed || c.Align > 0) {
		padding := (maxAlign - (currentOffset % maxAlign)) % maxAlign
		currentOffset += padding
	}

	c.Size = currentOffset
}

// ClassDecl represents a class definition (to be desugared to maps and closures)
type ClassDecl struct {
	Name         string                 // Class name
	ClassVars    map[string]Expression  // Class-level variables (ClassName.var)
	Methods      map[string]*LambdaExpr // Methods (instance functions)
	Compositions []string               // Names of behavior maps to compose with <>
}

func (c *ClassDecl) String() string {
	return fmt.Sprintf("class %s { ... }", c.Name)
}
func (c *ClassDecl) statementNode() {}

type ExpressionStmt struct {
	Expr Expression
}

func (e *ExpressionStmt) String() string { return e.Expr.String() }
func (e *ExpressionStmt) statementNode() {}

type LoopStmt struct {
	// No explicit label - determined by nesting depth when created with @
	Iterator      string     // Variable name (e.g., "i")
	Iterable      Expression // Expression to iterate over (e.g., range(10))
	Body          []Statement
	MaxIterations int64       // Maximum allowed iterations (math.MaxInt64 for infinite)
	NeedsMaxCheck bool        // Whether to emit runtime max iteration checking
	BaseOffset    int         // Stack offset before loop body (set during collectSymbols)
	NumThreads    int         // Number of threads for parallel execution (0 = sequential, -1 = all cores, N = specific count)
	Reducer       *LambdaExpr // Optional reduction lambda for parallel loops: | a,b | { a + b }
	Vectorized    bool        // Whether this loop has been marked for SIMD vectorization
	VectorWidth   int         // Elements per SIMD vector (e.g., 4 for AVX doubles, 8 for AVX floats)
}

type WhileStmt struct {
	Condition     Expression  // Condition expression (e.g., n < 5)
	Body          []Statement // Body statements to execute while condition is true
	MaxIterations int64       // Maximum allowed iterations (required for condition loops)
	BaseOffset    int         // Stack offset before loop body
	NumThreads    int         // Number of threads for parallel execution (0 = sequential)
}

func (w *WhileStmt) String() string {
	return fmt.Sprintf("@ %s max %d { ... }", w.Condition.String(), w.MaxIterations)
}

func (w *WhileStmt) statementNode() {}

type ReceiveLoopStmt struct {
	MessageVar string      // Variable name for received message (e.g., "msg")
	SenderVar  string      // Variable name for sender address (e.g., "from")
	Address    Expression  // Address expression to bind to (e.g., ":5000")
	Body       []Statement // Body statements to execute for each message
	BaseOffset int         // Stack offset before loop body
}

func (r *ReceiveLoopStmt) String() string {
	return fmt.Sprintf("@ %s, %s in %s { ... }", r.MessageVar, r.SenderVar, r.Address.String())
}

func (r *ReceiveLoopStmt) statementNode() {}

type LoopExpr struct {
	// No explicit label - determined by nesting depth when created with @
	Iterator      string      // Variable name (e.g., "i")
	Iterable      Expression  // Expression to iterate over (e.g., range(10))
	Body          []Statement // Body statements
	MaxIterations int64       // Maximum allowed iterations (math.MaxInt64 for infinite)
	NeedsMaxCheck bool        // Whether to emit runtime max iteration checking
	NumThreads    int         // Number of threads for parallel execution (0 = sequential, -1 = all cores, N = specific count)
	Reducer       *LambdaExpr // Optional reduction lambda for parallel loops: | a,b | { a + b }
}

func (l *LoopExpr) String() string {
	return fmt.Sprintf("@ %s in %s { ... }", l.Iterator, l.Iterable.String())
}
func (l *LoopExpr) expressionNode() {}

func (l *LoopStmt) String() string {
	var out strings.Builder
	// Show parallel prefix if NumThreads is set
	if l.NumThreads == -1 {
		out.WriteString("@@ ")
	} else if l.NumThreads > 0 {
		out.WriteString(fmt.Sprintf("%d @ ", l.NumThreads))
	} else {
		out.WriteString("@ ")
	}
	out.WriteString(l.Iterator)
	out.WriteString(" in ")
	out.WriteString(l.Iterable.String())
	out.WriteString(" {\n")
	for _, stmt := range l.Body {
		out.WriteString("  ")
		out.WriteString(stmt.String())
		out.WriteString("\n")
	}
	out.WriteString("}")
	return out.String()
}
func (l *LoopStmt) statementNode() {}

// JumpStmt represents a ret statement or loop continue
// ret (Label=0) = return from function
// ret @N (Label=N) = exit loop N and all inner loops
// @N (without ret) = continue loop N (IsBreak=false)
type JumpStmt struct {
	IsBreak bool       // true for ret (return/exit loop), false for continue (@N without ret)
	Label   int        // 0 for function return, N for loop label
	Value   Expression // Optional value to return
}

func (j *JumpStmt) String() string {
	keyword := "@"
	if j.IsBreak {
		keyword = "ret"
	}

	if j.Label > 0 {
		if j.Value != nil {
			return fmt.Sprintf("%s @%d %s", keyword, j.Label, j.Value.String())
		}
		return fmt.Sprintf("%s @%d", keyword, j.Label)
	}

	if j.Value != nil {
		return fmt.Sprintf("%s %s", keyword, j.Value.String())
	}
	return keyword
}
func (j *JumpStmt) statementNode() {}

type Expression interface {
	Node
	expressionNode()
}

type NumberExpr struct {
	Value float64
}

func (n *NumberExpr) String() string  { return fmt.Sprintf("%g", n.Value) }
func (n *NumberExpr) expressionNode() {}

type RandomExpr struct {
	// Represents the ?? operator - secure random float64 in [0.0, 1.0) using getrandom
}

func (r *RandomExpr) String() string  { return "??" }
func (r *RandomExpr) expressionNode() {}

type StringExpr struct {
	Value string
}

func (s *StringExpr) String() string  { return fmt.Sprintf("\"%s\"", s.Value) }
func (s *StringExpr) expressionNode() {}

// FStringExpr represents an f-string with interpolated expressions
// Parts alternates between string literals and expressions
// Example: f"Hello {name}" -> Parts = [StringExpr("Hello "), IdentExpr("name")]
type FStringExpr struct {
	Parts []Expression // Alternating string literals and expressions
}

func (f *FStringExpr) String() string  { return "f\"...\"" }
func (f *FStringExpr) expressionNode() {}

type AddressLiteralExpr struct {
	Value string // The full address like "&8080" or "&localhost:8080"
}

func (a *AddressLiteralExpr) String() string  { return a.Value }
func (a *AddressLiteralExpr) expressionNode() {}

type IdentExpr struct {
	Name string
}

func (i *IdentExpr) String() string  { return i.Name }
func (i *IdentExpr) expressionNode() {}

// NamespacedIdentExpr represents a namespaced identifier like sdl.SDL_INIT_VIDEO
type NamespacedIdentExpr struct {
	Namespace string // e.g., "sdl"
	Name      string // e.g., "SDL_INIT_VIDEO"
}

func (n *NamespacedIdentExpr) String() string  { return n.Namespace + "." + n.Name }
func (n *NamespacedIdentExpr) expressionNode() {}

// LoopStateExpr represents special loop variables: @first, @last, @counter, @i, @i1, @i2, etc.
type LoopStateExpr struct {
	Type      string // "first", "last", "counter", "i"
	LoopLevel int    // 0 for @i (current loop), 1 for @i1 (outermost), 2 for @i2, etc.
}

func (l *LoopStateExpr) String() string {
	return "@" + l.Type
}
func (l *LoopStateExpr) expressionNode() {}

// JumpExpr represents a label jump used as an expression (e.g., in match blocks)
type JumpExpr struct {
	Label   int        // Target label (0 = outer scope, N = loop label)
	Value   Expression // Optional value to return (for @0 value syntax)
	IsBreak bool       // true for ret @N (exit loop), false for @N (continue loop)
}

func (j *JumpExpr) String() string {
	prefix := "@"
	if j.IsBreak {
		prefix = "ret @"
	}
	if j.Value != nil {
		return fmt.Sprintf("%s%d %s", prefix, j.Label, j.Value.String())
	}
	return fmt.Sprintf("%s%d", prefix, j.Label)
}
func (j *JumpExpr) expressionNode() {}

type BinaryExpr struct {
	Left     Expression
	Operator string
	Right    Expression
}

func (b *BinaryExpr) String() string {
	return "(" + b.Left.String() + " " + b.Operator + " " + b.Right.String() + ")"
}
func (b *BinaryExpr) expressionNode() {}

// FMAExpr represents a Fused Multiply-Add pattern: a * b + c or a * b - c
// This is recognized by the optimizer and can be compiled to FMA instructions (VFMADD/VFMSUB)
type FMAExpr struct {
	A        Expression // First multiplicand
	B        Expression // Second multiplicand
	C        Expression // Addend/subtrahend
	IsSub    bool       // true for FMSUB (a * b - c), false for FMADD (a * b + c)
	IsNegMul bool       // true for FNMADD (-(a * b) + c), not currently detected
}

func (f *FMAExpr) String() string {
	op := "+"
	if f.IsSub {
		op = "-"
	}
	return fmt.Sprintf("fma(%s * %s %s %s)", f.A.String(), f.B.String(), op, f.C.String())
}
func (f *FMAExpr) expressionNode() {}

// UnaryExpr represents a unary operation: not, -, #, ++expr, --expr
type UnaryExpr struct {
	Operator string
	Operand  Expression
}

func (u *UnaryExpr) String() string {
	return "(" + u.Operator + u.Operand.String() + ")"
}
func (u *UnaryExpr) expressionNode() {}

// PostfixExpr: expr++, expr-- (increment/decrement after evaluation)
type PostfixExpr struct {
	Operator string // "++", "--"
	Operand  Expression
}

func (p *PostfixExpr) String() string {
	return "(" + p.Operand.String() + p.Operator + ")"
}
func (p *PostfixExpr) expressionNode() {}

// MoveExpr: expr! (move semantics - transfers ownership)
type MoveExpr struct {
	Expr Expression // The expression being moved (typically an IdentExpr)
}

func (m *MoveExpr) String() string {
	return m.Expr.String() + "!"
}
func (m *MoveExpr) expressionNode() {}

type InExpr struct {
	Value     Expression // Value to search for
	Container Expression // List or map to search in
}

func (i *InExpr) String() string {
	return "(" + i.Value.String() + " in " + i.Container.String() + ")"
}
func (i *InExpr) expressionNode() {}

type MatchClause struct {
	Guard        Expression
	Result       Expression
	IsValueMatch bool // True if this is a value match (0 -> ...), false if guard (| x > 0 -> ...)
}

type MatchExpr struct {
	Condition       Expression
	Clauses         []*MatchClause
	DefaultExpr     Expression
	DefaultExplicit bool
}

func (m *MatchExpr) String() string {
	var parts []string
	for _, clause := range m.Clauses {
		if clause.Guard != nil {
			if clause.Result != nil {
				parts = append(parts, clause.Guard.String()+" -> "+clause.Result.String())
			} else {
				parts = append(parts, clause.Guard.String()+" -> <statement>")
			}
		} else {
			if clause.Result != nil {
				parts = append(parts, "-> "+clause.Result.String())
			} else {
				parts = append(parts, "-> <statement>")
			}
		}
	}
	if m.DefaultExpr != nil && (m.DefaultExplicit || len(m.Clauses) == 0) {
		parts = append(parts, "~> "+m.DefaultExpr.String())
	}
	return m.Condition.String() + " { " + strings.Join(parts, " ") + " }"
}
func (m *MatchExpr) expressionNode() {}

type BlockExpr struct {
	Statements []Statement
}

func (b *BlockExpr) String() string {
	var parts []string
	for _, stmt := range b.Statements {
		parts = append(parts, stmt.String())
	}
	return "{ " + strings.Join(parts, "; ") + " }"
}
func (b *BlockExpr) expressionNode() {}

type CallExpr struct {
	Function            string
	Args                []Expression
	MaxRecursionDepth   int64 // Maximum recursion depth (math.MaxInt64 for infinite)
	NeedsRecursionCheck bool  // Whether to emit runtime recursion depth checking
	IsCFFI              bool  // Whether this is a C FFI call (c.malloc, c.free, etc.)
	RawBitcast          bool  // Whether to use raw bitcast for return value (call()! syntax)
}

func (c *CallExpr) String() string {
	args := make([]string, len(c.Args))
	for i, arg := range c.Args {
		if arg == nil {
			args[i] = "<nil>"
		} else {
			args[i] = arg.String()
		}
	}
	return c.Function + "(" + strings.Join(args, ", ") + ")"
}
func (c *CallExpr) expressionNode() {}

type DirectCallExpr struct {
	Callee Expression // The expression being called (e.g., a lambda)
	Args   []Expression
}

func (d *DirectCallExpr) String() string {
	args := make([]string, len(d.Args))
	for i, arg := range d.Args {
		args[i] = arg.String()
	}
	return "(" + d.Callee.String() + ")(" + strings.Join(args, ", ") + ")"
}
func (d *DirectCallExpr) expressionNode() {}

type ListExpr struct {
	Elements []Expression
}

func (l *ListExpr) String() string {
	elements := make([]string, len(l.Elements))
	for i, elem := range l.Elements {
		elements[i] = elem.String()
	}
	return "[" + strings.Join(elements, ", ") + "]"
}
func (l *ListExpr) expressionNode() {}

type MapExpr struct {
	Keys   []Expression
	Values []Expression
}

func (m *MapExpr) String() string {
	var pairs []string
	for i := range m.Keys {
		pairs = append(pairs, m.Keys[i].String()+": "+m.Values[i].String())
	}
	return "{" + strings.Join(pairs, ", ") + "}"
}
func (m *MapExpr) expressionNode() {}

type IndexExpr struct {
	List  Expression
	Index Expression
}

func (i *IndexExpr) String() string {
	if i.List == nil || i.Index == nil {
		return fmt.Sprintf("IndexExpr{List=%v, Index=%v}", i.List, i.Index)
	}
	return i.List.String() + "[" + i.Index.String() + "]"
}
func (i *IndexExpr) expressionNode() {}

// FieldAccessExpr: obj.field (for C structs with known layout)
// Different from IndexExpr which accesses map elements by key
// FieldAccessExpr accesses memory at a fixed offset
type FieldAccessExpr struct {
	Object     Expression // The struct/pointer expression
	FieldName  string     // Name of the field
	StructName string     // Name of the C struct type (if known)
	Offset     int        // Byte offset of field in struct (if known at parse time)
}

func (f *FieldAccessExpr) String() string {
	if f.Object == nil {
		return fmt.Sprintf("FieldAccessExpr{Object=nil, Field=%s}", f.FieldName)
	}
	return f.Object.String() + "." + f.FieldName
}
func (f *FieldAccessExpr) expressionNode() {}

// SliceExpr: list[start:end:step] or string[start:end:step] (Python-style slicing)
type SliceExpr struct {
	List  Expression
	Start Expression // nil means start from beginning
	End   Expression // nil means go to end
	Step  Expression // nil means step of 1, negative means reverse
}

func (s *SliceExpr) String() string {
	start := ""
	if s.Start != nil {
		start = s.Start.String()
	}
	end := ""
	if s.End != nil {
		end = s.End.String()
	}
	result := s.List.String() + "[" + start + ":" + end
	if s.Step != nil {
		result += ":" + s.Step.String()
	}
	result += "]"
	return result
}
func (s *SliceExpr) expressionNode() {}

// RangeExpr represents a range like 0..<10 or 0..=10
type RangeExpr struct {
	Start     Expression
	End       Expression
	Inclusive bool // true for ..=, false for ..<
}

func (r *RangeExpr) String() string {
	op := "..<"
	if r.Inclusive {
		op = "..="
	}
	return r.Start.String() + op + r.End.String()
}
func (r *RangeExpr) expressionNode() {}

type StructLiteralExpr struct {
	StructName string
	Fields     map[string]Expression
}

func (s *StructLiteralExpr) String() string {
	var fields []string
	for name, expr := range s.Fields {
		fields = append(fields, name+": "+expr.String())
	}
	return s.StructName + " { " + strings.Join(fields, ", ") + " }"
}
func (s *StructLiteralExpr) expressionNode() {}

type LambdaExpr struct {
	Params           []string
	ParamTypes       map[string]*Vibe67Type // Type annotations for parameters (nil if none)
	VariadicParam    string                 // Name of variadic parameter (if any), empty if none
	ReturnType       *Vibe67Type            // Return type annotation (nil if none)
	Body             Expression
	IsPure           bool              // Automatically detected: true if function has no side effects
	CapturedVars     []string          // Variables captured from outer scope (for closures)
	CapturedVarTypes map[string]string // Types of captured variables (for correct codegen)
	IsNestedLambda   bool              // True if this lambda is defined inside another lambda
}

func (l *LambdaExpr) String() string {
	return "(" + strings.Join(l.Params, ", ") + ") -> " + l.Body.String()
}
func (l *LambdaExpr) expressionNode() {}

// Pattern represents a pattern match clause parameter
type Pattern interface {
	Node
	patternNode()
}

// LiteralPattern matches a specific literal value
type LiteralPattern struct {
	Value Expression // NumberExpr, StringExpr, or BoolExpr
}

func (lp *LiteralPattern) String() string { return lp.Value.String() }
func (lp *LiteralPattern) patternNode()   {}

// VarPattern binds the argument to a variable
type VarPattern struct {
	Name string // Variable name to bind
}

func (vp *VarPattern) String() string { return vp.Name }
func (vp *VarPattern) patternNode()   {}

// WildcardPattern matches any value without binding
type WildcardPattern struct{}

func (wp *WildcardPattern) String() string { return "_" }
func (wp *WildcardPattern) patternNode()   {}

// PatternClause represents one pattern case: (pattern1, pattern2, ...) -> body
type PatternClause struct {
	Patterns []Pattern
	Body     Expression
}

func (pc *PatternClause) String() string {
	pats := make([]string, len(pc.Patterns))
	for i, p := range pc.Patterns {
		pats[i] = p.String()
	}
	return "(" + strings.Join(pats, ", ") + ") -> " + pc.Body.String()
}

// PatternLambdaExpr: lambda with pattern matching on parameters
// Example: factorial := (0) -> 1 | (n) -> n * factorial(n-1)
type PatternLambdaExpr struct {
	Clauses        []*PatternClause
	IsPure         bool
	CapturedVars   []string
	IsNestedLambda bool
}

func (pl *PatternLambdaExpr) String() string {
	clauses := make([]string, len(pl.Clauses))
	for i, c := range pl.Clauses {
		clauses[i] = c.String()
	}
	return strings.Join(clauses, " | ")
}
func (pl *PatternLambdaExpr) expressionNode() {}

// MultiLambdaExpr: multiple lambda dispatch based on argument count
// Example: f = (x) -> x, (x, y) -> x + y
type MultiLambdaExpr struct {
	Lambdas []*LambdaExpr
}

func (m *MultiLambdaExpr) String() string {
	parts := make([]string, len(m.Lambdas))
	for i, lambda := range m.Lambdas {
		parts[i] = lambda.String()
	}
	return strings.Join(parts, ", ")
}
func (m *MultiLambdaExpr) expressionNode() {}

type ParallelExpr struct {
	List      Expression // The list/data to operate on
	Operation Expression // The lambda or function to apply
}

func (p *ParallelExpr) String() string {
	return p.List.String() + " || " + p.Operation.String()
}
func (p *ParallelExpr) expressionNode() {}

type PipeExpr struct {
	Left  Expression // Input to the pipe
	Right Expression // Operation to apply
}

func (p *PipeExpr) String() string {
	return p.Left.String() + " | " + p.Right.String()
}
func (p *PipeExpr) expressionNode() {}

type ComposeExpr struct {
	Left  Expression // Outer function (applied second)
	Right Expression // Inner function (applied first)
}

func (c *ComposeExpr) String() string {
	return c.Left.String() + " <> " + c.Right.String()
}
func (c *ComposeExpr) expressionNode() {}

type BackgroundExpr struct {
	Expr Expression // Expression to execute in background process
}

func (b *BackgroundExpr) String() string {
	return b.Expr.String() + " &"
}
func (b *BackgroundExpr) expressionNode() {}

type SendExpr struct {
	Target  Expression // Port or address to send to (e.g., &8080)
	Message Expression // Message to send (typically string)
}

func (s *SendExpr) String() string {
	return s.Target.String() + " <- " + s.Message.String()
}
func (s *SendExpr) expressionNode() {}

type ReceiveExpr struct {
	Source Expression // Port or address to receive from (e.g., &8080)
}

func (r *ReceiveExpr) String() string {
	return "<= " + r.Source.String()
}
func (r *ReceiveExpr) expressionNode() {}

type LengthExpr struct {
	Operand Expression
}

func (l *LengthExpr) String() string {
	return "#" + l.Operand.String()
}
func (l *LengthExpr) expressionNode() {}

type CastExpr struct {
	Expr       Expression
	Type       string // "i8", "i32", "u64", "f32", "f64", "cstr", "ptr", "number", "string", "list"
	RawBitcast bool   // true for as!, false for as (numeric conversion)
}

func (c *CastExpr) String() string {
	if c.Expr == nil {
		return "<nil> as " + c.Type
	}
	return c.Expr.String() + " as " + c.Type
}
func (c *CastExpr) expressionNode() {}

type UnsafeExpr struct {
	X86_64Block   []Statement       // x86_64 architecture block
	ARM64Block    []Statement       // arm64 architecture block
	RISCV64Block  []Statement       // riscv64 architecture block
	X86_64Return  *UnsafeReturnStmt // optional return value for x86_64
	ARM64Return   *UnsafeReturnStmt // optional return value for arm64
	RISCV64Return *UnsafeReturnStmt // optional return value for riscv64
}

func (u *UnsafeExpr) String() string {
	return fmt.Sprintf("unsafe { x86_64: %d stmts } { arm64: %d stmts } { riscv64: %d stmts }",
		len(u.X86_64Block), len(u.ARM64Block), len(u.RISCV64Block))
}
func (u *UnsafeExpr) expressionNode() {}

type RegisterExpr struct {
	Name string // Register name (e.g., "rax", "xmm0", "x0", "a0")
}

func (r *RegisterExpr) String() string {
	return r.Name
}
func (r *RegisterExpr) expressionNode() {}

type RegisterAssignStmt struct {
	Register string      // Register name (e.g., "rax", "x0", "a0") or memory address like "[rax]"
	Value    interface{} // Either Expression, string (register), *RegisterOp, or *MemoryLoad
}

func (r *RegisterAssignStmt) String() string {
	switch v := r.Value.(type) {
	case Expression:
		return r.Register + " <- " + v.String()
	case string:
		return r.Register + " <- " + v
	case *RegisterOp:
		return r.Register + " <- " + v.String()
	case *MemoryLoad:
		return r.Register + " <- " + v.String()
	default:
		return r.Register + " <- <unknown>"
	}
}
func (r *RegisterAssignStmt) statementNode() {}

type UnsafeReturnStmt struct {
	Register string // Register to return (e.g., "rax", "xmm0")
	AsType   string // Optional type cast (e.g., "cstr", "pointer", empty for Vibe67 value)
}

func (u *UnsafeReturnStmt) String() string {
	if u.AsType != "" {
		return fmt.Sprintf("%s as %s", u.Register, u.AsType)
	}
	return u.Register
}
func (u *UnsafeReturnStmt) statementNode() {}

type RegisterOp struct {
	Left     string      // Register name or empty for unary
	Operator string      // +, -, *, /, %, &, |, ^, <<, >>, ~
	Right    interface{} // Register name (string) or immediate (NumberExpr)
}

func (r *RegisterOp) String() string {
	if r.Left == "" {
		// Unary operation
		switch v := r.Right.(type) {
		case string:
			return r.Operator + v
		default:
			return r.Operator + "<unknown>"
		}
	}
	// Binary operation
	switch v := r.Right.(type) {
	case Expression:
		return r.Left + " " + r.Operator + " " + v.String()
	case string:
		return r.Left + " " + r.Operator + " " + v
	default:
		return r.Left + " " + r.Operator + " <unknown>"
	}
}

type MemoryLoad struct {
	Size    string // "u8", "u16", "u32", "u64", "i8", "i16", "i32", "i64" (empty = u64)
	Address string // Register containing address (e.g., "rax", "rbx")
	Offset  int64  // Optional offset (e.g., [rax + 16])
}

func (m *MemoryLoad) String() string {
	sizeStr := ""
	if m.Size != "" && m.Size != "u64" {
		sizeStr = m.Size + " "
	}
	offsetStr := ""
	if m.Offset != 0 {
		offsetStr = fmt.Sprintf(" + %d", m.Offset)
	}
	return sizeStr + "[" + m.Address + offsetStr + "]"
}

type MemoryStore struct {
	Size    string      // "uint8", "uint16", "uint32", "uint64" (empty = uint64)
	Address string      // Register containing address (e.g., "rax", "rbx")
	Offset  int64       // Optional offset (e.g., [rax + 16])
	Value   interface{} // Value to store (register name string or *NumberExpr)
}

func (m *MemoryStore) String() string {
	sizeStr := ""
	if m.Size != "" && m.Size != "uint64" {
		sizeStr = " as " + m.Size
	}
	offsetStr := ""
	if m.Offset != 0 {
		offsetStr = fmt.Sprintf(" + %d", m.Offset)
	}
	return "[" + m.Address + offsetStr + "] <- " + fmt.Sprint(m.Value) + sizeStr
}

func (m *MemoryStore) statementNode() {}

type SyscallStmt struct{}

func (s *SyscallStmt) String() string { return "syscall" }
func (s *SyscallStmt) statementNode() {}

// ArenaStmt represents an arena memory block: arena { ... }
// All allocations within the block are freed when the arena exits
type ArenaStmt struct {
	Body []Statement // Statements executed within the arena
}

func (a *ArenaStmt) String() string {
	var out strings.Builder
	out.WriteString("arena {\n")
	for _, stmt := range a.Body {
		out.WriteString("  ")
		out.WriteString(stmt.String())
		out.WriteString("\n")
	}
	out.WriteString("}")
	return out.String()
}
func (a *ArenaStmt) statementNode() {}

// ArenaExpr represents an arena block used as an expression
type ArenaExpr struct {
	Body []Statement // Statements executed within the arena
}

func (a *ArenaExpr) String() string {
	var out strings.Builder
	out.WriteString("arena { ")
	for i, stmt := range a.Body {
		if i > 0 {
			out.WriteString("; ")
		}
		out.WriteString(stmt.String())
	}
	out.WriteString(" }")
	return out.String()
}
func (a *ArenaExpr) expressionNode() {}

// VectorExpr represents a SIMD vector literal: vec2(x, y) or vec4(x, y, z, w)
type VectorExpr struct {
	Components []Expression // 2 or 4 components
	Size       int          // 2 or 4
}

func (v *VectorExpr) String() string {
	var out strings.Builder
	out.WriteString(fmt.Sprintf("vec%d(", v.Size))
	for i, comp := range v.Components {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(comp.String())
	}
	out.WriteString(")")
	return out.String()
}
func (v *VectorExpr) expressionNode() {}

// DeferStmt represents a deferred expression: defer expr
// Executed at the end of the current scope in LIFO order
type DeferStmt struct {
	Call Expression // Expression to execute at scope exit (typically a function call)
}

func (d *DeferStmt) String() string { return "defer " + d.Call.String() }
func (d *DeferStmt) statementNode() {}

// SpawnStmt represents a vibe67ped process: vibe67 expr [ | params | block ]
// Creates a new process via fork() and optionally waits for result
type SpawnStmt struct {
	Expr   Expression // Expression to execute in child process
	Params []string   // Optional: variable names for pipe destructuring
	Block  *BlockExpr // Optional: block to execute with result (implies wait)
}

func (s *SpawnStmt) String() string {
	result := "vibe67 " + s.Expr.String()
	if s.Block != nil {
		result += " | "
		for i, param := range s.Params {
			if i > 0 {
				result += ", "
			}
			result += param
		}
		result += " | " + s.Block.String()
	}
	return result
}
func (s *SpawnStmt) statementNode() {}

// AliasStmt represents a keyword alias: alias for=@
// Creates alternative syntax for existing keywords (useful for language packs)
type AliasStmt struct {
	NewName    string    // New keyword name (e.g., "for")
	TargetName string    // Target keyword/token (e.g., "@")
	Target     TokenType // Resolved target token type
}

func (a *AliasStmt) String() string { return "alias " + a.NewName + "=" + a.TargetName }
func (a *AliasStmt) statementNode() {}
