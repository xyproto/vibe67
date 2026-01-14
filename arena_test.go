package main

import (
	"testing"
)

func TestArenaBasicAllocation(t *testing.T) {
	code := `
		x := alloc(100)
		println(x)
	`
	result := compileAndRun(t, code)
	if result == "" {
		t.Fatal("Expected pointer output")
	}
}

func TestArenaMultipleAllocations(t *testing.T) {
	code := `
		x := alloc(10)
		y := alloc(20)
		z := alloc(30)
		println("OK")
	`
	result := compileAndRun(t, code)
	if result != "OK\n" {
		t.Fatalf("Expected 'OK', got %q", result)
	}
}

func TestArenaGrowth(t *testing.T) {
	code := `
		x := alloc(1000000)
		println("OK")
	`
	result := compileAndRun(t, code)
	if result != "OK\n" {
		t.Fatalf("Expected 'OK', got %q", result)
	}
}

func TestArenaBlock(t *testing.T) {
	code := `
		arena {
			x := alloc(100)
			println("Inside")
		}
		println("Outside")
	`
	result := compileAndRun(t, code)
	if result != "Inside\nOutside\n" {
		t.Fatalf("Expected 'Inside\\nOutside\\n', got %q", result)
	}
}

func TestArenaNestedBlocks(t *testing.T) {
	code := `
		arena {
			x := alloc(100)
			arena {
				y := alloc(200)
				println("Inner")
			}
			println("Outer")
		}
		println("Done")
	`
	result := compileAndRun(t, code)
	if result != "Inner\nOuter\nDone\n" {
		t.Fatalf("Expected 'Inner\\nOuter\\nDone\\n', got %q", result)
	}
}

func TestArenaStringAllocation(t *testing.T) {
	code := `
		x := 42
		s := x as string
		println(s)
	`
	result := compileAndRun(t, code)
	if result != "42\n" {
		t.Fatalf("Expected '42\\n', got %q", result)
	}
}

func TestArenaListAllocation(t *testing.T) {
	code := `
		xs := [1, 2, 3]
		println(xs[0])
		println(xs[1])
		println(xs[2])
	`
	result := compileAndRun(t, code)
	if result != "1\n2\n3\n" {
		t.Fatalf("Expected '1\\n2\\n3\\n', got %q", result)
	}
}









