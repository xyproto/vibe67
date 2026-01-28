package main

import (
	"testing"
)

func TestVibe67HashMapBasicOperations(t *testing.T) {
	m := NewVibe67HashMap(16)

	// Test Set and Get
	m.Set(1, 10.5)
	m.Set(2, 20.5)
	m.Set(3, 30.5)

	if val, ok := m.Get(1); !ok || val != 10.5 {
		t.Errorf("Expected 10.5, got %v", val)
	}

	if val, ok := m.Get(2); !ok || val != 20.5 {
		t.Errorf("Expected 20.5, got %v", val)
	}

	if val, ok := m.Get(3); !ok || val != 30.5 {
		t.Errorf("Expected 30.5, got %v", val)
	}

	// Test Count
	if m.Count() != 3 {
		t.Errorf("Expected count 3, got %d", m.Count())
	}
}

func TestVibe67HashMapUpdate(t *testing.T) {
	m := NewVibe67HashMap(16)

	m.Set(1, 10.5)
	m.Set(1, 99.9) // Update

	if val, ok := m.Get(1); !ok || val != 99.9 {
		t.Errorf("Expected 99.9, got %v", val)
	}

	if m.Count() != 1 {
		t.Errorf("Expected count 1, got %d", m.Count())
	}
}

func TestVibe67HashMapDelete(t *testing.T) {
	m := NewVibe67HashMap(16)

	m.Set(1, 10.5)
	m.Set(2, 20.5)
	m.Set(3, 30.5)

	// Delete existing key
	if !m.Delete(2) {
		t.Error("Expected delete to return true")
	}

	if m.Count() != 2 {
		t.Errorf("Expected count 2, got %d", m.Count())
	}

	if _, ok := m.Get(2); ok {
		t.Error("Expected key 2 to be deleted")
	}

	// Delete non-existing key
	if m.Delete(999) {
		t.Error("Expected delete of non-existing key to return false")
	}
}

func TestVibe67HashMapCollision(t *testing.T) {
	m := NewVibe67HashMap(4) // Small size to force collisions

	// Add many entries to force collisions
	for i := uint64(1); i <= 20; i++ {
		m.Set(i, float64(i)*10.5)
	}

	// Verify all entries
	for i := uint64(1); i <= 20; i++ {
		if val, ok := m.Get(i); !ok || val != float64(i)*10.5 {
			t.Errorf("Expected %v, got %v for key %d", float64(i)*10.5, val, i)
		}
	}

	if m.Count() != 20 {
		t.Errorf("Expected count 20, got %d", m.Count())
	}
}

func TestVibe67HashMapResize(t *testing.T) {
	m := NewVibe67HashMap(4)

	// Add entries to trigger resize
	for i := uint64(1); i <= 100; i++ {
		m.Set(i, float64(i))
	}

	// Verify all entries after resize
	for i := uint64(1); i <= 100; i++ {
		if val, ok := m.Get(i); !ok || val != float64(i) {
			t.Errorf("Expected %v, got %v for key %d", float64(i), val, i)
		}
	}

	if m.Count() != 100 {
		t.Errorf("Expected count 100, got %d", m.Count())
	}
}

func TestVibe67HashMapKeys(t *testing.T) {
	m := NewVibe67HashMap(16)

	m.Set(1, 10.5)
	m.Set(2, 20.5)
	m.Set(3, 30.5)

	keys := m.Keys()
	if len(keys) != 3 {
		t.Errorf("Expected 3 keys, got %d", len(keys))
	}

	// Check all keys are present
	keyMap := make(map[uint64]bool)
	for _, k := range keys {
		keyMap[k] = true
	}

	for i := uint64(1); i <= 3; i++ {
		if !keyMap[i] {
			t.Errorf("Expected key %d to be present", i)
		}
	}
}

func TestVibe67HashMapValues(t *testing.T) {
	m := NewVibe67HashMap(16)

	m.Set(1, 10.5)
	m.Set(2, 20.5)
	m.Set(3, 30.5)

	values := m.Values()
	if len(values) != 3 {
		t.Errorf("Expected 3 values, got %d", len(values))
	}

	// Check all values are present
	valueMap := make(map[float64]bool)
	for _, v := range values {
		valueMap[v] = true
	}

	for _, expected := range []float64{10.5, 20.5, 30.5} {
		if !valueMap[expected] {
			t.Errorf("Expected value %v to be present", expected)
		}
	}
}

func TestVibe67HashMapEmpty(t *testing.T) {
	m := NewVibe67HashMap(16)

	if m.Count() != 0 {
		t.Errorf("Expected count 0 for empty map, got %d", m.Count())
	}

	if _, ok := m.Get(1); ok {
		t.Error("Expected Get on empty map to return false")
	}

	keys := m.Keys()
	if len(keys) != 0 {
		t.Errorf("Expected 0 keys for empty map, got %d", len(keys))
	}

	values := m.Values()
	if len(values) != 0 {
		t.Errorf("Expected 0 values for empty map, got %d", len(values))
	}
}
