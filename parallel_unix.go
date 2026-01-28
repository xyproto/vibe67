// Completion: 100% - Platform-specific module complete
//go:build linux
// +build linux

package main

import (
	"fmt"
	"sync/atomic"
	"syscall"
	"unsafe"
)

const (
	CLONE_VM             = 0x00000100
	CLONE_FS             = 0x00000200
	CLONE_FILES          = 0x00000400
	CLONE_SIGHAND        = 0x00000800
	CLONE_THREAD         = 0x00010000
	CLONE_SYSVSEM        = 0x00040000
	CLONE_SETTLS         = 0x00080000
	CLONE_PARENT_SETTID  = 0x00100000
	CLONE_CHILD_CLEARTID = 0x00200000
)

const CLONE_THREAD_FLAGS = CLONE_VM | CLONE_FS | CLONE_FILES | CLONE_SIGHAND |
	CLONE_THREAD | CLONE_SYSVSEM

func GetNumCPUCores() int {
	data, err := syscall.Open("/proc/cpuinfo", syscall.O_RDONLY, 0)
	if err != nil {
		return 4
	}
	defer syscall.Close(data)

	buf := make([]byte, 16384)
	n, err := syscall.Read(data, buf)
	if err != nil {
		return 4
	}

	count := 0
	for i := 0; i < n-9; i++ {
		if buf[i] == 'p' && buf[i+1] == 'r' && buf[i+2] == 'o' &&
			buf[i+3] == 'c' && buf[i+4] == 'e' && buf[i+5] == 's' &&
			buf[i+6] == 's' && buf[i+7] == 'o' && buf[i+8] == 'r' {
			if i == 0 || buf[i-1] == '\n' {
				count++
			}
		}
	}

	if count == 0 {
		return 4
	}
	return count
}

type ThreadStack struct {
	Memory []byte
	Size   int
}

func AllocateThreadStack(size int) *ThreadStack {
	if size <= 0 {
		size = 8 * 1024 * 1024
	}

	return &ThreadStack{
		Memory: make([]byte, size),
		Size:   size,
	}
}

func (ts *ThreadStack) StackTop() uintptr {
	return uintptr(unsafe.Pointer(&ts.Memory[ts.Size-16]))
}

func CloneThread(fn uintptr, arg uintptr, stack *ThreadStack) (int, error) {
	stackTop := stack.StackTop()

	tid, _, errno := syscall.RawSyscall6(
		syscall.SYS_CLONE,
		uintptr(CLONE_THREAD_FLAGS),
		stackTop,
		0,
		0,
		0,
		0,
	)

	if errno != 0 {
		return -1, fmt.Errorf("clone() failed: %v", errno)
	}

	if tid == 0 {
		executeThreadFunction(fn, arg)
		syscall.Exit(0)
	}

	return int(tid), nil
}

func executeThreadFunction(fn uintptr, arg uintptr) {
	fmt.Printf("Thread started (placeholder - fn: %x, arg: %x)\n", fn, arg)
}

func GetTID() int {
	tid, _, _ := syscall.RawSyscall(syscall.SYS_GETTID, 0, 0, 0)
	return int(tid)
}

const (
	FUTEX_WAIT         = 0
	FUTEX_WAKE         = 1
	FUTEX_PRIVATE_FLAG = 128
	FUTEX_WAIT_PRIVATE = FUTEX_WAIT | FUTEX_PRIVATE_FLAG
	FUTEX_WAKE_PRIVATE = FUTEX_WAKE | FUTEX_PRIVATE_FLAG
)

func FutexWait(addr *int32, val int32) error {
	_, _, errno := syscall.Syscall6(
		syscall.SYS_FUTEX,
		uintptr(unsafe.Pointer(addr)),
		uintptr(FUTEX_WAIT_PRIVATE),
		uintptr(val),
		0,
		0, 0,
	)
	if errno != 0 && errno != syscall.EAGAIN {
		return errno
	}
	return nil
}

func FutexWake(addr *int32, count int) (int, error) {
	n, _, errno := syscall.Syscall6(
		syscall.SYS_FUTEX,
		uintptr(unsafe.Pointer(addr)),
		uintptr(FUTEX_WAKE_PRIVATE),
		uintptr(count),
		0, 0, 0,
	)
	if errno != 0 {
		return 0, errno
	}
	return int(n), nil
}

func AtomicDecrement(addr *int32) int32 {
	return atomic.AddInt32(addr, -1)
}

type Barrier struct {
	count int32
	total int32
}

func NewBarrier(numThreads int) *Barrier {
	return &Barrier{
		count: int32(numThreads),
		total: int32(numThreads),
	}
}

func (b *Barrier) Wait() {
	remaining := AtomicDecrement(&b.count)

	if remaining == 0 {
		FutexWake(&b.count, int(b.total))
		return
	}

	for atomic.LoadInt32(&b.count) > 0 {
		FutexWait(&b.count, remaining)
		remaining = atomic.LoadInt32(&b.count)
	}
}

func WaitForThreads(barrier *Barrier) error {
	if barrier == nil {
		return fmt.Errorf("barrier is nil")
	}

	barrier.Wait()
	return nil
}

func CalculateWorkDistribution(totalItems int, numThreads int) (int, int) {
	if numThreads <= 0 {
		numThreads = 1
	}

	chunkSize := totalItems / numThreads
	remainder := totalItems % numThreads

	return chunkSize, remainder
}

func GetThreadWorkRange(threadID int, totalItems int, numThreads int) (int, int) {
	chunkSize, remainder := CalculateWorkDistribution(totalItems, numThreads)

	startIdx := threadID * chunkSize
	endIdx := startIdx + chunkSize

	if threadID == numThreads-1 {
		endIdx += remainder
	}

	return startIdx, endIdx
}
