//go:build darwin
// +build darwin

package main

import (
	"runtime"
)

func GetNumCPUCores() int {
	return runtime.NumCPU()
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
