package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestListPrograms tests list operations
func TestListPrograms(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name: "list_literal",
			source: `items := [1, 2, 3, 4, 5]
println(#items)
`,
			expected: "5\n",
		},
		{
			name: "list_indexing",
			source: `items := [10, 20, 30]
println(items[0])
println(items[1])
println(items[2])
`,
			expected: "10\n20\n30\n",
		},
		{
			name: "list_update",
			source: `items := [1, 2, 3]
items[1] <- 99
println(items[0])
println(items[1])
println(items[2])
`,
			expected: "1\n99\n3\n",
		},
		{
			name: "empty_list",
			source: `empty := []
println(#empty)
`,
			expected: "0\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testInlineVibe67(t, tt.name, tt.source, tt.expected)
		})
	}
}

// TestExistingListPrograms runs existing list test programs
func TestExistingListPrograms(t *testing.T) {
	tests := []string{
		"list_test",
		"list_test2",
		"list_simple",
		"list_index_test",
		"list_iter_test",
		"manual_list_test",
		"len_test",
		"len_simple",
		"len_empty",
	}

	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			srcPath := filepath.Join("testprograms", name+".vibe67")
			resultPath := filepath.Join("testprograms", name+".result")

			if _, err := os.Stat(srcPath); os.IsNotExist(err) {
				t.Skipf("Source file %s not found", srcPath)
				return
			}

			var expected string
			if data, err := os.ReadFile(resultPath); err == nil {
				expected = string(data)
			}

			tmpDir := t.TempDir()
			exePath := filepath.Join(tmpDir, name)

			platform := GetDefaultPlatform()
			if err := CompileC67(srcPath, exePath, platform); err != nil {
				t.Fatalf("Compilation failed: %v", err)
			}

			cmd := exec.Command("timeout", "5s", exePath)
			output, err := cmd.CombinedOutput()
			if err != nil {
				if _, ok := err.(*exec.ExitError); !ok {
					t.Fatalf("Execution failed: %v", err)
				}
			}

			if expected != "" {
				actual := string(output)
				if actual != expected {
					t.Errorf("Output mismatch:\nExpected:\n%s\nActual:\n%s",
						expected, actual)
				}
			}
		})
	}
}

// TestHeadFunction tests the head() function
func TestHeadFunction(t *testing.T) {
	source := `list := [1, 2, 3, 4]
first := head(list)
println(first)
`
	result := compileAndRun(t, source)
	if !strings.Contains(result, "1\n") {
		t.Errorf("Expected output to contain '1\\n', got: %s", result)
	}
}

// TestTailFunction tests the tail() function
func TestTailFunction(t *testing.T) {
	source := `list := [1, 2, 3, 4]
rest := tail(list)
println(rest[0])
println(rest[1])
println(rest[2])
`
	result := compileAndRun(t, source)
	if !strings.Contains(result, "2\n3\n4\n") {
		t.Errorf("Expected output to contain '2\\n3\\n4\\n', got: %s", result)
	}
}

// TestAppendMethod tests the .append() method syntax sugar
func TestAppendMethod(t *testing.T) {
	source := `xs := [1, 2, 3]
ys := xs.append(4)
println(ys[0])
println(ys[1])
println(ys[2])
println(ys[3])
println(#ys)
`
	testInlineVibe67(t, "append_method", source, "1\n2\n3\n4\n4\n")
}

// TestAppendFunctionBasic tests the append() function directly
func TestAppendFunctionBasic(t *testing.T) {
	source := `xs := [10, 20]
ys := append(xs, 30)
println(ys[0])
println(ys[1])
println(ys[2])
println(#ys)
`
	testInlineVibe67(t, "append_function", source, "10\n20\n30\n3\n")
}

// TestPopMethod tests the .pop() method syntax sugar
// Confidence that this function is working: 95%
func TestPopMethod(t *testing.T) {
	source := `xs := [1, 2, 3, 4]
new_list, popped_value = xs.pop()
println(new_list[0])
println(new_list[1])
println(new_list[2])
println(popped_value)
println(#new_list)
`
	testInlineVibe67(t, "pop_method", source, "1\n2\n3\n4\n3\n")
}

// TestPopFunction tests the pop() function directly
// Confidence that this function is working: 95%
func TestPopFunction(t *testing.T) {
	source := `xs := [10, 20, 30]
new_list, popped = pop(xs)
println(new_list[0])
println(new_list[1])
println(popped)
println(#new_list)
`
	testInlineVibe67(t, "pop_function", source, "10\n20\n30\n2\n")
}

// TestPopEmptyList tests pop() on an empty list
// Confidence that this function is working: 95%
func TestPopEmptyList(t *testing.T) {
	source := `xs := []
new_list, popped = pop(xs)
println(#new_list)
println(is_nan(popped))
`
	testInlineVibe67(t, "pop_empty", source, "0\n1\n")
}

// TestAppendChaining tests method chaining with append
func TestAppendChaining(t *testing.T) {
	source := `xs := []
ys := xs.append(1).append(2).append(3)
println(ys[0])
println(ys[1])
println(ys[2])
println(#ys)
`
	testInlineVibe67(t, "append_chaining", source, "1\n2\n3\n3\n")
}
