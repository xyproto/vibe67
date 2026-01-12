// Completion: 100% - Utility module complete
package engine

import (
	"hash/fnv"
	"sort"
	"strings"
)

// utils.go - Utility helper functions
//
// This file contains general-purpose utility functions used throughout
// the compiler for string operations, hashing, and similarity matching.

// hashStringKey hashes a string identifier to a uint64 for use as a map key.
// Uses FNV-1a hash algorithm for deterministic, collision-resistant hashing.
// Currently limited to 30-bit hash due to compiler integer literal limitations.
// Sets bit 30 to distinguish symbolic keys from typical numeric indices.
func hashStringKey(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	// Use FNV-1a 32-bit variant for now, mask to 30 bits (0x3FFFFFFF)
	// Then set bit 30 (0x40000000) to distinguish symbolic keys
	// This gives us range 0x40000000 to 0x7FFFFFFF (1073741824 to 2147483647)
	h32 := fnv.New32a()
	h32.Write([]byte(s))
	return uint64((h32.Sum32() & 0x3FFFFFFF) | 0x40000000)
}

// levenshteinDistance calculates the edit distance between two strings
func levenshteinDistance(s1, s2 string) int {
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}

	// Create matrix
	matrix := make([][]int, len(s1)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(s2)+1)
	}

	// Initialize first row and column
	for i := 0; i <= len(s1); i++ {
		matrix[i][0] = i
	}
	for j := 0; j <= len(s2); j++ {
		matrix[0][j] = j
	}

	// Fill matrix
	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			cost := 1
			if s1[i-1] == s2[j-1] {
				cost = 0
			}
			matrix[i][j] = min(
				matrix[i-1][j]+1, // deletion
				min(matrix[i][j-1]+1, // insertion
					matrix[i-1][j-1]+cost)) // substitution
		}
	}

	return matrix[len(s1)][len(s2)]
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// findSimilarIdentifiers finds identifiers similar to the given name
func findSimilarIdentifiers(name string, availableVars map[string]int, maxSuggestions int) []string {
	type suggestion struct {
		name     string
		distance int
	}

	var suggestions []suggestion
	threshold := 3 // Maximum edit distance for suggestions

	for varName := range availableVars {
		dist := levenshteinDistance(name, varName)
		if dist <= threshold && dist > 0 {
			suggestions = append(suggestions, suggestion{varName, dist})
		}
	}

	// Sort by distance (closest first)
	sort.Slice(suggestions, func(i, j int) bool {
		if suggestions[i].distance == suggestions[j].distance {
			return suggestions[i].name < suggestions[j].name
		}
		return suggestions[i].distance < suggestions[j].distance
	})

	// Return top suggestions
	result := make([]string, 0, maxSuggestions)
	for i := 0; i < len(suggestions) && i < maxSuggestions; i++ {
		result = append(result, suggestions[i].name)
	}
	return result
}

// isUppercase checks if an identifier is all uppercase (constant naming convention)
func isUppercase(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, ch := range s {
		if ch >= 'a' && ch <= 'z' {
			return false
		}
	}
	return true
}

func isAllUppercase(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, ch := range s {
		if ch >= 'a' && ch <= 'z' {
			return false
		}
		if ch >= 'A' && ch <= 'Z' {
			continue
		}
		if ch >= '0' && ch <= '9' {
			continue
		}
		if ch == '_' {
			continue
		}
		return false
	}
	// Must start with uppercase letter
	firstCh := rune(s[0])
	return firstCh >= 'A' && firstCh <= 'Z'
}

// deriveAliasFromSource extracts a suitable alias from an import source
// Examples:
// - "github.com/user/repo" -> "repo"
// - "github.com/user/repo@v1.0.0" -> "repo"
// - "sdl3" -> "sdl3"
// - "./mylib" -> "mylib"
func deriveAliasFromSource(source string) string {
	// Remove version suffix if present
	if idx := strings.Index(source, "@"); idx != -1 {
		source = source[:idx]
	}

	// Remove trailing slashes
	source = strings.TrimRight(source, "/\\")

	// Get the last component
	if lastSlash := strings.LastIndexAny(source, "/\\"); lastSlash != -1 {
		return source[lastSlash+1:]
	}

	// No slashes, use as-is
	return source
}
