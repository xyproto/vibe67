// Completion: 100% - Utility module complete
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// FunctionRepository maps function names to Git repository URLs
// When the compiler encounters an unknown function, it looks it up here
// and automatically fetches the repository containing the implementation
var FunctionRepository = map[string]string{
	// Math functions
	// Keep these as auto-dependencies (can be pure Vibe67 implementations):
	"abs": "github.com/xyproto/vibe67_math", // Simple: x < 0 { -> -x ~> x }
	"min": "github.com/xyproto/vibe67_math",
	"max": "github.com/xyproto/vibe67_math",

	// These have excellent x87 FPU instruction support - consider making builtin:
	// sqrt: SQRTSD (SSE2)
	// sin, cos, tan: FSIN, FCOS, FPTAN (x87)
	// asin, acos, atan: FPATAN (x87)
	// log, exp: FYL2X, F2XM1 (x87)
	// pow: FYL2X + F2XM1 (x87)
	// floor, ceil, round: FRNDINT (x87)
	"sqrt":  "github.com/xyproto/vibe67_math",
	"pow":   "github.com/xyproto/vibe67_math",
	"sin":   "github.com/xyproto/vibe67_math",
	"cos":   "github.com/xyproto/vibe67_math",
	"tan":   "github.com/xyproto/vibe67_math",
	"asin":  "github.com/xyproto/vibe67_math",
	"acos":  "github.com/xyproto/vibe67_math",
	"atan":  "github.com/xyproto/vibe67_math",
	"atan2": "github.com/xyproto/vibe67_math",
	"log":   "github.com/xyproto/vibe67_math",
	"log10": "github.com/xyproto/vibe67_math",
	"exp":   "github.com/xyproto/vibe67_math",
	"floor": "github.com/xyproto/vibe67_math",
	"ceil":  "github.com/xyproto/vibe67_math",
	"round": "github.com/xyproto/vibe67_math",

	// Standard library
	// "println": "github.com/xyproto/vibe67_core",  // Commented out - println is now a builtin
	"print": "github.com/xyproto/vibe67_core", // Print without newline

	// Graphics (example)
	"InitWindow":    "github.com/xyproto/vibe67_raylib",
	"CloseWindow":   "github.com/xyproto/vibe67_raylib",
	"DrawRectangle": "github.com/xyproto/vibe67_raylib",
}

// GetFunctionRepository returns the repository URL for a function
// Checks environment variable FLAPC_FUNCTIONNAME first, then falls back to FunctionRepository map
// Example: FLAPC_PRINTLN=github.com/xyproto/vibe67_alternative_core overrides the default
func GetFunctionRepository(funcName string) (string, bool) {
	// Check for environment variable override
	// Convert function name to uppercase for env var: println -> PRINTLN
	envVarName := "FLAPC_" + strings.ToUpper(funcName)
	if repoURL := os.Getenv(envVarName); repoURL != "" {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "Using environment override for %s: %s=%s\n", funcName, envVarName, repoURL)
		}
		return repoURL, true
	}

	// Fall back to FunctionRepository map
	repoURL, ok := FunctionRepository[funcName]
	return repoURL, ok
}

// GetCachePath returns the cache directory for vibe67 dependencies
// Respects XDG_CACHE_HOME environment variable
// Default: $XDG_CACHE_HOME/vibe67 or ~/.cache/vibe67/
func GetCachePath() (string, error) {
	// Check XDG_CACHE_HOME first
	if xdgCache := os.Getenv("XDG_CACHE_HOME"); xdgCache != "" {
		return filepath.Join(xdgCache, "vibe67"), nil
	}

	// Fall back to ~/.cache/vibe67
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	cachePath := filepath.Join(homeDir, ".cache", "vibe67")
	return cachePath, nil
}

// GetRepoCachePath returns the local path for a cloned repository
// Example: "github.com/xyproto/vibe67_math" -> "~/.cache/vibe67/github.com/xyproto/vibe67_math"
func GetRepoCachePath(repoURL string) (string, error) {
	cachePath, err := GetCachePath()
	if err != nil {
		return "", err
	}

	// Remove protocol prefix if present
	repoURL = strings.TrimPrefix(repoURL, "https://")
	repoURL = strings.TrimPrefix(repoURL, "http://")
	repoURL = strings.TrimPrefix(repoURL, "git://")

	// Remove .git suffix if present
	repoURL = strings.TrimSuffix(repoURL, ".git")

	return filepath.Join(cachePath, repoURL), nil
}

// EnsureRepoCloned ensures a repository is cloned to the cache
// If already cloned, does nothing (unless updateDeps is true)
func EnsureRepoCloned(repoURL string, updateDeps bool) (string, error) {
	repoPath, err := GetRepoCachePath(repoURL)
	if err != nil {
		return "", err
	}

	// Check if repo already exists
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err == nil {
		// Repo exists
		if updateDeps {
			if err := GitPull(repoPath); err != nil {
				return "", fmt.Errorf("failed to update %s: %w", repoURL, err)
			}
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "Updated dependency: %s\n", repoURL)
			}
		}
		return repoPath, nil
	}

	// Repo doesn't exist, clone it
	if err := GitClone(repoURL, repoPath); err != nil {
		return "", fmt.Errorf("failed to clone %s: %w", repoURL, err)
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Cloned dependency: %s\n", repoURL)
	}
	return repoPath, nil
}

// EnsureRepoClonedWithVersion ensures a repository is cloned and checked out at a specific version
// version can be: tag (e.g., "v1.0.0"), branch (e.g., "main"), commit hash, "latest", "HEAD", or ""
// Empty string or "latest" means use the latest tag (or main/master if no tags)
func EnsureRepoClonedWithVersion(repoURL, version string, updateDeps bool) (string, error) {
	repoPath, err := GetRepoCachePath(repoURL)
	if err != nil {
		return "", err
	}

	// Check if repo already exists
	repoExists := false
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err == nil {
		repoExists = true
	}

	if !repoExists {
		// Repo doesn't exist, clone it with the specific version
		if err := GitCloneWithVersion(repoURL, repoPath, version); err != nil {
			return "", fmt.Errorf("failed to clone %s: %w", repoURL, err)
		}
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "Cloned dependency: %s", repoURL)
		}
		if version != "" {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, " at %s", version)
			}
		}
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "\n")
		}
	} else {
		// Repo exists
		if updateDeps {
			// Fetch updates
			fetchCmd := exec.Command("git", "-C", repoPath, "fetch", "--all", "--tags")
			fetchCmd.Stdout = os.Stderr
			fetchCmd.Stderr = os.Stderr
			if err := fetchCmd.Run(); err != nil {
				return "", fmt.Errorf("failed to fetch updates for %s: %w", repoURL, err)
			}
		}

		// Checkout the requested version
		checkoutRef := resolveVersionRef(version, repoPath)
		if checkoutRef != "" {
			checkoutCmd := exec.Command("git", "-C", repoPath, "checkout", checkoutRef)
			checkoutCmd.Stdout = os.Stderr
			checkoutCmd.Stderr = os.Stderr
			if err := checkoutCmd.Run(); err != nil {
				return "", fmt.Errorf("failed to checkout %s in %s: %w", checkoutRef, repoURL, err)
			}
			if updateDeps {
				if VerboseMode {
					fmt.Fprintf(os.Stderr, "Updated dependency: %s at %s\n", repoURL, checkoutRef)
				}
			}
		}
	}

	return repoPath, nil
}

// resolveVersionRef converts a version string to a git ref
// "" or "latest" -> use smart detection (latest tag > main > master)
// "HEAD" -> "HEAD"
// anything else -> as-is (tag/branch/commit)
func resolveVersionRef(version, repoPath string) string {
	if version == "" || version == "latest" {
		// Try to get latest tag
		if latestTag, err := getLatestTagInRepo(repoPath); err == nil && latestTag != "" {
			return latestTag
		}
		// Fall back to main or master
		if remoteBranchExists(repoPath, "origin/main") {
			return "origin/main"
		}
		if remoteBranchExists(repoPath, "origin/master") {
			return "origin/master"
		}
		return "" // Let git use default
	}
	if version == "HEAD" {
		return "HEAD"
	}
	return version
}

// GitCloneWithVersion clones a repository and checks out a specific version
func GitCloneWithVersion(repoURL, destPath, version string) error {
	// Create parent directory
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Build full clone URL
	cloneURL := repoURL
	if !strings.HasPrefix(repoURL, "http://") &&
		!strings.HasPrefix(repoURL, "https://") &&
		!strings.HasPrefix(repoURL, "git://") &&
		!strings.HasPrefix(repoURL, "git@") {
		cloneURL = "https://" + repoURL
	}

	// Handle version-specific cloning
	if version == "" || version == "latest" {
		// Use smart detection (latest tag > main > master)
		// First do a bare clone to discover refs
		cmd := exec.Command("git", "clone", "--bare", cloneURL, destPath+".tmp")
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("git clone failed: %w (tried to clone %s)", err, cloneURL)
		}

		// Determine best ref
		checkoutRef, err := determineCheckoutRef(destPath + ".tmp")
		if err != nil {
			os.RemoveAll(destPath + ".tmp")
			return fmt.Errorf("failed to determine checkout ref: %w", err)
		}

		// Remove temp bare clone
		os.RemoveAll(destPath + ".tmp")

		// Clone with determined ref
		if checkoutRef != "" {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "Cloning %s at %s...\n", repoURL, checkoutRef)
			}
			cloneCmd := exec.Command("git", "clone", "--depth=1", "--branch", checkoutRef, cloneURL, destPath)
			cloneCmd.Stdout = os.Stderr
			cloneCmd.Stderr = os.Stderr
			if err := cloneCmd.Run(); err != nil {
				return fmt.Errorf("git clone failed: %w", err)
			}
		} else {
			cloneCmd := exec.Command("git", "clone", "--depth=1", cloneURL, destPath)
			cloneCmd.Stdout = os.Stderr
			cloneCmd.Stderr = os.Stderr
			if err := cloneCmd.Run(); err != nil {
				return fmt.Errorf("git clone failed: %w", err)
			}
		}
	} else {
		// Try cloning with the specific version as a branch/tag first
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "Cloning %s at %s...\n", repoURL, version)
		}
		cloneCmd := exec.Command("git", "clone", "--depth=1", "--branch", version, cloneURL, destPath)
		cloneCmd.Stdout = os.Stderr
		cloneCmd.Stderr = os.Stderr

		if err := cloneCmd.Run(); err != nil {
			// Branch/tag didn't work, might be a commit hash
			// Do a full clone and then checkout
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "Not a branch/tag, trying as commit...\n")
			}
			cloneCmd = exec.Command("git", "clone", cloneURL, destPath)
			cloneCmd.Stdout = os.Stderr
			cloneCmd.Stderr = os.Stderr
			if err := cloneCmd.Run(); err != nil {
				return fmt.Errorf("git clone failed: %w", err)
			}

			// Try to checkout the commit
			checkoutCmd := exec.Command("git", "-C", destPath, "checkout", version)
			checkoutCmd.Stdout = os.Stderr
			checkoutCmd.Stderr = os.Stderr
			if err := checkoutCmd.Run(); err != nil {
				return fmt.Errorf("git checkout %s failed: %w", version, err)
			}
		}
	}

	return nil
}

// GitClone clones a Git repository to the specified path
// Strategy: Use latest tag if available, otherwise main branch, otherwise master
func GitClone(repoURL, destPath string) error {
	// Create parent directory
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Build full clone URL (add https:// if needed)
	cloneURL := repoURL
	if !strings.HasPrefix(repoURL, "http://") &&
		!strings.HasPrefix(repoURL, "https://") &&
		!strings.HasPrefix(repoURL, "git://") &&
		!strings.HasPrefix(repoURL, "git@") {
		cloneURL = "https://" + repoURL
	}

	// First, do a shallow clone (just enough to discover tags and branches)
	cmd := exec.Command("git", "clone", "--bare", cloneURL, destPath+".tmp")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone failed: %w (tried to clone %s)", err, cloneURL)
	}

	// Determine what to checkout: latest tag > main > master
	checkoutRef, err := determineCheckoutRef(destPath + ".tmp")
	if err != nil {
		// Cleanup temp bare repo
		os.RemoveAll(destPath + ".tmp")
		return fmt.Errorf("failed to determine checkout ref: %w", err)
	}

	// Remove the temporary bare clone
	os.RemoveAll(destPath + ".tmp")

	// Now clone with the determined ref
	var cloneCmd *exec.Cmd
	if checkoutRef != "" {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "Cloning %s at %s...\n", repoURL, checkoutRef)
		}
		cloneCmd = exec.Command("git", "clone", "--depth=1", "--branch", checkoutRef, cloneURL, destPath)
	} else {
		// No specific ref, let git use default
		cloneCmd = exec.Command("git", "clone", "--depth=1", cloneURL, destPath)
	}
	cloneCmd.Stdout = os.Stderr
	cloneCmd.Stderr = os.Stderr

	if err := cloneCmd.Run(); err != nil {
		return fmt.Errorf("git clone failed: %w (tried to clone %s at %s)", err, cloneURL, checkoutRef)
	}

	return nil
}

// determineCheckoutRef determines which ref to checkout
// Priority: latest tag > main > master > empty (git default)
func determineCheckoutRef(bareRepoPath string) (string, error) {
	// Try to get the latest tag
	latestTag, err := getLatestTag(bareRepoPath)
	if err == nil && latestTag != "" {
		return latestTag, nil
	}

	// Try main branch
	if branchExists(bareRepoPath, "main") {
		return "main", nil
	}

	// Try master branch
	if branchExists(bareRepoPath, "master") {
		return "master", nil
	}

	// Let git use its default
	return "", nil
}

// getLatestTag returns the latest semver tag from a bare repository
func getLatestTag(bareRepoPath string) (string, error) {
	cmd := exec.Command("git", "--git-dir", bareRepoPath, "tag", "--sort=-version:refname")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	tags := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(tags) > 0 && tags[0] != "" {
		return tags[0], nil
	}

	return "", fmt.Errorf("no tags found")
}

// branchExists checks if a branch exists in a bare repository
func branchExists(bareRepoPath, branchName string) bool {
	cmd := exec.Command("git", "--git-dir", bareRepoPath, "show-ref", "--verify", "refs/heads/"+branchName)
	err := cmd.Run()
	return err == nil
}

// GitPull updates an existing Git repository
// Fetches latest tags and updates to latest tag, or latest main/master
func GitPull(repoPath string) error {
	// Fetch all updates including tags
	fetchCmd := exec.Command("git", "-C", repoPath, "fetch", "--all", "--tags")
	fetchCmd.Stdout = os.Stderr
	fetchCmd.Stderr = os.Stderr

	if err := fetchCmd.Run(); err != nil {
		return fmt.Errorf("git fetch failed: %w", err)
	}

	// Determine the best ref to checkout
	latestTag, err := getLatestTagInRepo(repoPath)
	var checkoutRef string
	if err == nil && latestTag != "" {
		checkoutRef = latestTag
	} else {
		// Try origin/main, then origin/master
		if remoteBranchExists(repoPath, "origin/main") {
			checkoutRef = "origin/main"
		} else if remoteBranchExists(repoPath, "origin/master") {
			checkoutRef = "origin/master"
		} else {
			return fmt.Errorf("no suitable branch found (tried main, master)")
		}
	}

	// Checkout the determined ref
	checkoutCmd := exec.Command("git", "-C", repoPath, "checkout", checkoutRef)
	checkoutCmd.Stdout = os.Stderr
	checkoutCmd.Stderr = os.Stderr

	if err := checkoutCmd.Run(); err != nil {
		return fmt.Errorf("git checkout %s failed: %w", checkoutRef, err)
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Updated to %s\n", checkoutRef)
	}
	return nil
}

// getLatestTagInRepo returns the latest tag from a working repository
func getLatestTagInRepo(repoPath string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "tag", "--sort=-version:refname")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	tags := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(tags) > 0 && tags[0] != "" {
		return tags[0], nil
	}

	return "", fmt.Errorf("no tags found")
}

// remoteBranchExists checks if a remote branch exists
func remoteBranchExists(repoPath, branchName string) bool {
	cmd := exec.Command("git", "-C", repoPath, "show-ref", "--verify", "refs/remotes/"+branchName)
	err := cmd.Run()
	return err == nil
}

// FindVibe67Files returns all .vibe67 files in a directory (recursively)
func FindVibe67Files(dir string) ([]string, error) {
	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories (like .git)
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") && path != dir {
			return filepath.SkipDir
		}

		// Add .vibe67 files
		if !info.IsDir() && strings.HasSuffix(path, ".vibe67") {
			files = append(files, path)
		}

		return nil
	})

	return files, err
}

// ResolveFunction looks up a function name and returns its repository URL
// Returns empty string if function is not in the repository map
// Checks environment variable FLAPC_FUNCTIONNAME first via GetFunctionRepository
func ResolveFunction(funcName string) string {
	if repoURL, ok := GetFunctionRepository(funcName); ok {
		return repoURL
	}
	return ""
}

// ResolveDependencies takes a list of unknown functions and returns
// unique repository URLs that need to be cloned
func ResolveDependencies(unknownFunctions []string) []string {
	repoSet := make(map[string]bool)

	for _, funcName := range unknownFunctions {
		if repoURL := ResolveFunction(funcName); repoURL != "" {
			repoSet[repoURL] = true
		}
	}

	// Convert set to slice
	repos := make([]string, 0, len(repoSet))
	for repo := range repoSet {
		repos = append(repos, repo)
	}

	return repos
}
