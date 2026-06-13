package source

import (
	"fmt"
	"path/filepath"
	"strings"
)

// // // // // // // // // //

func rewriteExternalRefsToRoot(loaderObj *loaderObj, value any, sourceFile string) error {
	switch castedValue := value.(type) {
	case map[string]any:
		if rawRefValue, existsFlag := castedValue["$ref"]; existsFlag {
			refText, ok := rawRefValue.(string)
			if !ok {
				return fmt.Errorf("$ref value must be string in %s", sourceFile)
			}

			nextRefText, err := rewriteRefToRoot(loaderObj, sourceFile, refText)
			if err != nil {
				return err
			}
			castedValue["$ref"] = nextRefText
		}

		for _, innerValue := range castedValue {
			if err := rewriteExternalRefsToRoot(loaderObj, innerValue, sourceFile); err != nil {
				return err
			}
		}
	case []any:
		for _, innerValue := range castedValue {
			if err := rewriteExternalRefsToRoot(loaderObj, innerValue, sourceFile); err != nil {
				return err
			}
		}
	}

	return nil
}

func rewriteRefToRoot(loaderObj *loaderObj, sourceFile string, refText string) (string, error) {
	if isRemoteRef(refText) {
		return refText, nil
	}

	refPath, fragmentText := splitRef(refText)
	if strings.TrimSpace(refPath) == "" {
		return refText, nil
	}

	targetPath, err := resolveRefPath(loaderObj, sourceFile, refPath)
	if err != nil {
		return "", err
	}

	relPath, err := filepath.Rel(loaderObj.sourceRoot, targetPath)
	if err != nil {
		return "", fmt.Errorf("resolve relative ref path: %w", err)
	}

	relPath = filepath.ToSlash(relPath)
	if !strings.HasPrefix(relPath, ".") {
		relPath = "./" + relPath
	}
	if fragmentText != "" {
		return relPath + "#" + fragmentText, nil
	}

	return relPath, nil
}
