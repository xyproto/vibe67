// elf_writer.go - Object-oriented ELF writer
package main

import (
	"fmt"
	"os"
)

// ELFWriter handles ELF file generation with proper state management
type ELFWriter struct {
	target    Target
	eb        *ExecutableBuilder
	baseAddr  uint64 // Virtual base address (0 for PIE, 0x400000 for non-PIE)
	pageSize  uint64
	isDynamic bool

	// Layout information
	layout     map[string]SegmentLayout
	entryPoint uint64

	// State tracking
	phase CompilationPhase
}

type SegmentLayout struct {
	offset uint64
	addr   uint64
	size   int
}

// NewELFWriter creates a new ELF writer with proper configuration
func NewELFWriter(target Target, eb *ExecutableBuilder, isDynamic bool) *ELFWriter {
	baseAddr := uint64(0x0) // PIE by default
	if !isDynamic {
		// Static executables can use fixed base
		baseAddr = 0x400000
	}

	return &ELFWriter{
		target:    target,
		eb:        eb,
		baseAddr:  baseAddr,
		pageSize:  0x1000,
		isDynamic: isDynamic,
		layout:    make(map[string]SegmentLayout),
		phase:     PhaseInitial,
	}
}

// GetBaseAddr returns the virtual base address
func (w *ELFWriter) GetBaseAddr() uint64 {
	return w.baseAddr
}

// GetEstimatedRodataAddr returns an estimated rodata address for first-pass compilation
func (w *ELFWriter) GetEstimatedRodataAddr() uint64 {
	// Typical layout: headers at 0x0-0x2FFF, code at 0x3000+
	return w.baseAddr + 0x3000 + 0x100
}

// CalculateLayout computes the memory layout for all sections
func (w *ELFWriter) CalculateLayout(codeSize, rodataSize int, dataSymbols map[string]string) error {
	if w.phase != PhaseInitial && w.phase != PhaseELFLayout {
		return fmt.Errorf("CalculateLayout called in wrong phase: %v", w.phase)
	}

	w.phase = PhaseELFLayout

	elfHeaderSize := uint64(64)
	progHeaderSize := uint64(56)

	// Calculate header sizes
	numProgHeaders := 4 // PT_PHDR, PT_INTERP, PT_LOAD, PT_DYNAMIC for dynamic
	if !w.isDynamic {
		numProgHeaders = 1 // Just PT_LOAD for static
	}

	headersSize := elfHeaderSize + uint64(numProgHeaders)*progHeaderSize
	alignedHeaders := (headersSize + w.pageSize - 1) & ^(w.pageSize - 1)

	currentOffset := alignedHeaders
	currentAddr := w.baseAddr + currentOffset

	// Interp (for dynamic only)
	if w.isDynamic {
		interpSize := len(w.eb.getInterpreterPath()) + 1
		w.layout["interp"] = SegmentLayout{currentOffset, currentAddr, interpSize}
		currentOffset += uint64((interpSize + 7) & ^7)
		currentAddr += uint64((interpSize + 7) & ^7)
	}

	// Code section
	w.layout["text"] = SegmentLayout{currentOffset, currentAddr, codeSize}
	currentOffset += uint64((codeSize + 7) & ^7)
	currentAddr += uint64((codeSize + 7) & ^7)

	// Rodata section
	w.layout["rodata"] = SegmentLayout{currentOffset, currentAddr, rodataSize}
	currentOffset += uint64((rodataSize + 7) & ^7)
	currentAddr += uint64((rodataSize + 7) & ^7)

	// Data section (writable globals)
	dataSize := 0
	for _, value := range dataSymbols {
		dataSize += len(value)
	}
	w.layout["data"] = SegmentLayout{currentOffset, currentAddr, dataSize}

	// Entry point (typically in text section)
	if textLayout, ok := w.layout["text"]; ok {
		w.entryPoint = textLayout.addr
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "=== ELF Layout (baseAddr=0x%x) ===\n", w.baseAddr)
		for name, seg := range w.layout {
			fmt.Fprintf(os.Stderr, "  %s: offset=0x%x addr=0x%x size=%d\n",
				name, seg.offset, seg.addr, seg.size)
		}
	}

	return nil
}

// GetLayout returns layout information for a section
func (w *ELFWriter) GetLayout(section string) (SegmentLayout, bool) {
	layout, ok := w.layout[section]
	return layout, ok
}

// WriteDynamicELF writes a complete dynamic ELF file
func (w *ELFWriter) WriteDynamicELF() error {
	if !w.isDynamic {
		return fmt.Errorf("WriteDynamicELF called but isDynamic=false")
	}

	w.phase = PhaseWriting

	// This would call the actual ELF writing code
	// For now, we'll integrate with the existing WriteCompleteDynamicELF

	return nil
}

// Validate performs sanity checks on the ELF structure
func (w *ELFWriter) Validate() error {
	// Check that layout is sensible
	if len(w.layout) == 0 {
		return fmt.Errorf("ELF layout not calculated")
	}

	// Check that segments don't overlap
	// Check that addresses are page-aligned where necessary
	// etc.

	return nil
}
