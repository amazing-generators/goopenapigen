package source

import (
	"fmt"
	"path/filepath"
	"strings"
)

// // // // // // // // // //

type bundleObj struct {
	loaderObj                  *loaderObj
	stackMap                   map[string]struct{}
	canonicalComponentRefsFlag bool
}

// //

func (obj *GraphObj) BundleDocument() (map[string]any, error) {
	return obj.BundleDocumentWithOptions(BundleOptionsObj{
		CanonicalComponentRefs: true,
	})
}

func (obj *GraphObj) BundleDocumentWithOptions(optionsObj BundleOptionsObj) (map[string]any, error) {
	bundleObj := &bundleObj{
		loaderObj:                  obj.loaderObj,
		stackMap:                   make(map[string]struct{}),
		canonicalComponentRefsFlag: optionsObj.CanonicalComponentRefs,
	}

	bundledValue, err := bundleObj.bundleValue(obj.CloneDocument(), obj.RootFile, true)
	if err != nil {
		return nil, err
	}

	bundledMap, ok := bundledValue.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("bundled OpenAPI document must be object")
	}

	return bundledMap, nil
}

func (obj *bundleObj) bundleValue(value any, currentFile string, rootRelativeFlag bool) (any, error) {
	switch castedValue := value.(type) {
	case map[string]any:
		rawRefValue, hasRefFlag := castedValue["$ref"]
		if hasRefFlag {
			refText, ok := rawRefValue.(string)
			if !ok {
				return nil, fmt.Errorf("$ref value must be string in %s", currentFile)
			}

			canonicalRefText, canonicalFlag, err := obj.canonicalComponentRef(currentFile, rootRelativeFlag, refText)
			if err != nil {
				return nil, err
			}
			if canonicalFlag {
				return obj.bundleRefMap(castedValue, currentFile, rootRelativeFlag, canonicalRefText)
			}

			inlineFlag, err := shouldInlineRef(refText, rootRelativeFlag)
			if err != nil {
				return nil, err
			}
			if inlineFlag {
				return obj.inlineRef(currentFile, rootRelativeFlag, refText)
			}
		}

		resultMap := make(map[string]any, len(castedValue))
		keyArr := sortedMapKeys(castedValue)
		for _, key := range keyArr {
			bundledValue, err := obj.bundleValue(castedValue[key], currentFile, rootRelativeFlag)
			if err != nil {
				return nil, err
			}
			resultMap[key] = bundledValue
		}

		return resultMap, nil
	case []any:
		resultArr := make([]any, len(castedValue))
		for index, innerValue := range castedValue {
			bundledValue, err := obj.bundleValue(innerValue, currentFile, rootRelativeFlag)
			if err != nil {
				return nil, err
			}
			resultArr[index] = bundledValue
		}

		return resultArr, nil
	default:
		return value, nil
	}
}

func (obj *bundleObj) inlineRef(currentFile string, rootRelativeFlag bool, refText string) (any, error) {
	targetObj, err := resolveRefTarget(obj.loaderObj, currentFile, rootRelativeFlag, refText)
	if err != nil {
		return nil, err
	}

	if _, existsFlag := obj.stackMap[targetObj.idText]; existsFlag {
		return nil, fmt.Errorf("recursive $ref detected while bundling: %s", targetObj.displayText)
	}

	obj.stackMap[targetObj.idText] = struct{}{}
	clonedValue := cloneAny(targetObj.value)
	resultValue, err := obj.bundleValue(clonedValue, targetObj.filePath, targetObj.rootRelativeFlag)
	delete(obj.stackMap, targetObj.idText)
	if err != nil {
		return nil, err
	}

	return resultValue, nil
}

func (obj *bundleObj) bundleRefMap(sourceMap map[string]any, currentFile string, rootRelativeFlag bool, refText string) (map[string]any, error) {
	resultMap := make(map[string]any, len(sourceMap))
	keyArr := sortedMapKeys(sourceMap)
	for _, key := range keyArr {
		if key == "$ref" {
			resultMap[key] = refText
			continue
		}

		bundledValue, err := obj.bundleValue(sourceMap[key], currentFile, rootRelativeFlag)
		if err != nil {
			return nil, err
		}
		resultMap[key] = bundledValue
	}

	return resultMap, nil
}

func (obj *bundleObj) canonicalComponentRef(currentFile string, rootRelativeFlag bool, refText string) (string, bool, error) {
	if isRemoteRef(refText) {
		return "", false, nil
	}

	refPath, fragmentText := splitRef(refText)
	if strings.TrimSpace(refPath) == "" {
		if rootRelativeFlag {
			canonicalPointerText := canonicalRootPointer(obj.loaderObj.assembledDocument, fragmentText)
			if canonicalPointerText != fragmentText {
				return "#" + canonicalPointerText, true, nil
			}
		}

		return "", false, nil
	}
	if !obj.canonicalComponentRefsFlag {
		return "", false, nil
	}

	targetFile, err := resolveRefPath(obj.loaderObj, currentFile, refPath)
	if err != nil {
		return "", false, err
	}

	return canonicalComponentRefForFile(obj.loaderObj.sourceRoot, targetFile, fragmentText)
}

func canonicalComponentRefForFile(sourceRoot string, targetFile string, fragmentText string) (string, bool, error) {
	componentsDir := filepath.Join(sourceRoot, "components")
	relPath, err := filepath.Rel(componentsDir, targetFile)
	if err != nil {
		return "", false, fmt.Errorf("resolve component ref path for %s: %w", targetFile, err)
	}
	if relPath == "." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) || relPath == ".." || filepath.IsAbs(relPath) {
		return "", false, nil
	}

	partArr := strings.Split(filepath.ToSlash(relPath), "/")
	if len(partArr) >= 2 {
		sectionName := partArr[0]
		if _, existsFlag := componentSectionNameMap[sectionName]; !existsFlag {
			return "", false, nil
		}

		componentName, err := componentNameFromFile(componentsDir, targetFile, sectionName)
		if err != nil {
			return "", false, err
		}

		return buildCanonicalComponentRef(sectionName, componentName, fragmentText), true, nil
	}

	sectionName, existsFlag := componentSectionNameFromRootFile(targetFile)
	if !existsFlag {
		return canonicalComponentRefFromPointer(fragmentText)
	}

	nameText, tailText, ok := splitComponentPointer(fragmentText, sectionName)
	if !ok || nameText == "" {
		return "", false, nil
	}

	return buildCanonicalComponentRef(sectionName, nameText, tailText), true, nil
}

func canonicalComponentRefFromPointer(fragmentText string) (string, bool, error) {
	tokenArr, ok := pointerTokenArr(fragmentText)
	if !ok || len(tokenArr) < 2 {
		return "", false, nil
	}

	if tokenArr[0] == "components" {
		if len(tokenArr) < 3 {
			return "", false, nil
		}
		sectionName := tokenArr[1]
		if _, existsFlag := componentSectionNameMap[sectionName]; !existsFlag {
			return "", false, nil
		}

		return buildCanonicalComponentRef(sectionName, tokenArr[2], pointerFromTokenArr(tokenArr[3:])), true, nil
	}

	sectionName := tokenArr[0]
	if _, existsFlag := componentSectionNameMap[sectionName]; !existsFlag {
		return "", false, nil
	}

	return buildCanonicalComponentRef(sectionName, tokenArr[1], pointerFromTokenArr(tokenArr[2:])), true, nil
}

func shouldInlineRef(refText string, rootRelativeFlag bool) (bool, error) {
	if isRemoteRef(refText) {
		return false, fmt.Errorf("remote references are not supported: %s", refText)
	}

	refPath, _ := splitRef(refText)
	if strings.TrimSpace(refPath) != "" {
		return true, nil
	}

	return !rootRelativeFlag, nil
}

func buildCanonicalComponentRef(sectionName string, componentName string, tailPointerText string) string {
	resultText := "#/components/" + escapePointerToken(sectionName) + "/" + escapePointerToken(componentName)
	if strings.TrimSpace(tailPointerText) == "" {
		return resultText
	}
	if strings.HasPrefix(tailPointerText, "/") {
		return resultText + tailPointerText
	}

	return resultText + "/" + tailPointerText
}

func splitComponentPointer(fragmentText string, sectionName string) (string, string, bool) {
	tokenArr, ok := pointerTokenArr(fragmentText)
	if !ok || len(tokenArr) == 0 {
		return "", "", false
	}

	if len(tokenArr) >= 3 && tokenArr[0] == "components" && tokenArr[1] == sectionName {
		return tokenArr[2], pointerFromTokenArr(tokenArr[3:]), true
	}

	return tokenArr[0], pointerFromTokenArr(tokenArr[1:]), true
}

func pointerTokenArr(fragmentText string) ([]string, bool) {
	if fragmentText == "" || !strings.HasPrefix(fragmentText, "/") {
		return nil, false
	}

	rawArr := strings.Split(fragmentText[1:], "/")
	resultArr := make([]string, 0, len(rawArr))
	for _, rawText := range rawArr {
		resultArr = append(resultArr, unescapePointerToken(rawText))
	}

	return resultArr, true
}

func pointerFromTokenArr(tokenArr []string) string {
	if len(tokenArr) == 0 {
		return ""
	}

	partArr := make([]string, 0, len(tokenArr))
	for _, tokenText := range tokenArr {
		partArr = append(partArr, escapePointerToken(tokenText))
	}

	return "/" + strings.Join(partArr, "/")
}

func escapePointerToken(rawText string) string {
	rawText = strings.ReplaceAll(rawText, "~", "~0")
	rawText = strings.ReplaceAll(rawText, "/", "~1")
	return rawText
}
