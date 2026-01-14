package main

import (
	"os/exec"
	"runtime"
	"testing"
)

// TestPeepholeOptimizations tests peephole optimization patterns
func TestPeepholeOptimizations(t *testing.T) {
	// Skip on non-Linux
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("Peephole optimization tests only run on Linux x86_64")
	}

	tests := []struct {
		name       string
		code       string
		wantStdout string
	}{
		{
			name: "self comparison equality",
			code: `
x := 5
result := x == x
println(result)
`,
			wantStdout: "1\n",
		},
		{
			name: "self comparison inequality",
			code: `
x := 5
result := x != x
println(result)
`,
			wantStdout: "0\n",
		},
		{
			name: "self comparison less than",
			code: `
x := 5
result := x < x
println(result)
`,
			wantStdout: "0\n",
		},
		{
			name: "self comparison less than or equal",
			code: `
x := 5
result := x <= x
println(result)
`,
			wantStdout: "1\n",
		},
		{
			name: "constant comparison true",
			code: `
result := 5 > 3
println(result)
`,
			wantStdout: "1\n",
		},
		{
			name: "constant comparison false",
			code: `
result := 5 < 3
println(result)
`,
			wantStdout: "0\n",
		},
		{
			name: "and with false left",
			code: `
result := (0 and 5)
println(result)
`,
			wantStdout: "0\n",
		},
		{
			name: "and with false right",
			code: `
result := (5 and 0)
println(result)
`,
			wantStdout: "0\n",
		},
		{
			name: "and with true left",
			code: `
x := 7
result := (1 and x)
println(result)
`,
			wantStdout: "1\n",
		},
		{
			name: "and with true right",
			code: `
x := 7
result := (x and 1)
println(result)
`,
			wantStdout: "1\n",
		},
		{
			name: "or with true left",
			code: `
result := (1 or 0)
println(result)
`,
			wantStdout: "1\n",
		},
		{
			name: "or with true right",
			code: `
result := (0 or 5)
println(result)
`,
			wantStdout: "1\n",
		},
		{
			name: "or with false left",
			code: `
x := 7
result := (0 or x)
println(result)
`,
			wantStdout: "1\n",
		},
		{
			name: "or with false right",
			code: `
x := 7
result := (x or 0)
println(result)
`,
			wantStdout: "1\n",
		},
		{
			name: "double negation",
			code: `
x := 5
result := not(not(x))
println(result)
`,
			wantStdout: "1\n",
		},
		{
			name: "double negation with zero",
			code: `
x := 0
result := not(not(x))
println(result)
`,
			wantStdout: "0\n",
		},
		{
			name: "not of constant true",
			code: `
result := not(1)
println(result)
`,
			wantStdout: "0\n",
		},
		{
			name: "not of constant false",
			code: `
result := not(0)
println(result)
`,
			wantStdout: "1\n",
		},
		{
			name: "not of comparison inverted",
			code: `
x := 5
result := not(x < 10)
println(result)
`,
			wantStdout: "0\n",
		},
		{
			name: "complex optimization chain",
			code: `
x := 5
y := (x + 0) * 1
z := y - 0
result := z == z
println(result)
`,
			wantStdout: "1\n",
		},
		{
			name: "algebraic simplification with comparison",
			code: `
a := 10
b := (a * 1 + 0)
result := b > 5
println(result)
`,
			wantStdout: "1\n",
		},
		{
			name: "de morgan not x and not y to not or",
			code: `
x := 5
y := 10
result := (not(x < 3) and not(y < 8))
println(result)
`,
			wantStdout: "1\n",
		},
		{
			name: "de morgan or not applied for or to preserve short circuit",
			code: `
x := 5
y := 10
// (not x) or (not y) should NOT be transformed to not(x and y)
// to preserve short-circuit evaluation
result := (not(x > 10) or not(y > 20))
println(result)
`,
			wantStdout: "1\n",
		},
		{
			name: "zero divided by number",
			code: `
result := 0 / 5
println(result)
`,
			wantStdout: "0\n",
		},
		{
			name: "zero divided by expression",
			code: `
x := 10
result := 0 / x
println(result)
`,
			wantStdout: "0\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Compile code
			binary := compileTestBinary(t, tt.code)

			// Run and capture output
			cmd := exec.Command(binary)
			stdout, stderr, exitCode := runCommand(cmd)

			if exitCode != 0 {
				t.Errorf("Unexpected exit code %d, stderr: %s", exitCode, stderr)
			}

			if stdout != tt.wantStdout {
				t.Errorf("stdout mismatch:\nwant: %q\ngot:  %q", tt.wantStdout, stdout)
			}
		})
	}
}









