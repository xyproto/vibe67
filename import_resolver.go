// Completion: 100% - Import resolution module complete
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ImportSpec represents a parsed import statement
type ImportSpec struct {
	Source  string // The import path/URL
	Version string // Version specifier (@v1.0.0, @main, @latest, etc.)
	Alias   string // Optional alias (from "as alias")
	IsLocal bool   // True if local path (starts with ., /, or is absolute)
}

// ParseImportSource parses an import source string and returns an ImportSpec
// Handles:
// - "sdl3" -> library import
// - "github.com/user/repo" -> git repo
// - "github.com/user/repo@v1.0.0" -> git repo with version
// - "git@github.com:user/repo.git" -> SSH format git repo
// - "." or "./path" or "/path" -> local directory
// - "/path/to/lib.so" -> C library file
func ParseImportSource(source string) (*ImportSpec, error) {
	spec := &ImportSpec{Source: source}

	// Handle version specifier
	if idx := strings.Index(source, "@"); idx != -1 {
		spec.Source = source[:idx]
		spec.Version = source[idx+1:]
	}

	// Check if it's a local path
	if strings.HasPrefix(spec.Source, ".") || strings.HasPrefix(spec.Source, "/") || filepath.IsAbs(spec.Source) {
		spec.IsLocal = true
	}

	return spec, nil
}

// ResolveImport resolves an import and returns the path to the resolved files
// Priority: libraries first → git repos → directories
func ResolveImport(spec *ImportSpec, targetOS, targetArch string) ([]string, error) {
	// Try library import first (highest priority)
	if paths, err := tryResolveLibrary(spec, targetOS); err == nil && len(paths) > 0 {
		return paths, nil
	}

	// Try git repository import second
	if !spec.IsLocal && isGitURL(spec.Source) {
		return resolveGitRepo(spec)
	}

	// Try directory import third (lowest priority)
	if spec.IsLocal || isLikelyDirectory(spec.Source) {
		return resolveDirectory(spec)
	}

	// Last resort: try as library name
	if paths, err := tryResolveLibrary(spec, targetOS); err == nil && len(paths) > 0 {
		return paths, nil
	}

	return nil, fmt.Errorf("could not resolve import: %s", spec.Source)
}

// tryResolveLibrary attempts to resolve an import as a library
// Uses pkg-config on Linux/macOS, searches for .dll on Windows
func tryResolveLibrary(spec *ImportSpec, targetOS string) ([]string, error) {
	libName := spec.Source

	// Check if it's a .so or .dll file path
	if strings.HasSuffix(libName, ".so") || strings.Contains(libName, ".so.") ||
		strings.HasSuffix(libName, ".dll") || strings.HasSuffix(libName, ".dylib") {
		// Absolute library file path
		if _, err := os.Stat(libName); err == nil {
			return []string{libName}, nil
		}
		return nil, fmt.Errorf("library file not found: %s", libName)
	}

	// Platform-specific library resolution
	if targetOS == "windows" {
		return resolveWindowsLibrary(libName)
	}

	// Try pkg-config first (Linux/macOS)
	if paths := resolvePkgConfig(libName); len(paths) > 0 {
		return paths, nil
	}

	// Try standard library paths
	return resolveSystemLibrary(libName)
}

// resolvePkgConfig uses pkg-config to find a library
func resolvePkgConfig(libName string) []string {
	// Try pkg-config
	cmd := exec.Command("pkg-config", "--cflags-only-I", libName)
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	// Parse -I flags to find header directories
	var paths []string
	for _, flag := range strings.Fields(string(output)) {
		if strings.HasPrefix(flag, "-I") {
			headerPath := strings.TrimPrefix(flag, "-I")
			// Look for .h files in this directory
			matches, err := filepath.Glob(filepath.Join(headerPath, "*.h"))
			if err == nil && len(matches) > 0 {
				paths = append(paths, matches...)
			}
		}
	}

	return paths
}

// resolveSystemLibrary searches standard system library paths
func resolveSystemLibrary(libName string) ([]string, error) {
	standardPaths := []string{
		"./include",
		"/usr/include",
		"/usr/local/include",
		"/opt/local/include",
	}

	for _, basePath := range standardPaths {
		// Try direct header file
		headerPath := filepath.Join(basePath, libName+".h")
		if _, err := os.Stat(headerPath); err == nil {
			return []string{headerPath}, nil
		}

		// Try subdirectory with headers
		dirPath := filepath.Join(basePath, libName)
		if info, err := os.Stat(dirPath); err == nil && info.IsDir() {
			// Find all .h files in the directory
			matches, err := filepath.Glob(filepath.Join(dirPath, "*.h"))
			if err == nil && len(matches) > 0 {
				return matches, nil
			}
		}
	}

	return nil, fmt.Errorf("library not found: %s", libName)
}

// resolveWindowsLibrary searches for .dll files on Windows
func resolveWindowsLibrary(libName string) ([]string, error) {
	// Check current directory first
	dllName := libName + ".dll"
	if _, err := os.Stat(dllName); err == nil {
		return []string{dllName}, nil
	}

	// Check current directory for library-specific DLLs
	upperLib := strings.ToUpper(libName)
	potentialDLLs := []string{
		fmt.Sprintf("%s.dll", upperLib),
		fmt.Sprintf("%s.dll", libName),
		fmt.Sprintf("lib%s.dll", libName),
	}
	for _, dll := range potentialDLLs {
		if _, err := os.Stat(dll); err == nil {
			return []string{dll}, nil
		}
	}

	// Check Windows system directories
	systemPaths := []string{
		os.Getenv("WINDIR") + "\\System32",
		os.Getenv("WINDIR") + "\\SysWOW64",
	}

	for _, sysPath := range systemPaths {
		dllPath := filepath.Join(sysPath, dllName)
		if _, err := os.Stat(dllPath); err == nil {
			return []string{dllPath}, nil
		}
	}

	return nil, fmt.Errorf("windows library not found: %s", libName)
}

// isGitURL checks if a string looks like a git repository URL
func isGitURL(source string) bool {
	// Check for common git URL patterns
	return strings.Contains(source, "github.com") ||
		strings.Contains(source, "gitlab.com") ||
		strings.Contains(source, "bitbucket.org") ||
		strings.HasPrefix(source, "git@") ||
		strings.HasSuffix(source, ".git")
}

// isLikelyDirectory checks if a path is likely a directory
func isLikelyDirectory(source string) bool {
	// Check if it's a path-like string
	return strings.Contains(source, "/") || strings.Contains(source, "\\")
}

// resolveGitRepo clones or updates a git repository and returns paths to .vibe67 files
func resolveGitRepo(spec *ImportSpec) ([]string, error) {
	// Normalize the URL
	repoURL := spec.Source

	// Handle SSH format: git@github.com:user/repo.git -> github.com/user/repo
	if strings.HasPrefix(repoURL, "git@") {
		repoURL = strings.TrimPrefix(repoURL, "git@")
		repoURL = strings.Replace(repoURL, ":", "/", 1)
		repoURL = strings.TrimSuffix(repoURL, ".git")
	}

	// Add https:// if no protocol
	if !strings.HasPrefix(repoURL, "http://") && !strings.HasPrefix(repoURL, "https://") {
		repoURL = "https://" + repoURL
	}

	// Clone or update the repository
	var repoPath string
	var err error
	if spec.Version != "" {
		repoPath, err = EnsureRepoClonedWithVersion(repoURL, spec.Version, false)
	} else {
		repoPath, err = EnsureRepoCloned(repoURL, false)
	}
	if err != nil {
		return nil, err
	}

	// Find all top-level .vibe67 files
	return findVibe67Files(repoPath, false)
}

// resolveDirectory resolves a local directory or file import and returns paths to .vibe67 files
func resolveDirectory(spec *ImportSpec) ([]string, error) {
	dirPath := spec.Source

	// Handle "." as current directory
	if dirPath == "." {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get working directory: %w", err)
		}
		dirPath = wd
	}

	// Make absolute if relative
	if !filepath.IsAbs(dirPath) {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get working directory: %w", err)
		}
		dirPath = filepath.Join(wd, dirPath)
	}

	// Check if path exists
	info, err := os.Stat(dirPath)
	if err != nil {
		return nil, fmt.Errorf("path not found: %s", dirPath)
	}

	// If it's a .vibe67 file, return it directly
	if !info.IsDir() {
		if strings.HasSuffix(dirPath, ".vibe67") {
			return []string{dirPath}, nil
		}
		return nil, fmt.Errorf("not a .vibe67 file or directory: %s", dirPath)
	}

	// Find all top-level .vibe67 files (not recursive for directories)
	return findVibe67Files(dirPath, true)
}

// findVibe67Files finds all .vibe67 files in a directory
// If topLevelOnly is true, only returns files in the root of the directory
func findVibe67Files(dirPath string, topLevelOnly bool) ([]string, error) {
	var files []string

	if topLevelOnly {
		// Only look in the root directory
		entries, err := os.ReadDir(dirPath)
		if err != nil {
			return nil, err
		}

		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".vibe67") {
				// Exclude test files (test_*.vibe67) and generated files (_*.vibe67)
				name := entry.Name()
				if !strings.HasPrefix(name, "test_") && !strings.HasPrefix(name, "_") {
					files = append(files, filepath.Join(dirPath, entry.Name()))
				}
			}
		}
	} else {
		// Recursively find all .vibe67 files
		err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && strings.HasSuffix(path, ".vibe67") {
				// Exclude test files (test_*.vibe67) and generated files (_*.vibe67)
				baseName := filepath.Base(path)
				if !strings.HasPrefix(baseName, "test_") && !strings.HasPrefix(baseName, "_") {
					files = append(files, path)
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no .vibe67 files found in: %s", dirPath)
	}

	return files, nil
}
