package main

import (
	"strings"
	"testing"
)

// TestBasicClass tests basic class definition and instantiation
func TestBasicClass(t *testing.T) {
	t.Skip("OOP features not yet fully implemented in 3.0")
	code := `
class Point {
    init = (x, y) -> {
        .x = x
        .y = y
    }
    
    distance_from_origin = () -> {
        sqrt(.x * .x + .y * .y)
    }
}

p = Point(3, 4)
println(p.distance_from_origin())
`
	result := compileAndRun(t, code)

	if !strings.Contains(result, "5") {
		t.Errorf("Expected output to contain '5', got: %s", result)
	}
}

// TestClassWithMutableFields tests mutable class fields
func TestClassWithMutableFields(t *testing.T) {
	t.Skip("OOP features not yet fully implemented in 3.0")
	code := `
class Counter {
    init = start -> {
        .count := start
    }
    
    increment = () -> {
        .count <- .count + 1
    }
    
    get = () -> .count
}

c = Counter(10)
c.increment()
c.increment()
println(c.get())
`
	result := compileAndRun(t, code)

	if !strings.Contains(result, "12") {
		t.Errorf("Expected output to contain '12', got: %s", result)
	}
}

// TestClassComposition tests the <> composition operator
func TestClassComposition(t *testing.T) {
	t.Skip("OOP features not yet fully implemented in 3.0")
	code := `
// Behavior map for printing
Printable = {
    to_string = () -> {
        result := "{"
        @ key in keys(.) {
            result <- result :: f"{key}: {.[key]} "
        }
        result :: "}"
    }
}

// Class with composition
class Person <> Printable {
    init = (name, age) -> {
        .name = name
        .age = age
    }
}

p = Person("Alice", 30)
println(p.to_string())
`
	result := compileAndRun(t, code)

	if !strings.Contains(result, "Alice") || !strings.Contains(result, "30") {
		t.Errorf("Expected output to contain 'Alice' and '30', got: %s", result)
	}
}

// TestClassInheritance tests class inheritance/extension
func TestClassInheritance(t *testing.T) {
	t.Skip("OOP features not yet fully implemented in 3.0")
	code := `
class Animal {
    init = name -> {
        .name = name
    }
    
    speak = () -> {
        println(f"{.name} makes a sound")
    }
}

class Dog <> Animal {
    init = name -> {
        .name = name
        .species = "dog"
    }
    
    bark = () -> {
        println(f"{.name} barks!")
    }
}

d = Dog("Rex")
d.speak()
d.bark()
`
	result := compileAndRun(t, code)

	if !strings.Contains(result, "Rex makes a sound") {
		t.Errorf("Expected 'Rex makes a sound', got: %s", result)
	}
	if !strings.Contains(result, "Rex barks!") {
		t.Errorf("Expected 'Rex barks!', got: %s", result)
	}
}

// TestDotNotation tests the . (dot space) for "this"
func TestDotNotation(t *testing.T) {
	t.Skip("OOP features not yet fully implemented in 3.0")
	code := `
class Box {
    init = value -> {
        .value = value
    }
    
    double = () -> {
        .value <- .value * 2
        ret .  // Return this
    }
    
    get = () -> .value
}

b = Box(5)
b.double()
println(b.get())
`
	result := compileAndRun(t, code)

	if !strings.Contains(result, "10") {
		t.Errorf("Expected output to contain '10', got: %s", result)
	}
}

// TestMultipleComposition tests multiple behavior composition
func TestMultipleComposition(t *testing.T) {
	t.Skip("OOP features not yet fully implemented in 3.0")
	code := `
Comparable = {
    equals = other -> {
        @ key in keys(.) {
            .[key] != other[key] { ret 0 }
        }
        ret 1
    }
}

Serializable = {
    to_json = () -> {
        result := "{"
        @ key in keys(.) {
            result <- result :: f'"{key}": "{.[key]}", '
        }
        result :: "}"
    }
}

class Data <> Comparable <> Serializable {
    init = (a, b) -> {
        .a = a
        .b = b
    }
}

d1 = Data(1, 2)
d2 = Data(1, 2)
println(d1.equals(d2))
`
	result := compileAndRun(t, code)

	if !strings.Contains(result, "1") {
		t.Errorf("Expected output to contain '1', got: %s", result)
	}
}









