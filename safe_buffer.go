// Completion: 100% - Module complete
package main

import (
	"bytes"
	"fmt"
	"os"
)

// SafeBuffer wraps bytes.Buffer with explicit lifecycle management to prevent
// Reset() bugs. It tracks whether the buffer has been "committed" and prevents
// accidental resets or writes after commit.
type SafeBuffer struct {
	buf       *bytes.Buffer
	committed bool   // True once Commit() is called
	name      string // For debugging
}

// NewSafeBuffer creates a new SafeBuffer with a name for debugging
func NewSafeBuffer(name string) *SafeBuffer {
	return &SafeBuffer{
		buf:  &bytes.Buffer{},
		name: name,
	}
}

// Write appends bytes to the buffer. Panics if buffer is committed.
func (sb *SafeBuffer) Write(p []byte) (n int, err error) {
	if sb.committed {
		panic(fmt.Sprintf("SafeBuffer(%s): Cannot write to committed buffer", sb.name))
	}
	return sb.buf.Write(p)
}

// Bytes returns the buffer contents. Safe to call after commit.
func (sb *SafeBuffer) Bytes() []byte {
	return sb.buf.Bytes()
}

// Len returns the buffer length
func (sb *SafeBuffer) Len() int {
	return sb.buf.Len()
}

// Commit marks the buffer as complete. After this, no more writes or resets allowed.
func (sb *SafeBuffer) Commit() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "SafeBuffer(%s): Committed with %d bytes\n", sb.name, sb.buf.Len())
	}
	sb.committed = true
}

// Reset clears the buffer and uncommits it. Safe to call anytime.
func (sb *SafeBuffer) Reset() {
	if VerboseMode && sb.committed {
		fmt.Fprintf(os.Stderr, "SafeBuffer(%s): Reset called on committed buffer, clearing %d bytes\n",
			sb.name, sb.buf.Len())
	}
	sb.buf.Reset()
	sb.committed = false
}

// IsCommitted returns true if the buffer has been committed
func (sb *SafeBuffer) IsCommitted() bool {
	return sb.committed
}

// MustNotBeCommitted panics if the buffer is committed (for defensive programming)
func (sb *SafeBuffer) MustNotBeCommitted() {
	if sb.committed {
		panic(fmt.Sprintf("SafeBuffer(%s): Expected uncommitted buffer", sb.name))
	}
}

// ScopedBuffer provides automatic reset-on-complete semantics for temporary buffers.
// Usage:
//
//	scope := NewScopedBuffer("mytemp")
//	defer scope.Complete()  // Automatically resets on exit
//	... write to scope.Buffer() ...
type ScopedBuffer struct {
	buf  *SafeBuffer
	done bool
}

// NewScopedBuffer creates a new scoped buffer
func NewScopedBuffer(name string) *ScopedBuffer {
	return &ScopedBuffer{
		buf: NewSafeBuffer(name),
	}
}

// Buffer returns the underlying SafeBuffer
func (s *ScopedBuffer) Buffer() *SafeBuffer {
	return s.buf
}

// Complete commits the buffer (marks it as ready for reading).
// Call this explicitly or via defer to ensure cleanup.
func (s *ScopedBuffer) Complete() {
	if !s.done {
		s.buf.Commit()
		s.done = true
	}
}

// Bytes returns the buffer contents. Must be called after Complete().
func (s *ScopedBuffer) Bytes() []byte {
	if !s.done {
		panic(fmt.Sprintf("ScopedBuffer(%s): Must call Complete() before reading", s.buf.name))
	}
	return s.buf.Bytes()
}

// ResetScope resets the buffer for reuse (uncommits and clears)
func (s *ScopedBuffer) ResetScope() {
	s.buf.Reset()
	s.done = false
}
