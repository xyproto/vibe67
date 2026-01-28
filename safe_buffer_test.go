package main

import (
	"testing"
)

func TestSafeBufferBasicUsage(t *testing.T) {
	sb := NewSafeBuffer("test")

	// Write some data
	sb.Write([]byte("hello"))
	if sb.Len() != 5 {
		t.Errorf("Expected length 5, got %d", sb.Len())
	}

	// Commit the buffer
	sb.Commit()

	// Reading is safe after commit
	if string(sb.Bytes()) != "hello" {
		t.Errorf("Expected 'hello', got '%s'", string(sb.Bytes()))
	}
}

func TestSafeBufferPreventsWriteAfterCommit(t *testing.T) {
	sb := NewSafeBuffer("test")
	sb.Write([]byte("data"))
	sb.Commit()

	// This should panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when writing to committed buffer")
		}
	}()

	sb.Write([]byte("more"))
}

func TestSafeBufferReset(t *testing.T) {
	sb := NewSafeBuffer("test")

	// Write and commit
	sb.Write([]byte("first"))
	sb.Commit()

	// Reset should allow reuse
	sb.Reset()
	if sb.IsCommitted() {
		t.Error("Buffer should not be committed after reset")
	}

	// Write new data
	sb.Write([]byte("second"))
	if string(sb.Bytes()) != "second" {
		t.Errorf("Expected 'second', got '%s'", string(sb.Bytes()))
	}
}

func TestScopedBuffer(t *testing.T) {
	scope := NewScopedBuffer("test")
	defer scope.Complete()

	// Write data
	scope.Buffer().Write([]byte("scoped data"))

	// Complete marks it ready
	scope.Complete()

	// Now we can read
	if string(scope.Bytes()) != "scoped data" {
		t.Errorf("Expected 'scoped data', got '%s'", string(scope.Bytes()))
	}
}

func TestScopedBufferReuse(t *testing.T) {
	scope := NewScopedBuffer("test")

	// First use
	scope.Buffer().Write([]byte("first"))
	scope.Complete()
	if string(scope.Bytes()) != "first" {
		t.Errorf("Expected 'first', got '%s'", string(scope.Bytes()))
	}

	// Reset and reuse
	scope.ResetScope()
	scope.Buffer().Write([]byte("second"))
	scope.Complete()
	if string(scope.Bytes()) != "second" {
		t.Errorf("Expected 'second', got '%s'", string(scope.Bytes()))
	}
}

func TestScopedBufferMustCompleteBeforeRead(t *testing.T) {
	scope := NewScopedBuffer("test")
	scope.Buffer().Write([]byte("data"))

	// This should panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when reading uncommitted scoped buffer")
		}
	}()

	_ = scope.Bytes()
}
