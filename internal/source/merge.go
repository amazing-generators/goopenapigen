package source

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// // // // // // // // // //

var pathHTTPMethodNameMap = map[string]struct{}{
	"get": {}, "put": {}, "post": {}, "delete": {}, "options": {}, "head": {}, "patch": {}, "trace": {},
}
var pathItemFieldNameMap = map[string]struct{}{
	"$ref": {}, "summary": {}, "description": {}, "servers": {}, "parameters": {},
}
var componentSectionNameMap = map[string]struct{}{
	"schemas":         {},
	"responses":       {},
	"parameters":      {},
	"examples":        {},
	"requestBodies":   {},
	"headers":         {},
	"securitySchemes": {},
	"links":           {},
	"callbacks":       {},
	"pathItems":       {},
}

// //

func mergeFolderField(contextObj context.Context, loaderObj *loaderObj, rootMap map[string]any, fieldName string) error {
	dirPath := filepath.Join(loaderObj.sourceRoot, fieldName)
	infoObj, err := os.Stat(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("stat merge directory %s: %w", dirPath, err)
	}
	if !infoObj.IsDir() {
		return fmt.Errorf("merge path is not a directory: %s", dirPath)
	}

	if err = rejectSymlinkPath(loaderObj.sourceRoot, dirPath); err != nil {
		return err
	}

	filePathArr, err := listMergeFiles(contextObj, loaderObj.sourceRoot, dirPath, fieldName)
	if err != nil {
		return err
	}
	if len(filePathArr) == 0 {
		return nil
	}

	targetMap, err := ensureRootMapField(rootMap, fieldName)
	if err != nil {
		return err
	}

	for _, filePath := range filePathArr {
		if err = contextObj.Err(); err != nil {
			return err
		}

		loaderObj.markRootRelative(filePath)

		documentValue, err := loaderObj.loadDocument(filePath)
		if err != nil {
			return err
		}

		documentMap, ok := documentValue.(map[string]any)
		if !ok {
			return fmt.Errorf("merge file %s must contain object", filePath)
		}

		clonedValue := cloneAny(documentMap)
		clonedMap, _ := clonedValue.(map[string]any)
		if err = rewriteExternalRefsToRoot(loaderObj, clonedMap, filePath); err != nil {
			return err
		}

		sourceMap, err := normalizeMergeFileMap(fieldName, dirPath, filePath, clonedMap)
		if err != nil {
			return err
		}

		if err = mergeFieldMap(fieldName, targetMap, sourceMap, []string{fieldName}, filePath); err != nil {
			return err
		}
	}

	return nil
}

func ensureRootMapField(rootMap map[string]any, fieldName string) (map[string]any, error) {
	rawValue, existsFlag := rootMap[fieldName]
	if !existsFlag {
		resultMap := make(map[string]any)
		rootMap[fieldName] = resultMap
		return resultMap, nil
	}

	targetMap, ok := rawValue.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("root field %s must be object when %s/ merge directory exists", fieldName, fieldName)
	}

	return targetMap, nil
}

func mergeFieldMap(fieldName string, targetMap map[string]any, sourceMap map[string]any, pathArr []string, filePath string) error {
	keyArr := make([]string, 0, len(sourceMap))
	for key := range sourceMap {
		keyArr = append(keyArr, key)
	}
	sort.Strings(keyArr)

	for _, key := range keyArr {
		sourceValue := sourceMap[key]
		if targetValue, existsFlag := targetMap[key]; existsFlag {
			if isMergeCollision(fieldName, pathArr) {
				return fmt.Errorf("merge collision at %s.%s from %s", strings.Join(pathArr, "."), key, filePath)
			}

			targetInnerMap, targetOK := targetValue.(map[string]any)
			sourceInnerMap, sourceOK := sourceValue.(map[string]any)
			if !targetOK || !sourceOK {
				return fmt.Errorf("merge collision at %s.%s from %s", strings.Join(pathArr, "."), key, filePath)
			}

			if err := mergeFieldMap(fieldName, targetInnerMap, sourceInnerMap, append(pathArr, key), filePath); err != nil {
				return err
			}
			continue
		}

		targetMap[key] = sourceValue
	}

	return nil
}

func isMergeCollision(fieldName string, pathArr []string) bool {
	switch fieldName {
	case "paths":
		return len(pathArr) == 1
	case "components":
		return len(pathArr) == 2
	default:
		return false
	}
}

func normalizeMergeFileMap(fieldName string, dirPath string, filePath string, sourceMap map[string]any) (map[string]any, error) {
	switch fieldName {
	case "paths":
		return normalizePathMergeFileMap(dirPath, filePath, sourceMap)
	case "components":
		return normalizeComponentMergeFileMap(dirPath, filePath, sourceMap)
	default:
		return sourceMap, nil
	}
}

func normalizePathMergeFileMap(dirPath string, filePath string, sourceMap map[string]any) (map[string]any, error) {
	if isExplicitPathsMap(sourceMap) {
		if err := validateExplicitPathsMap(filePath, sourceMap); err != nil {
			return nil, err
		}

		return sourceMap, nil
	}
	if isPathItemMap(sourceMap) {
		routeText, err := routeFromPathFile(dirPath, filePath)
		if err != nil {
			return nil, err
		}

		return map[string]any{routeText: sourceMap}, nil
	}

	return nil, fmt.Errorf("paths merge file %s must contain a path item object or explicit /path keys", filePath)
}

func normalizeComponentMergeFileMap(dirPath string, filePath string, sourceMap map[string]any) (map[string]any, error) {
	sectionName, nestedFlag, err := nestedComponentSectionName(dirPath, filePath)
	if err != nil {
		return nil, err
	}
	if !nestedFlag {
		sectionName, existsFlag := componentSectionNameFromRootFile(filePath)
		if existsFlag {
			if _, hasWrapperFlag := sourceMap[sectionName]; hasWrapperFlag {
				return sourceMap, nil
			}

			return map[string]any{sectionName: sourceMap}, nil
		}

		return sourceMap, nil
	}
	if _, existsFlag := sourceMap[sectionName]; existsFlag {
		return sourceMap, nil
	}

	componentName, err := componentNameFromFile(dirPath, filePath, sectionName)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		sectionName: map[string]any{
			componentName: sourceMap,
		},
	}, nil
}

func nestedComponentSectionName(dirPath string, filePath string) (string, bool, error) {
	relPath, err := filepath.Rel(dirPath, filePath)
	if err != nil {
		return "", false, fmt.Errorf("resolve component path for %s: %w", filePath, err)
	}

	partArr := strings.Split(filepath.ToSlash(relPath), "/")
	if len(partArr) < 2 {
		return "", false, nil
	}

	sectionName := partArr[0]
	if _, existsFlag := componentSectionNameMap[sectionName]; !existsFlag {
		return "", false, nil
	}

	return sectionName, true, nil
}

func componentSectionNameFromRootFile(filePath string) (string, bool) {
	baseName := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	switch baseName {
	case "schemas":
		return "schemas", true
	case "responses":
		return "responses", true
	case "parameters":
		return "parameters", true
	case "examples":
		return "examples", true
	case "requestBodies", "request_bodies":
		return "requestBodies", true
	case "headers":
		return "headers", true
	case "securitySchemes", "security_schemes":
		return "securitySchemes", true
	case "links":
		return "links", true
	case "callbacks":
		return "callbacks", true
	case "pathItems", "path_items":
		return "pathItems", true
	default:
		return "", false
	}
}

func componentNameFromFile(dirPath string, filePath string, sectionName string) (string, error) {
	sectionDir := filepath.Join(dirPath, sectionName)
	relPath, err := filepath.Rel(sectionDir, filePath)
	if err != nil {
		return "", fmt.Errorf("resolve component name for %s: %w", filePath, err)
	}

	nameText := strings.TrimSuffix(filepath.ToSlash(relPath), filepath.Ext(relPath))
	partArr := strings.Split(nameText, "/")
	builderObj := strings.Builder{}
	for _, partText := range partArr {
		partName := componentNamePart(partText)
		if partName == "" {
			return "", fmt.Errorf("component file %s cannot be converted to component name", filePath)
		}

		builderObj.WriteString(partName)
	}

	resultText := builderObj.String()
	if resultText == "" {
		return "", fmt.Errorf("component file %s cannot be converted to component name", filePath)
	}
	resultText = appendComponentNameSuffix(resultText, sectionName)
	firstRune, _ := utf8.DecodeRuneInString(resultText)
	if !unicode.IsLetter(firstRune) {
		return "", fmt.Errorf("component file %s produces invalid component name %s", filePath, resultText)
	}

	return resultText, nil
}

// applyComponentNameSuffixes appends domain suffixes (Obj/RespObj) to ALL components in
// a section regardless of how they are authored (standalone file or wrapper), so the
// public names are uniform. $ref values to the pre-suffix names are rewritten later
// during bundling/validation via canonicalRootPointer.
func applyComponentNameSuffixes(rootMap map[string]any) error {
	componentsMap, ok := rootMap["components"].(map[string]any)
	if !ok {
		return nil
	}

	for _, sectionName := range []string{"schemas", "responses"} {
		sectionMap, ok := componentsMap[sectionName].(map[string]any)
		if !ok {
			continue
		}

		suffixedMap, err := suffixSectionNames(sectionName, sectionMap)
		if err != nil {
			return err
		}
		componentsMap[sectionName] = suffixedMap
	}

	return nil
}

func suffixSectionNames(sectionName string, sectionMap map[string]any) (map[string]any, error) {
	resultMap := make(map[string]any, len(sectionMap))
	for _, nameText := range sortedMapKeys(sectionMap) {
		suffixedName := appendComponentNameSuffix(nameText, sectionName)
		if _, existsFlag := resultMap[suffixedName]; existsFlag {
			return nil, fmt.Errorf("component name collision after suffixing: components.%s.%s", sectionName, suffixedName)
		}

		resultMap[suffixedName] = sectionMap[nameText]
	}

	return resultMap, nil
}

func appendComponentNameSuffix(nameText string, sectionName string) string {
	suffixText := componentNameSuffix(sectionName)
	if suffixText == "" || strings.HasSuffix(nameText, suffixText) {
		return nameText
	}

	return nameText + suffixText
}

func componentNameSuffix(sectionName string) string {
	switch sectionName {
	case "schemas":
		return "Obj"
	case "responses":
		return "RespObj"
	default:
		return ""
	}
}

func componentNamePart(rawText string) string {
	builderObj := strings.Builder{}
	upperNextFlag := true
	for _, symbol := range rawText {
		if !unicode.IsLetter(symbol) && !unicode.IsDigit(symbol) {
			upperNextFlag = true
			continue
		}
		if upperNextFlag {
			builderObj.WriteRune(unicode.ToUpper(symbol))
			upperNextFlag = false
			continue
		}

		builderObj.WriteRune(symbol)
	}

	return builderObj.String()
}

func isExplicitPathsMap(sourceMap map[string]any) bool {
	for keyText := range sourceMap {
		if strings.HasPrefix(keyText, "/") {
			return true
		}
	}

	return false
}

func validateExplicitPathsMap(filePath string, sourceMap map[string]any) error {
	for keyText := range sourceMap {
		if strings.HasPrefix(keyText, "/") || strings.HasPrefix(keyText, "x-") {
			continue
		}

		return fmt.Errorf("paths merge file %s mixes explicit /path keys with non-path key %s", filePath, keyText)
	}

	return nil
}

func isPathItemMap(sourceMap map[string]any) bool {
	for keyText := range sourceMap {
		if _, existsFlag := pathHTTPMethodNameMap[strings.ToLower(keyText)]; existsFlag {
			return true
		}
		if _, existsFlag := pathItemFieldNameMap[keyText]; existsFlag {
			return true
		}
	}

	return false
}

func routeFromPathFile(pathsDir string, filePath string) (string, error) {
	relPath, err := filepath.Rel(pathsDir, filePath)
	if err != nil {
		return "", fmt.Errorf("resolve path route for %s: %w", filePath, err)
	}

	relPath = filepath.ToSlash(relPath)
	routeText := strings.TrimSuffix(relPath, filepath.Ext(relPath))
	routeText = strings.TrimSpace(routeText)
	if routeText == "" {
		return "", fmt.Errorf("path route is empty: %s", filePath)
	}

	partArr := strings.Split(routeText, "/")
	for index, partText := range partArr {
		partText = strings.TrimSpace(partText)
		if partText == "" {
			return "", fmt.Errorf("path route has empty segment: %s", filePath)
		}

		partArr[index] = normalizePathRouteSegment(partText)
	}

	return "/" + strings.Join(partArr, "/"), nil
}

func normalizePathRouteSegment(segmentText string) string {
	if len(segmentText) > 2 && strings.HasPrefix(segmentText, "[") && strings.HasSuffix(segmentText, "]") {
		return "{" + segmentText[1:len(segmentText)-1] + "}"
	}
	if len(segmentText) > 1 && strings.HasPrefix(segmentText, "$") {
		return "{" + segmentText[1:] + "}"
	}

	return segmentText
}

func listMergeFiles(contextObj context.Context, sourceRoot string, dirPath string, fieldName string) ([]string, error) {
	resultArr := make([]string, 0)

	err := filepath.WalkDir(dirPath, func(pathText string, entryObj os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := contextObj.Err(); err != nil {
			return err
		}

		if shouldSkipMergeEntry(fieldName, entryObj) {
			if entryObj.IsDir() {
				return filepath.SkipDir
			}

			return nil
		}

		infoObj, err := entryObj.Info()
		if err != nil {
			return fmt.Errorf("stat merge entry %s: %w", pathText, err)
		}
		if infoObj.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in source graph is not allowed: %s", pathText)
		}

		if entryObj.IsDir() {
			return nil
		}
		if !isSourceExt(entryObj.Name()) {
			return nil
		}

		absPath, err := cleanAbs(pathText)
		if err != nil {
			return err
		}
		if err = rejectSymlinkPath(sourceRoot, absPath); err != nil {
			return err
		}

		resultArr = append(resultArr, absPath)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk merge directory %s: %w", dirPath, err)
	}

	sort.Strings(resultArr)
	return resultArr, nil
}

func shouldSkipMergeEntry(fieldName string, entryObj os.DirEntry) bool {
	nameText := entryObj.Name()
	if nameText == "." || !strings.HasPrefix(nameText, ".") {
		return false
	}

	return fieldName != "paths" || nameText != ".well-known"
}
