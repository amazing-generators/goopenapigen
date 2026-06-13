package write

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// // // // // // // // // //

// PrepareDir ensures the output directory exists; force mode creates it.
func PrepareDir(outputDir string, force bool) error {
	infoObj, err := os.Stat(outputDir)
	switch {
	case err == nil:
		if !infoObj.IsDir() {
			return fmt.Errorf("output path is not directory: %s", outputDir)
		}
		return nil
	case errors.Is(err, os.ErrNotExist):
		if !force {
			return fmt.Errorf("output directory does not exist: %s (use -force to create it)", outputDir)
		}
		if mkdirErr := os.MkdirAll(outputDir, 0o755); mkdirErr != nil {
			return fmt.Errorf("create output directory: %w", mkdirErr)
		}
		return nil
	default:
		return fmt.Errorf("stat output directory: %w", err)
	}
}

// CleanupStale removes .go files with our header when they are absent from the new set.
// Foreign files and files without the header are left untouched.
func CleanupStale(outputDir string, keepNameMap map[string]struct{}, force bool) error {
	entryObjArr, err := os.ReadDir(outputDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read output directory: %w", err)
	}

	staleNameArr := make([]string, 0)
	for _, entryObj := range entryObjArr {
		if entryObj.IsDir() {
			continue
		}

		nameText := entryObj.Name()
		if !strings.HasSuffix(nameText, ".go") {
			continue
		}
		if _, keepFlag := keepNameMap[nameText]; keepFlag {
			continue
		}

		pathText := filepath.Join(outputDir, nameText)
		generatedFlag, headerErr := HasGeneratedHeader(pathText)
		if headerErr != nil {
			return fmt.Errorf("read stale generated file [%s]: %w", nameText, headerErr)
		}
		if !generatedFlag {
			continue
		}

		if !force {
			staleNameArr = append(staleNameArr, nameText)
			continue
		}
		if err = os.Remove(pathText); err != nil {
			return fmt.Errorf("remove stale generated file [%s]: %w", nameText, err)
		}
	}

	if len(staleNameArr) > 0 {
		sort.Strings(staleNameArr)
		return fmt.Errorf("stale generated files would be removed with force: %v", staleNameArr)
	}
	return nil
}

// HasGeneratedHeader reads only the file prefix instead of loading the whole file.
func HasGeneratedHeader(pathText string) (bool, error) {
	fileObj, err := os.Open(pathText)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = fileObj.Close()
	}()

	bufferArr := make([]byte, len(GeneratedHeader))
	readCount, err := io.ReadFull(fileObj, bufferArr)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return false, err
	}

	return bytes.Equal(bufferArr[:readCount], []byte(GeneratedHeader)), nil
}
