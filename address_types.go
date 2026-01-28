// address_types.go - Strongly typed addresses to prevent mixing file offsets, virtual addresses, and text offsets
package main

import "fmt"

// VirtualAddr represents an address in virtual memory (e.g., 0x403000)
type VirtualAddr uint64

// FileOffset represents an offset in the ELF file (e.g., 0x3000)
type FileOffset uint64

// TextOffset represents an offset within the .text buffer (e.g., 0x9b)
type TextOffset uint64

// RodataOffset represents an offset within the .rodata buffer
type RodataOffset uint64

// DataOffset represents an offset within the .data buffer
type DataOffset uint64

func (v VirtualAddr) String() string {
	return fmt.Sprintf("0x%x", uint64(v))
}

func (f FileOffset) String() string {
	return fmt.Sprintf("file:0x%x", uint64(f))
}

func (t TextOffset) String() string {
	return fmt.Sprintf("text:0x%x", uint64(t))
}

// AddressSpace tracks the mapping between different address spaces
type AddressSpace struct {
	baseAddr     VirtualAddr // Base virtual address (e.g., 0x400000)
	textFileOff  FileOffset  // Where .text starts in file
	textVirtAddr VirtualAddr // Where .text is loaded in memory
}

func NewAddressSpace(base VirtualAddr, textFile FileOffset, textVirt VirtualAddr) *AddressSpace {
	return &AddressSpace{
		baseAddr:     base,
		textFileOff:  textFile,
		textVirtAddr: textVirt,
	}
}

// TextOffsetToVirtAddr converts a text buffer offset to virtual address
func (as *AddressSpace) TextOffsetToVirtAddr(offset TextOffset) VirtualAddr {
	return as.textVirtAddr + VirtualAddr(offset)
}

// FileOffsetToVirtAddr converts a file offset to virtual address
func (as *AddressSpace) FileOffsetToVirtAddr(offset FileOffset) VirtualAddr {
	return as.baseAddr + VirtualAddr(offset)
}

// VirtAddrToFileOffset converts a virtual address to file offset
func (as *AddressSpace) VirtAddrToFileOffset(addr VirtualAddr) FileOffset {
	if addr < as.baseAddr {
		panic(fmt.Sprintf("Virtual address %s is before base %s", addr, as.baseAddr))
	}
	return FileOffset(addr - as.baseAddr)
}
