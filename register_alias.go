// Completion: 100% - Helper module complete
package main

// resolveRegisterAlias maps cross-platform register aliases to architecture-specific names
// Aliases provide a portable way to write unsafe blocks across x86_64, ARM64, and RISC-V
//
// Common aliases:
//
//	a, b, c, d, e, f = first 6 general purpose registers
//	s = stack pointer
//	p = frame pointer
//
// Examples:
//
//	x86_64:  a->rax, b->rbx, c->rcx, d->rdx, e->rsi, f->rdi, s->rsp, p->rbp
//	ARM64:   a->x0, b->x1, c->x2, d->x3, e->x4, f->x5, s->sp, p->fp (x29)
//	RISC-V:  a->a0, b->a1, c->a2, d->a3, e->a4, f->a5, s->sp, p->fp (s0)
func resolveRegisterAlias(alias string, arch Arch) string {
	switch arch {
	case ArchX86_64:
		return resolveX86_64Alias(alias)
	case ArchARM64:
		return resolveARM64Alias(alias)
	case ArchRiscv64:
		return resolveRISCV64Alias(alias)
	default:
		// Unknown architecture - return as-is (will fail later if invalid)
		return alias
	}
}

func resolveX86_64Alias(alias string) string {
	switch alias {
	case "a":
		return "rax"
	case "b":
		return "rbx"
	case "c":
		return "rcx"
	case "d":
		return "rdx"
	case "e":
		return "rsi"
	case "f":
		return "rdi"
	case "s":
		return "rsp"
	case "p":
		return "rbp"
	default:
		return alias // Not an alias, return original
	}
}

func resolveARM64Alias(alias string) string {
	switch alias {
	case "a":
		return "x0"
	case "b":
		return "x1"
	case "c":
		return "x2"
	case "d":
		return "x3"
	case "e":
		return "x4"
	case "f":
		return "x5"
	case "s":
		return "sp"
	case "p":
		return "fp" // ARM64 frame pointer is x29, but assemblers accept "fp"
	default:
		return alias
	}
}

func resolveRISCV64Alias(alias string) string {
	switch alias {
	case "a":
		return "a0"
	case "b":
		return "a1"
	case "c":
		return "a2"
	case "d":
		return "a3"
	case "e":
		return "a4"
	case "f":
		return "a5"
	case "s":
		return "sp"
	case "p":
		return "fp" // RISC-V frame pointer is s0, but "fp" is common alias
	default:
		return alias
	}
}
