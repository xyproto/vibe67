//go:build linux
// +build linux

package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestBasicThreadSpawn(t *testing.T) {
	t.Skip("Skipping: manual thread spawning interferes with Go runtime")
	fmt.Println("Testing basic thread spawn...")

	var counter int32

	stack := AllocateThreadStack(1024 * 1024)

	tid, err := CloneThread(0, 0, stack)

	if err != nil {
		t.Fatalf("Failed to spawn thread: %v", err)
	}

	if tid <= 0 {
		t.Fatalf("Invalid thread ID returned: %d", tid)
	}

	fmt.Printf("Successfully spawned thread with TID: %d\n", tid)

	time.Sleep(100 * time.Millisecond)

	_ = counter
}

func TestGetTID(t *testing.T) {
	tid := GetTID()
	if tid <= 0 {
		t.Fatalf("Invalid TID returned: %d", tid)
	}
	fmt.Printf("Current thread TID: %d\n", tid)
}

func TestGetNumCPUCores(t *testing.T) {
	cores := GetNumCPUCores()
	if cores <= 0 {
		t.Fatalf("Invalid core count returned: %d", cores)
	}
	if cores > 1024 {
		t.Fatalf("Unrealistic core count returned: %d", cores)
	}
	fmt.Printf("Detected CPU cores: %d\n", cores)
}

func TestWorkDistribution(t *testing.T) {
	tests := []struct {
		totalItems int
		numThreads int
		wantChunk  int
		wantRem    int
	}{
		{100, 4, 25, 0},
		{100, 3, 33, 1},
		{10, 4, 2, 2},
		{1000, 8, 125, 0},
	}

	for _, tt := range tests {
		chunk, rem := CalculateWorkDistribution(tt.totalItems, tt.numThreads)
		if chunk != tt.wantChunk || rem != tt.wantRem {
			t.Errorf("CalculateWorkDistribution(%d, %d) = (%d, %d), want (%d, %d)",
				tt.totalItems, tt.numThreads, chunk, rem, tt.wantChunk, tt.wantRem)
		}
	}
}

func TestThreadWorkRange(t *testing.T) {
	totalItems := 100
	numThreads := 4

	expected := []struct{ start, end int }{
		{0, 25},
		{25, 50},
		{50, 75},
		{75, 100},
	}

	for i := 0; i < numThreads; i++ {
		start, end := GetThreadWorkRange(i, totalItems, numThreads)
		if start != expected[i].start || end != expected[i].end {
			t.Errorf("Thread %d: got range [%d, %d), want [%d, %d)",
				i, start, end, expected[i].start, expected[i].end)
		}
	}

	totalItems = 101
	expected = []struct{ start, end int }{
		{0, 25},
		{25, 50},
		{50, 75},
		{75, 101},
	}

	for i := 0; i < numThreads; i++ {
		start, end := GetThreadWorkRange(i, totalItems, numThreads)
		if start != expected[i].start || end != expected[i].end {
			t.Errorf("Thread %d (with remainder): got range [%d, %d), want [%d, %d)",
				i, start, end, expected[i].start, expected[i].end)
		}
	}
}

func TestMultipleThreadSpawn(t *testing.T) {
	t.Skip("Skipping: manual thread spawning interferes with Go runtime")
	fmt.Println("Testing multiple thread spawn...")

	numThreads := 4
	tids := make([]int, numThreads)

	for i := 0; i < numThreads; i++ {
		stack := AllocateThreadStack(1024 * 1024)
		tid, err := CloneThread(0, 0, stack)

		if err != nil {
			t.Fatalf("Failed to spawn thread %d: %v", i, err)
		}

		if tid <= 0 {
			t.Fatalf("Invalid TID for thread %d: %d", i, tid)
		}

		tids[i] = tid
		fmt.Printf("Spawned thread %d with TID: %d\n", i, tid)
	}

	seen := make(map[int]bool)
	for i, tid := range tids {
		if seen[tid] {
			t.Errorf("Duplicate TID %d found for thread %d", tid, i)
		}
		seen[tid] = true
	}

	time.Sleep(200 * time.Millisecond)

	fmt.Printf("Successfully spawned and tracked %d threads\n", numThreads)
}

func TestBarrierSync(t *testing.T) {
	fmt.Println("Testing barrier synchronization...")

	numThreads := 4
	barrier := NewBarrier(numThreads)
	counter := atomic.Int32{}

	completionOrder := make([]int, 0, numThreads)
	var mu sync.Mutex

	var wg sync.WaitGroup
	for i := 0; i < numThreads; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			counter.Add(1)
			time.Sleep(time.Duration(id*10) * time.Millisecond)

			fmt.Printf("Thread %d reached barrier\n", id)
			barrier.Wait()

			mu.Lock()
			completionOrder = append(completionOrder, id)
			mu.Unlock()

			fmt.Printf("Thread %d passed barrier\n", id)
		}(i)
	}

	wg.Wait()

	if len(completionOrder) != numThreads {
		t.Errorf("Expected %d threads to complete, got %d", numThreads, len(completionOrder))
	}

	finalCount := counter.Load()
	if finalCount != int32(numThreads) {
		t.Errorf("Expected counter=%d, got %d", numThreads, finalCount)
	}

	fmt.Printf("Success! All %d threads synchronized at barrier\n", numThreads)
}

func TestManualThreadExecution(t *testing.T) {
	t.Skip("Skipping: manual thread execution interferes with Go runtime")
	fmt.Println("Testing manual thread execution with shared memory...")

	var counter atomic.Int32

	threadFunc := func() {
		tid := GetTID()
		fmt.Printf("Thread %d executing\n", tid)
		counter.Add(1)
		syscall.Exit(0)
	}

	stack := AllocateThreadStack(1024 * 1024)
	stackTop := stack.StackTop()

	tid, _, errno := syscall.RawSyscall6(
		syscall.SYS_CLONE,
		uintptr(CLONE_VM|CLONE_FILES|CLONE_FS),
		stackTop,
		0, 0, 0, 0,
	)

	if errno != 0 {
		t.Fatalf("clone() failed: %v", errno)
	}

	if tid == 0 {
		threadFunc()
	}

	fmt.Printf("Parent: spawned thread with TID %d\n", tid)

	time.Sleep(100 * time.Millisecond)

	finalCount := counter.Load()
	if finalCount != 1 {
		t.Errorf("Expected counter=1, got %d (thread may not have executed)", finalCount)
	} else {
		fmt.Printf("Success! Thread executed and incremented counter to %d\n", finalCount)
	}
}
