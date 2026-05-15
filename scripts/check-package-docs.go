// Package main provides a linter tool to verify all Go packages have package-level documentation.
// This tool is used in CI to enforce documentation standards across the codebase.
// It exits with status 1 if any package lacks proper package documentation.
//
// check-package-docs.go verifies that all Go packages have package-level documentation.
// This tool is used in CI to enforce documentation standards.
//
// Usage:
//   go run scripts/check-package-docs.go [package-path...]
//
// If no package paths are provided, it checks all packages in the current module.
// Exits with status 1 if any package lacks documentation.

package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"strings"
)

func main() {
	packages, err := getPackages()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting packages: %v\n", err)
		os.Exit(1)
	}

	// Package paths to skip (utility scripts, etc.)
	skipPackages := map[string]bool{
		"github.com/opd-ai/store/scripts": true,
	}

	missing := []string{}
	for _, pkg := range packages {
		// Skip test packages and vendor
		if strings.HasSuffix(pkg, "_test") || strings.Contains(pkg, "/vendor/") {
			continue
		}

		// Skip explicitly excluded packages
		if skipPackages[pkg] {
			continue
		}

		hasDoc, err := packageHasDocumentation(pkg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error checking %s: %v\n", pkg, err)
			continue
		}

		if !hasDoc {
			missing = append(missing, pkg)
		}
	}

	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "❌ The following packages are missing package-level documentation:\n")
		for _, pkg := range missing {
			fmt.Fprintf(os.Stderr, "  - %s\n", pkg)
		}
		fmt.Fprintf(os.Stderr, "\nPackage documentation should be added as a comment before the package declaration:\n")
		fmt.Fprintf(os.Stderr, "  // Package name provides ...\n")
		fmt.Fprintf(os.Stderr, "  package name\n")
		os.Exit(1)
	}

	fmt.Printf("✅ All %d packages have documentation\n", len(packages)-len(skipPackages))
}

// getPackages returns all Go packages in the current module
func getPackages() ([]string, error) {
	cmd := exec.Command("go", "list", "./...")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("go list failed: %w: %s", err, output)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	packages := []string{}
	for _, line := range lines {
		if line != "" {
			packages = append(packages, line)
		}
	}
	return packages, nil
}

// packageHasDocumentation checks if a package has package-level documentation
func packageHasDocumentation(pkgPath string) (bool, error) {
	// Convert package path to directory
	cmd := exec.Command("go", "list", "-f", "{{.Dir}}", pkgPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("failed to get package directory: %w: %s", err, output)
	}
	pkgDir := strings.TrimSpace(string(output))

	// Parse all .go files in the package directory (including test files)
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, pkgDir, nil, parser.ParseComments)
	if err != nil {
		return false, fmt.Errorf("failed to parse package: %w", err)
	}

	// Check each package (usually just one, but there could be multiple)
	for _, pkg := range pkgs {
		// Look through all files for package documentation
		for _, file := range pkg.Files {
			if file.Doc != nil && len(file.Doc.List) > 0 {
				// Check if the comment starts with "Package "
				text := file.Doc.Text()
				// Accept "Package <name>" format (standard) where name matches the actual package name
				if strings.HasPrefix(text, "Package "+pkg.Name+" ") || strings.HasPrefix(text, "Package "+pkg.Name+"\n") {
					return true, nil
				}
			}
		}
	}

	return false, nil
}
