package source

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// // // // // // // // // //

var rootFileNameArr = []string{"openapi.yaml", "openapi.yml", "openapi.json"}
var manifestFileNameMap = map[string]struct{}{
	"meta.json":   {},
	"meta.yml":    {},
	"meta.yaml":   {},
	"values.json": {},
	"values.yml":  {},
	"values.yaml": {},
}

// //

func resolveSource(rawSource string) (string, string, bool, error) {
	sourcePath, err := cleanAbs(rawSource)
	if err != nil {
		return "", "", false, err
	}

	infoObj, err := os.Stat(sourcePath)
	if err != nil {
		return "", "", false, fmt.Errorf("stat source: %w", err)
	}

	if infoObj.IsDir() {
		rootFile, err := findRootFile(sourcePath)
		if err != nil {
			return "", "", false, err
		}

		return sourcePath, rootFile, true, nil
	}

	if !isSourceExt(sourcePath) {
		return "", "", false, fmt.Errorf("root OpenAPI file must use .json, .yml, or .yaml: %s", sourcePath)
	}

	return filepath.Dir(sourcePath), sourcePath, false, nil
}

func findRootFile(sourceRoot string) (string, error) {
	foundArr := make([]string, 0, len(rootFileNameArr))
	for _, fileName := range rootFileNameArr {
		filePath := filepath.Join(sourceRoot, fileName)
		infoObj, err := os.Stat(filePath)
		if err == nil {
			if infoObj.IsDir() {
				return "", fmt.Errorf("root OpenAPI candidate is a directory: %s", filePath)
			}

			foundArr = append(foundArr, filePath)
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("stat root OpenAPI candidate %s: %w", filePath, err)
		}
	}

	switch len(foundArr) {
	case 1:
		return foundArr[0], nil
	case 0:
		return "", fmt.Errorf("root OpenAPI file not found in %s", sourceRoot)
	default:
		return "", fmt.Errorf("multiple root OpenAPI files found in %s (use explicit file source)", sourceRoot)
	}
}

func cleanAbs(rawPath string) (string, error) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		cwdPath, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("read working directory: %w", err)
		}

		rawPath = cwdPath
	}

	absPath, err := filepath.Abs(rawPath)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}

	return filepath.Clean(absPath), nil
}

func rejectSymlinkPath(sourceRoot string, absPath string) error {
	relPath, err := filepath.Rel(sourceRoot, absPath)
	if err != nil {
		return fmt.Errorf("resolve relative path for %s: %w", absPath, err)
	}
	if relPath == "." {
		relPath = ""
	}
	if relPath != "" && (strings.HasPrefix(relPath, ".."+string(filepath.Separator)) || relPath == ".." || filepath.IsAbs(relPath)) {
		return fmt.Errorf("source file escapes source root: %s", absPath)
	}

	rootInfoObj, err := os.Lstat(sourceRoot)
	if err != nil {
		return fmt.Errorf("lstat source root: %w", err)
	}
	if rootInfoObj.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("symlink in source graph is not allowed: %s", sourceRoot)
	}

	currentPath := sourceRoot
	if relPath == "" {
		return nil
	}

	for _, partText := range strings.Split(relPath, string(filepath.Separator)) {
		if partText == "" || partText == "." {
			continue
		}

		currentPath = filepath.Join(currentPath, partText)
		infoObj, err := os.Lstat(currentPath)
		if err != nil {
			return fmt.Errorf("lstat source graph path %s: %w", currentPath, err)
		}
		if infoObj.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in source graph is not allowed: %s", currentPath)
		}
	}

	return nil
}

func relativeSlash(sourceRoot string, absPath string) (string, error) {
	relPath, err := filepath.Rel(sourceRoot, absPath)
	if err != nil {
		return "", fmt.Errorf("resolve relative path for %s: %w", absPath, err)
	}
	if strings.HasPrefix(relPath, ".."+string(filepath.Separator)) || relPath == ".." || filepath.IsAbs(relPath) {
		return "", fmt.Errorf("source file escapes source root: %s", absPath)
	}

	return filepath.ToSlash(relPath), nil
}

func isRootFileName(nameText string) bool {
	for _, rootName := range rootFileNameArr {
		if nameText == rootName {
			return true
		}
	}

	return false
}

func isManifestFileName(nameText string) bool {
	_, existsFlag := manifestFileNameMap[nameText]
	return existsFlag
}
