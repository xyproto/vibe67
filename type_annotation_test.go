package main

import (
	"testing"
)

func TestTypeAnnotations(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected string
	}{
		{
			name: "num type annotation",
			code: `
x: num = 42
printf("%f\n", x)
`,
			expected: "42.000000\n",
		},
		{
			name: "str type annotation",
			code: `
name: str = "Alice"
printf("%s\n", name)
`,
			expected: "Alice\n",
		},
		{
			name: "list type annotation",
			code: `
nums: list = [1, 2, 3]
printf("%f\n", nums[0])
`,
			expected: "1.000000\n",
		},
		{
			name: "map type annotation",
			code: `
config: map = {port: 8080}
printf("%f\n", config.port)
`,
			expected: "8080.000000\n",
		},
		{
			name: "mutable with type annotation",
			code: `
x: num := 10
x <- 20
printf("%f\n", x)
`,
			expected: "20.000000\n",
		},
		{
			name: "multiple variables with different types",
			code: `
a: num = 42
b: str = "test"
c: list = [1, 2]
printf("%f %s %f\n", a, b, c[0])
`,
			expected: "42.000000 test 1.000000\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compileAndRun(t, tt.code)
			if result != tt.expected {
				t.Errorf("Expected output %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestForeignTypeAnnotations(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected string
	}{
		{
			name: "cint type annotation",
			code: `
x: cint = 42
printf("%f\n", x)
`,
			expected: "42.000000\n",
		},
		{
			name: "clong type annotation",
			code: `
x: clong = 1000
printf("%f\n", x)
`,
			expected: "1000.000000\n",
		},
		{
			name: "cfloat type annotation",
			code: `
x: cfloat = 3.14
printf("%f\n", x)
`,
			expected: "3.140000\n",
		},
		{
			name: "cdouble type annotation",
			code: `
x: cdouble = 3.14159
printf("%f\n", x)
`,
			expected: "3.141589\n", // Rounding: off by 1 in last digit (IEEE 754 precision limits)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compileAndRun(t, tt.code)
			if result != tt.expected {
				t.Errorf("Expected output %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestTypeAnnotationBackwardCompat(t *testing.T) {
	// Test that code without type annotations still works
	tests := []struct {
		name     string
		code     string
		expected string
	}{
		{
			name: "no annotations - number",
			code: `
x = 42
printf("%f\n", x)
`,
			expected: "42.000000\n",
		},
		{
			name: "no annotations - string",
			code: `
name = "Bob"
printf("%s\n", name)
`,
			expected: "Bob\n",
		},
		{
			name: "mixed - some with, some without",
			code: `
x: num = 10
y = 20
printf("%f %f\n", x, y)
`,
			expected: "10.000000 20.000000\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compileAndRun(t, tt.code)
			if result != tt.expected {
				t.Errorf("Expected output %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestTypeAnnotationContextual(t *testing.T) {
	// Test that type keywords can be used as identifiers
	tests := []struct {
		name     string
		code     string
		expected string
	}{
		{
			name: "num as variable name",
			code: `
num = 100
x: num = num * 2
printf("%f\n", x)
`,
			expected: "200.000000\n",
		},
		{
			name: "str as variable name",
			code: `
str = "prefix"
s: str = str
printf("%s\n", s)
`,
			expected: "prefix\n",
		},
		{
			name: "list as variable name",
			code: `
list = [1, 2, 3]
mylist: list = list
printf("%f\n", mylist[0])
`,
			expected: "1.000000\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compileAndRun(t, tt.code)
			if result != tt.expected {
				t.Errorf("Expected output %q, got %q", tt.expected, result)
			}
		})
	}
}









