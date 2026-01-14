// Completion: 100% - Utility module complete
package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

var errNoAssembly = errors.New("no Assembly given")

func (bw *BufferWrapper) Write(b byte) int {
	bw.buf.Write([]byte{b})
	if VerboseMode {
		fmt.Fprintf(os.Stderr, " %x", b)
	}
	return 1
}

func (bw *BufferWrapper) WriteN(b byte, n int) int {
	for i := 0; i < n; i++ {
		bw.Write(b)
	}
	return n
}

func (bw *BufferWrapper) Write2(b byte) int {
	bw.buf.Write([]byte{b, 0})
	if VerboseMode {
		fmt.Fprintf(os.Stderr, " %x %x", b, 0)
	}
	return 2
}

func (bw *BufferWrapper) Write4(b byte) int {
	bw.buf.Write([]byte{b, 0, 0, 0})
	if VerboseMode {
		fmt.Fprintf(os.Stderr, " %x %x %x %x", b, 0, 0, 0)
	}
	return 4
}

func (bw *BufferWrapper) Write8(b byte) int {
	bw.buf.Write([]byte{b, 0, 0, 0, 0, 0, 0, 0})
	if VerboseMode {
		fmt.Fprintf(os.Stderr, " %x %x %x %x %x %x %x %x", b, 0, 0, 0, 0, 0, 0, 0)
	}
	return 8
}

func (bw *BufferWrapper) Write8u(v uint64) int {
	binary.Write(bw.buf, binary.LittleEndian, v)
	if VerboseMode {
		fmt.Fprintf(os.Stderr, " %x", v)
	}
	return 8
}

func (bw *BufferWrapper) WriteBytes(bs []byte) int {
	bw.buf.Write(bs)
	if VerboseMode {
		for _, b := range bs {
			fmt.Fprintf(os.Stderr, " %x", b)
		}
	}
	return len(bs)
}

func (eb *ExecutableBuilder) PrependBytes(bs []byte) {
	var newBuf bytes.Buffer
	newBuf.Write(bs)
	newBuf.Write(eb.text.Bytes())
	eb.text = newBuf
}

func (bw *BufferWrapper) WriteUnsigned(i uint) int {
	a := byte(i & 0xff)
	b := byte((i >> 8) & 0xff)
	c := byte((i >> 16) & 0xff)
	d := byte((i >> 24) & 0xff)
	bw.buf.Write([]byte{a, b, c, d})
	if VerboseMode {
		fmt.Fprintf(os.Stderr, " %x %x %x %x", a, b, c, d)
	}
	return 4
}

func (eb *ExecutableBuilder) Emit(assembly string) error {
	trimmed := strings.TrimSpace(assembly)
	if trimmed == "" {
		return errNoAssembly
	}

	w := eb.TextWriter()
	all := strings.Fields(trimmed)
	head := strings.TrimSpace(all[0])
	var tail []string
	if len(all) > 1 {
		tail = all[1:]
	}
	rest := strings.TrimSpace(trimmed[len(head):])

	if len(all) == 1 {
		switch head {
		case "syscall", "ecall":
			if VerboseMode {
				fmt.Fprint(os.Stderr, assembly+":")
			}
			out := NewOut(eb.target, eb.TextWriter(), eb)
			out.Syscall()
			if VerboseMode {
				fmt.Fprintln(os.Stderr)
			}
		}
	} else if len(all) == 2 {
		switch head {
		case "call", "bl":
			funcName := strings.TrimSpace(tail[0])
			return eb.GenerateCallInstruction(funcName)
		case "svc":
			// ARM64 svc instruction with immediate (e.g., svc #0x80)
			immStr := strings.TrimSpace(tail[0])
			immStr = strings.TrimPrefix(immStr, "#")
			var immVal uint64
			if val, err := strconv.ParseUint(immStr, 0, 16); err == nil {
				immVal = val
			}
			// ARM64 SVC instruction: 1101 0100 000 imm16 000 01
			// Encoding: 0xD4000001 | (imm16 << 5)
			instr := uint32(0xD4000001) | (uint32(immVal&0xFFFF) << 5)
			w.Write(uint8(instr & 0xFF))
			w.Write(uint8((instr >> 8) & 0xFF))
			w.Write(uint8((instr >> 16) & 0xFF))
			w.Write(uint8((instr >> 24) & 0xFF))
			return nil
		}
	} else if len(all) == 3 {
		switch head {
		case "mov":
			dest := strings.TrimSuffix(strings.TrimSpace(tail[0]), ",")
			val := strings.TrimSpace(tail[1])
			return eb.MovInstruction(dest, val)
		}
	}

	out := NewOut(eb.target, w, eb)

	switch head {
	case "vmovupd":
		if rest == "" {
			return fmt.Errorf("vmovupd requires operands")
		}
		operands := splitOperands(rest)
		if len(operands) != 2 {
			return fmt.Errorf("vmovupd expects 2 operands, got %d", len(operands))
		}

		op0 := operands[0]
		op1 := operands[1]

		if isMemoryOperand(op1) {
			base, offset, err := parseMemoryOperand(op1)
			if err != nil {
				return err
			}
			out.VMovupdLoadFromMem(op0, base, offset)
			return nil
		}

		if isMemoryOperand(op0) {
			base, offset, err := parseMemoryOperand(op0)
			if err != nil {
				return err
			}
			out.VMovupdStoreToMem(op1, base, offset)
			return nil
		}

		return fmt.Errorf("vmovupd requires one memory operand")

	case "vbroadcastsd":
		if rest == "" {
			return fmt.Errorf("vbroadcastsd requires operands")
		}
		operands := splitOperands(rest)
		if len(operands) != 2 {
			return fmt.Errorf("vbroadcastsd expects 2 operands, got %d", len(operands))
		}

		dst := operands[0]
		src := operands[1]

		if isMemoryOperand(src) {
			base, offset, err := parseMemoryOperand(src)
			if err != nil {
				return err
			}
			out.VBroadcastSDMemToVector(dst, base, offset)
			return nil
		}

		out.VBroadcastSDScalarToVector(dst, src)
		return nil

	case "vaddpd":
		if rest == "" {
			return fmt.Errorf("vaddpd requires operands")
		}
		operands := splitOperands(rest)
		if len(operands) != 3 {
			return fmt.Errorf("vaddpd expects 3 operands, got %d", len(operands))
		}
		out.VAddPDVectorToVector(operands[0], operands[1], operands[2])
		return nil

	case "vmulpd":
		if rest == "" {
			return fmt.Errorf("vmulpd requires operands")
		}
		operands := splitOperands(rest)
		if len(operands) != 3 {
			return fmt.Errorf("vmulpd expects 3 operands, got %d", len(operands))
		}
		out.VMulPDVectorToVector(operands[0], operands[1], operands[2])
		return nil

	case "vsubpd":
		if rest == "" {
			return fmt.Errorf("vsubpd requires operands")
		}
		operands := splitOperands(rest)
		if len(operands) != 3 {
			return fmt.Errorf("vsubpd expects 3 operands, got %d", len(operands))
		}
		out.VSubPDVectorToVector(operands[0], operands[1], operands[2])
		return nil

	case "vdivpd":
		if rest == "" {
			return fmt.Errorf("vdivpd requires operands")
		}
		operands := splitOperands(rest)
		if len(operands) != 3 {
			return fmt.Errorf("vdivpd expects 3 operands, got %d", len(operands))
		}
		out.VDivPDVectorToVector(operands[0], operands[1], operands[2])
		return nil

	case "vcmppd":
		if rest == "" {
			return fmt.Errorf("vcmppd requires operands")
		}
		operands := splitOperands(rest)
		if len(operands) != 4 {
			return fmt.Errorf("vcmppd expects 4 operands, got %d", len(operands))
		}

		predicate, err := parseComparisonPredicate(operands[3])
		if err != nil {
			return err
		}
		out.VCmpPDVectorToVector(operands[0], operands[1], operands[2], predicate)
		return nil
	}
	return nil
}

func splitOperands(operands string) []string {
	var result []string
	currentStart := 0
	depth := 0
	for i, r := range operands {
		switch r {
		case '[':
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				part := strings.TrimSpace(operands[currentStart:i])
				if part != "" {
					result = append(result, part)
				}
				currentStart = i + 1
			}
		}
	}

	if currentStart < len(operands) {
		part := strings.TrimSpace(operands[currentStart:])
		if part != "" {
			result = append(result, part)
		}
	}

	return result
}

func isMemoryOperand(op string) bool {
	op = strings.TrimSpace(op)
	return strings.HasPrefix(op, "[") && strings.HasSuffix(op, "]")
}

func parseMemoryOperand(op string) (string, int32, error) {
	op = strings.TrimSpace(op)
	if !strings.HasPrefix(op, "[") || !strings.HasSuffix(op, "]") {
		return "", 0, fmt.Errorf("invalid memory operand %q", op)
	}

	inner := strings.TrimSpace(op[1 : len(op)-1])
	if inner == "" {
		return "", 0, fmt.Errorf("memory operand %q is empty", op)
	}

	inner = strings.ReplaceAll(inner, " ", "")

	base := inner
	offset := int64(0)
	for i := 1; i < len(inner); i++ {
		if inner[i] == '+' || inner[i] == '-' {
			base = inner[:i]
			displacement := inner[i:]
			val, err := strconv.ParseInt(displacement, 10, 32)
			if err != nil {
				return "", 0, fmt.Errorf("invalid displacement %q", displacement)
			}
			offset = val
			break
		}
	}

	base = strings.TrimSpace(base)
	if base == "" {
		return "", 0, fmt.Errorf("memory operand %q missing base register", op)
	}

	return base, int32(offset), nil
}

func parseComparisonPredicate(pred string) (ComparisonPredicate, error) {
	switch strings.ToLower(strings.TrimSpace(pred)) {
	case "eq":
		return CmpEQ, nil
	case "lt":
		return CmpLT, nil
	case "le":
		return CmpLE, nil
	case "neq":
		return CmpNE, nil
	case "nlt":
		return CmpNLT, nil
	case "nle":
		return CmpNLE, nil
	case "gt":
		return CmpGT, nil
	case "ge":
		return CmpGE, nil
	default:
		return 0, fmt.Errorf("unknown comparison predicate %q", pred)
	}
}









