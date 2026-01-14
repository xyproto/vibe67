package main

import (
	"strings"
	"testing"
)

// TestListUpdateMinimal tests most basic list update
func TestListUpdateMinimal(t *testing.T) {
	source := `nums := [5]
nums[0] <- 10
println(nums[0])
`
	result := compileAndRun(t, source)
	if !strings.Contains(result, "10\n") {
		t.Errorf("Expected output to contain '10\\n', got: %s", result)
	}
}

// TestListUpdateBasic tests basic list element update
func TestListUpdateBasic(t *testing.T) {
	source := `arr := [5, 10, 15]
println(arr[0])
arr[0] <- 99
println(arr[0])
`
	result := compileAndRun(t, source)
	if !strings.Contains(result, "5\n99\n") {
		t.Errorf("Expected output to contain '5\\n99\\n', got: %s", result)
	}
}

// TestListUpdateSingleElement tests updating a single-element list
func TestListUpdateSingleElement(t *testing.T) {
	source := `arr := [42]
arr[0] <- 100
println(arr[0])
`
	result := compileAndRun(t, source)
	if !strings.Contains(result, "100\n") {
		t.Errorf("Expected output to contain '100\\n', got: %s", result)
	}
}

// TestListUpdateMiddleElement tests updating a middle element
func TestListUpdateMiddleElement(t *testing.T) {
	source := `arr := [1, 2, 3, 4, 5]
arr[2] <- 99
println(arr[2])
`
	result := compileAndRun(t, source)
	if !strings.Contains(result, "99\n") {
		t.Errorf("Expected output to contain '99\\n', got: %s", result)
	}
}

// TestListUpdateLastElement tests updating the last element
func TestListUpdateLastElement(t *testing.T) {
	source := `arr := [10, 20, 30]
arr[2] <- 999
println(arr[2])
`
	result := compileAndRun(t, source)
	if !strings.Contains(result, "999\n") {
		t.Errorf("Expected output to contain '999\\n', got: %s", result)
	}
}

// TestListUpdateMultiple tests multiple updates
func TestListUpdateMultiple(t *testing.T) {
	source := `nums := [1, 2, 3]
nums[0] <- 10
nums[1] <- 20
nums[2] <- 30
println(nums[0])
println(nums[1])
println(nums[2])
`
	result := compileAndRun(t, source)
	if !strings.Contains(result, "10\n20\n30\n") {
		t.Errorf("Expected output to contain '10\\n20\\n30\\n', got: %s", result)
	}
}

// TestListUpdatePreservesOtherElements tests that other elements are unchanged
func TestListUpdatePreservesOtherElements(t *testing.T) {
	source := `arr := [100, 200, 300, 400]
arr[1] <- 999
println(arr[0])
println(arr[1])
println(arr[2])
println(arr[3])
`
	result := compileAndRun(t, source)
	if !strings.Contains(result, "100\n999\n300\n400\n") {
		t.Errorf("Expected output to contain '100\\n999\\n300\\n400\\n', got: %s", result)
	}
}

// TestAppendFunction tests the append() builtin function
func TestAppendFunction(t *testing.T) {
	source := `list1 := [1, 2, 3]
list2 := append(list1, 4)
println(list2[0])
println(list2[1])
println(list2[2])
println(list2[3])
`
	result := compileAndRun(t, source)
	if !strings.Contains(result, "1\n2\n3\n4\n") {
		t.Errorf("Expected output to contain '1\\n2\\n3\\n4\\n', got: %s", result)
	}
}

// TestTailOperatorUpdate tests the _ (tail) operator (list_update context)
func TestTailOperatorUpdate(t *testing.T) {
	t.Skip("Tail operator has known issues - see TAIL.md")
	source := `list := [1, 2, 3, 4]
rest := _list
println(rest[0])
println(rest[1])
`
	result := compileAndRun(t, source)
	if !strings.Contains(result, "2\n3\n") {
		t.Errorf("Expected output to contain '2\\n3\\n', got: %s", result)
	}
}









