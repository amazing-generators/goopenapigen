package source

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// // // // // // // // // //

// DefaultMaxRefDepth is the default $ref chain depth.
const DefaultMaxRefDepth = 16

// //

func Load(optionsObj OptionsObj) (*GraphObj, error) {
	contextObj := optionsObj.Context
	if contextObj == nil {
		contextObj = context.Background()
	}
	if err := contextObj.Err(); err != nil {
		return nil, err
	}

	maxRefDepth := optionsObj.MaxRefDepth
	if maxRefDepth <= 0 {
		maxRefDepth = DefaultMaxRefDepth
	}

	sourceRoot, rootFile, _, err := resolveSource(optionsObj.Source)
	if err != nil {
		return nil, err
	}

	loaderObj := newLoader(sourceRoot, rootFile)
	loaderObj.markRootRelative(rootFile)

	rootDocument, err := loaderObj.loadDocument(rootFile)
	if err != nil {
		return nil, err
	}

	rootMap, ok := rootDocument.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("root OpenAPI document must be object")
	}

	if err = validateOpenAPIVersion(rootMap); err != nil {
		return nil, err
	}

	if err = loadRootSidecars(contextObj, loaderObj, rootMap); err != nil {
		return nil, err
	}

	if err = mergeFolderField(contextObj, loaderObj, rootMap, "paths"); err != nil {
		return nil, err
	}
	if err = mergeFolderField(contextObj, loaderObj, rootMap, "components"); err != nil {
		return nil, err
	}

	if err = applyComponentNameSuffixes(rootMap); err != nil {
		return nil, err
	}

	loaderObj.assembledDocument = rootMap

	if err = validateGraphRefs(contextObj, loaderObj, maxRefDepth); err != nil {
		return nil, err
	}

	fileArr, hashValue, shortHashValue, err := calculateHash(loaderObj)
	if err != nil {
		return nil, err
	}

	return &GraphObj{
		SourceRoot: sourceRoot,
		RootFile:   rootFile,
		Document:   rootMap,
		Hash:       hashValue,
		HashShort:  shortHashValue,
		FileArr:    fileArr,
		loaderObj:  loaderObj,
	}, nil
}

// //

func validateOpenAPIVersion(rootMap map[string]any) error {
	rawValue, existsFlag := rootMap["openapi"]
	if !existsFlag {
		return fmt.Errorf("root OpenAPI document has no openapi field")
	}

	versionText, ok := rawValue.(string)
	if !ok {
		return fmt.Errorf("root openapi field must be string")
	}

	versionText = strings.TrimSpace(versionText)
	if strings.HasPrefix(versionText, "3.0.") || strings.HasPrefix(versionText, "3.1.") {
		return nil
	}

	return fmt.Errorf("unsupported OpenAPI version: %s", versionText)
}

func loadRootSidecars(contextObj context.Context, loaderObj *loaderObj, rootMap map[string]any) error {
	entryArr, err := os.ReadDir(loaderObj.sourceRoot)
	if err != nil {
		return fmt.Errorf("read source root: %w", err)
	}

	fieldFileMap := make(map[string]string)
	for _, entryObj := range entryArr {
		if err = contextObj.Err(); err != nil {
			return err
		}
		if entryObj.IsDir() || strings.HasPrefix(entryObj.Name(), ".") {
			continue
		}
		if !isSourceExt(entryObj.Name()) {
			continue
		}
		if isRootFileName(entryObj.Name()) || isManifestFileName(entryObj.Name()) {
			continue
		}

		absPath := filepath.Join(loaderObj.sourceRoot, entryObj.Name())
		absPath, err = cleanAbs(absPath)
		if err != nil {
			return err
		}
		if absPath == loaderObj.rootFile {
			continue
		}

		fieldName := strings.TrimSuffix(entryObj.Name(), filepath.Ext(entryObj.Name()))
		if fieldName == "" {
			continue
		}
		if previousPath, existsFlag := fieldFileMap[fieldName]; existsFlag {
			return fmt.Errorf("multiple root sidecar files for field %s: %s and %s", fieldName, previousPath, absPath)
		}

		fieldFileMap[fieldName] = absPath
	}

	fieldNameArr := make([]string, 0, len(fieldFileMap))
	for fieldName := range fieldFileMap {
		fieldNameArr = append(fieldNameArr, fieldName)
	}
	sort.Strings(fieldNameArr)

	for _, fieldName := range fieldNameArr {
		if _, existsFlag := rootMap[fieldName]; existsFlag {
			return fmt.Errorf("root sidecar collision for field %s", fieldName)
		}

		absPath := fieldFileMap[fieldName]
		loaderObj.markRootRelative(absPath)

		documentValue, err := loaderObj.loadDocument(absPath)
		if err != nil {
			return err
		}
		if isWrapperDocument(documentValue, fieldName) {
			return fmt.Errorf("root sidecar %s must contain field value directly, not %s wrapper", absPath, fieldName)
		}

		clonedValue, err := normalizeRootSidecarValue(fieldName, documentValue, absPath)
		if err != nil {
			return err
		}
		if err = rewriteExternalRefsToRoot(loaderObj, clonedValue, absPath); err != nil {
			return err
		}

		rootMap[fieldName] = clonedValue
	}

	return nil
}

func isWrapperDocument(documentValue any, fieldName string) bool {
	documentMap, ok := documentValue.(map[string]any)
	if !ok || len(documentMap) != 1 {
		return false
	}

	_, existsFlag := documentMap[fieldName]
	return existsFlag
}

func normalizeRootSidecarValue(fieldName string, documentValue any, filePath string) (any, error) {
	if fieldName == "tags" {
		return normalizeTagsSidecar(documentValue, filePath)
	}

	return cloneAny(documentValue), nil
}

func normalizeTagsSidecar(documentValue any, filePath string) (any, error) {
	tagMap, ok := documentValue.(map[string]any)
	if !ok {
		return cloneAny(documentValue), nil
	}

	keyArr := make([]string, 0, len(tagMap))
	for key := range tagMap {
		keyArr = append(keyArr, key)
	}
	sort.Strings(keyArr)

	resultArr := make([]any, 0, len(keyArr))
	for _, key := range keyArr {
		tagItemMap, ok := cloneAny(tagMap[key]).(map[string]any)
		if !ok {
			return nil, fmt.Errorf("root sidecar %s tag %q must contain object", filePath, key)
		}

		if rawNameValue, existsFlag := tagItemMap["name"]; existsFlag {
			nameText, ok := rawNameValue.(string)
			if !ok || strings.TrimSpace(nameText) == "" {
				return nil, fmt.Errorf("root sidecar %s tag %q name must be non-empty string", filePath, key)
			}
		} else {
			tagItemMap["name"] = key
		}

		resultArr = append(resultArr, tagItemMap)
	}

	return resultArr, nil
}
