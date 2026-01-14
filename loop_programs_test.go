package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestLoopPrograms tests loop constructs
func TestLoopPrograms(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name: "simple_range_loop",
			source: `@ i in 0..<5 {
    println(i)
}
`,
			expected: "0\n1\n2\n3\n4\n",
		},
		{
			name: "loop_with_arithmetic",
			source: `sum := 0
@ i in 1..<11 {
    sum += i
}
println(sum)
`,
			expected: "55\n",
		},
		{
			name: "nested_loops",
			source: `@ i in 0..<3 {
    @ j in 0..<3 {
        printf("%v ", i * 3 + j)
    }
    println("")
}
`,
			expected: "0.000000 1.000000 2.000000 \n3.000000 4.000000 5.000000 \n6.000000 7.000000 8.000000 \n",
		},
		{
			name: "loop_break",
			source: `@ i in 0..<10 {
    i > 5 {
        ret @
    }
    println(i)
}
`,
			expected: "0\n1\n2\n3\n4\n5\n",
		},
		{
			name: "list_iteration",
			source: `items := [10, 20, 30, 40]
@ item in items {
    println(item)
}
`,
			expected: "10\n20\n30\n40\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testInlineVibe67(t, tt.name, tt.source, tt.expected)
		})
	}
}

// TestExistingLoopPrograms runs existing loop test programs
func TestExistingLoopPrograms(t *testing.T) {
	tests := []string{
		"loop_test",
		"loop_test2",
		"loop_simple_test",
		"loop_mult",
		"loop_with_arithmetic",
		"loop_at_test",
		"loop_break_test",
		"nested_loop",
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

// TestInclusiveRange tests the inclusive range operator (..)
func TestInclusiveRange(t *testing.T) {
	source := `@ i in 1..5 {
    println(i)
}
`
	expected := "1\n2\n3\n4\n5\n"
	testInlineVibe67(t, "inclusive_range", source, expected)
}

// TestDeeplyNestedLoops tests loops with 5+ levels of nesting (uses stack-based counters)
func TestDeeplyNestedLoops(t *testing.T) {
	// Test 5-level nesting (levels 0-2 use registers, 3-4 use stack)
	source5 := `sum := 0
@ a in 0..<2 {
    @ b in 0..<2 {
        @ c in 0..<2 {
            @ d in 0..<2 {
                @ e in 0..<2 {
                    sum <- sum + 1
                }
            }
        }
    }
}
println(sum)
`
	testInlineVibe67(t, "5_level_nesting", source5, "32\n")

	// Test 6-level nesting (all stack-based beyond first 3)
	source6 := `count := 0
@ a in 0..<2 {
    @ b in 0..<2 {
        @ c in 0..<2 {
            @ d in 0..<2 {
                @ e in 0..<2 {
                    @ f in 0..<2 {
                        count <- count + 1
                    }
                }
            }
        }
    }
}
println(count)
`
	testInlineVibe67(t, "6_level_nesting", source6, "64\n")
}









