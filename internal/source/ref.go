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

type refTargetObj struct {
	idText           string
	displayText      string
	value            any
	filePath         string
	rootRelativeFlag bool
}

type refWalkerObj struct {
	contextObj context.Context
	loaderObj  *loaderObj
	maxDepth   int

	visitedMap map[string]struct{}
	stackArr   []string
	stackMap   map[string]int
}

// //

func validateGraphRefs(contextObj context.Context, loaderObj *loaderObj, maxDepth int) error {
	walkerObj := &refWalkerObj{
		contextObj: contextObj,
		loaderObj:  loaderObj,
		maxDepth:   maxDepth,
		visitedMap: make(map[string]struct{}),
		stackMap:   make(map[string]int),
	}

	filePathArr := make([]string, 0, len(loaderObj.rootRelativeFileMap))
	for filePath := range loaderObj.rootRelativeFileMap {
		filePathArr = append(filePathArr, filePath)
	}
	sort.Strings(filePathArr)

	for _, filePath := range filePathArr {
		if err := contextObj.Err(); err != nil {
			return err
		}

		documentValue, err := loaderObj.loadDocument(filePath)
		if err != nil {
			return err
		}

		if err = walkerObj.walkValue(documentValue, filePath, true, 0); err != nil {
			return err
		}
	}

	return nil
}

func (obj *refWalkerObj) walkValue(value any, currentFile string, rootRelativeFlag bool, depth int) error {
	if err := obj.contextObj.Err(); err != nil {
		return err
	}

	switch castedValue := value.(type) {
	case map[string]any:
		if rawRefValue, existsFlag := castedValue["$ref"]; existsFlag {
			refText, ok := rawRefValue.(string)
			if !ok {
				return fmt.Errorf("$ref value must be string in %s", currentFile)
			}

			if err := obj.followRef(currentFile, rootRelativeFlag, refText, depth); err != nil {
				return err
			}
		}

		keyArr := make([]string, 0, len(castedValue))
		for key := range castedValue {
			keyArr = append(keyArr, key)
		}
		sort.Strings(keyArr)

		for _, key := range keyArr {
			if key == "$ref" {
				continue
			}
			if err := obj.walkValue(castedValue[key], currentFile, rootRelativeFlag, depth); err != nil {
				return err
			}
		}
	case []any:
		for _, innerValue := range castedValue {
			if err := obj.walkValue(innerValue, currentFile, rootRelativeFlag, depth); err != nil {
				return err
			}
		}
	}

	return nil
}

func (obj *refWalkerObj) followRef(currentFile string, rootRelativeFlag bool, refText string, depth int) error {
	if depth+1 > obj.maxDepth {
		return fmt.Errorf("maximum $ref depth exceeded: %d", obj.maxDepth)
	}

	targetObj, err := resolveRefTarget(obj.loaderObj, currentFile, rootRelativeFlag, refText)
	if err != nil {
		return err
	}

	if startIndex, existsFlag := obj.stackMap[targetObj.idText]; existsFlag {
		chainArr := append([]string{}, obj.stackArr[startIndex:]...)
		chainArr = append(chainArr, targetObj.displayText)
		return fmt.Errorf("recursive $ref detected:\n%s", strings.Join(chainArr, " -> "))
	}

	if _, visitedFlag := obj.visitedMap[targetObj.idText]; visitedFlag {
		return nil
	}

	obj.stackMap[targetObj.idText] = len(obj.stackArr)
	obj.stackArr = append(obj.stackArr, targetObj.displayText)

	err = obj.walkValue(targetObj.value, targetObj.filePath, targetObj.rootRelativeFlag, depth+1)

	delete(obj.stackMap, targetObj.idText)
	obj.stackArr = obj.stackArr[:len(obj.stackArr)-1]

	if err != nil {
		return err
	}

	obj.visitedMap[targetObj.idText] = struct{}{}
	return nil
}

func resolveRefTarget(loaderObj *loaderObj, currentFile string, rootRelativeFlag bool, refText string) (*refTargetObj, error) {
	if isRemoteRef(refText) {
		return nil, fmt.Errorf("remote references are not supported: %s", refText)
	}

	refPath, fragmentText := splitRef(refText)
	if strings.TrimSpace(refPath) == "" {
		if rootRelativeFlag {
			targetValue, err := resolveRootPointer(loaderObj.assembledDocument, fragmentText)
			if err != nil {
				return nil, fmt.Errorf("resolve pointer #%s in assembled root: %w", fragmentText, err)
			}

			idText := "assembled#" + fragmentText
			return &refTargetObj{
				idText:           idText,
				displayText:      displayTarget(loaderObj, loaderObj.rootFile, fragmentText),
				value:            targetValue,
				filePath:         loaderObj.rootFile,
				rootRelativeFlag: true,
			}, nil
		}

		documentValue, err := loaderObj.loadDocument(currentFile)
		if err != nil {
			return nil, err
		}

		targetValue, err := resolvePointer(documentValue, fragmentText)
		if err != nil {
			return nil, fmt.Errorf("resolve pointer #%s in %s: %w", fragmentText, currentFile, err)
		}

		return &refTargetObj{
			idText:           currentFile + "#" + fragmentText,
			displayText:      displayTarget(loaderObj, currentFile, fragmentText),
			value:            targetValue,
			filePath:         currentFile,
			rootRelativeFlag: false,
		}, nil
	}

	targetFile, err := resolveRefPath(loaderObj, currentFile, refPath)
	if err != nil {
		return nil, err
	}

	documentValue, err := loaderObj.loadDocument(targetFile)
	if err != nil {
		return nil, err
	}

	targetValue, err := resolvePointer(documentValue, fragmentText)
	if err != nil {
		return nil, fmt.Errorf("resolve pointer #%s in %s: %w", fragmentText, targetFile, err)
	}

	return &refTargetObj{
		idText:           targetFile + "#" + fragmentText,
		displayText:      displayTarget(loaderObj, targetFile, fragmentText),
		value:            targetValue,
		filePath:         targetFile,
		rootRelativeFlag: loaderObj.isRootRelative(targetFile),
	}, nil
}

func resolveRefPath(loaderObj *loaderObj, currentFile string, refPath string) (string, error) {
	refPath = strings.TrimSpace(refPath)
	if refPath == "" {
		return currentFile, nil
	}

	candidateArr, err := refPathCandidateArr(loaderObj, currentFile, refPath)
	if err != nil {
		return "", err
	}
	for _, candidatePath := range candidateArr {
		absPath, foundFlag, err := findSourceFile(loaderObj.sourceRoot, candidatePath)
		if err != nil {
			return "", err
		}
		if foundFlag {
			return absPath, nil
		}
	}

	return "", fmt.Errorf("ref target not found: %s", refPath)
}

func refPathCandidateArr(loaderObj *loaderObj, currentFile string, refPath string) ([]string, error) {
	slashPath := filepath.ToSlash(refPath)
	if filepath.IsAbs(refPath) {
		return []string{refPath}, nil
	}
	if isExplicitRelativeRefPath(slashPath) {
		return []string{filepath.Join(filepath.Dir(currentFile), filepath.FromSlash(slashPath))}, nil
	}

	candidateArr, err := rootRefPathCandidateArr(loaderObj.sourceRoot, slashPath)
	if err != nil {
		return nil, err
	}

	relativePath := filepath.Join(filepath.Dir(currentFile), filepath.FromSlash(slashPath))
	candidateArr = append(candidateArr, relativePath)
	return uniquePathArr(candidateArr), nil
}

func rootRefPathCandidateArr(sourceRoot string, slashPath string) ([]string, error) {
	switch {
	case slashPath == "tags":
		return []string{filepath.Join(sourceRoot, "tags")}, nil
	case slashPath == "parameters":
		return []string{filepath.Join(sourceRoot, "components", "parameters")}, nil
	case slashPath == "security_schemes":
		return []string{filepath.Join(sourceRoot, "components", "security_schemes")}, nil
	case strings.HasPrefix(slashPath, "schemas/"):
		tailText := strings.TrimPrefix(slashPath, "schemas/")
		if strings.TrimSpace(tailText) == "" {
			return nil, fmt.Errorf("reference %q has empty schema name", slashPath)
		}
		return []string{filepath.Join(sourceRoot, "components", "schemas", filepath.FromSlash(tailText))}, nil
	case strings.HasPrefix(slashPath, "responses/"):
		tailText := strings.TrimPrefix(slashPath, "responses/")
		if strings.TrimSpace(tailText) == "" {
			return nil, fmt.Errorf("reference %q has empty response name", slashPath)
		}
		return []string{filepath.Join(sourceRoot, "components", "responses", filepath.FromSlash(tailText))}, nil
	case strings.HasPrefix(slashPath, "parameters/"):
		tailText := strings.TrimPrefix(slashPath, "parameters/")
		if strings.TrimSpace(tailText) == "" {
			return nil, fmt.Errorf("reference %q has empty parameter name", slashPath)
		}
		return []string{filepath.Join(sourceRoot, "components", "parameters", filepath.FromSlash(tailText))}, nil
	case strings.HasPrefix(slashPath, "requestBodies/"):
		tailText := strings.TrimPrefix(slashPath, "requestBodies/")
		if strings.TrimSpace(tailText) == "" {
			return nil, fmt.Errorf("reference %q has empty request body name", slashPath)
		}
		return []string{filepath.Join(sourceRoot, "components", "requestBodies", filepath.FromSlash(tailText))}, nil
	case strings.HasPrefix(slashPath, "headers/"):
		tailText := strings.TrimPrefix(slashPath, "headers/")
		if strings.TrimSpace(tailText) == "" {
			return nil, fmt.Errorf("reference %q has empty header name", slashPath)
		}
		return []string{filepath.Join(sourceRoot, "components", "headers", filepath.FromSlash(tailText))}, nil
	case strings.HasPrefix(slashPath, "paths/"):
		tailText := strings.TrimPrefix(slashPath, "paths/")
		if strings.TrimSpace(tailText) == "" {
			return nil, fmt.Errorf("reference %q has empty path name", slashPath)
		}
		return []string{filepath.Join(sourceRoot, "paths", filepath.FromSlash(tailText))}, nil
	case strings.HasPrefix(slashPath, "components/"):
		tailText := strings.TrimPrefix(slashPath, "components/")
		if strings.TrimSpace(tailText) == "" {
			return nil, fmt.Errorf("reference %q has empty components path", slashPath)
		}
		return []string{filepath.Join(sourceRoot, "components", filepath.FromSlash(tailText))}, nil
	case strings.Contains(slashPath, "/"):
		return []string{filepath.Join(sourceRoot, filepath.FromSlash(slashPath))}, nil
	default:
		return nil, nil
	}
}

func isExplicitRelativeRefPath(slashPath string) bool {
	return slashPath == "." || slashPath == ".." || strings.HasPrefix(slashPath, "./") || strings.HasPrefix(slashPath, "../")
}

func uniquePathArr(pathArr []string) []string {
	resultArr := make([]string, 0, len(pathArr))
	seenMap := make(map[string]struct{}, len(pathArr))
	for _, pathText := range pathArr {
		cleanPath, err := cleanAbs(pathText)
		if err != nil {
			cleanPath = filepath.Clean(pathText)
		}
		if _, existsFlag := seenMap[cleanPath]; existsFlag {
			continue
		}

		seenMap[cleanPath] = struct{}{}
		resultArr = append(resultArr, pathText)
	}

	return resultArr
}

func findSourceFile(sourceRoot string, candidatePath string) (string, bool, error) {
	if filepath.Ext(candidatePath) != "" {
		return statSourceFile(sourceRoot, candidatePath)
	}

	// With no explicit extension, try a fixed set and reject ambiguity when a single
	// ref matches several files with different extensions.
	matchArr := make([]string, 0, 3)
	for _, extText := range []string{".yaml", ".yml", ".json"} {
		absPath, foundFlag, err := statSourceFile(sourceRoot, candidatePath+extText)
		if err != nil {
			return "", false, err
		}
		if foundFlag {
			matchArr = append(matchArr, absPath)
		}
	}

	switch len(matchArr) {
	case 0:
		return "", false, nil
	case 1:
		return matchArr[0], true, nil
	default:
		return "", false, fmt.Errorf("ambiguous ref target %q matches multiple files: %s", candidatePath, strings.Join(matchArr, ", "))
	}
}

func statSourceFile(sourceRoot string, pathText string) (string, bool, error) {
	absPath, err := cleanAbs(pathText)
	if err != nil {
		return "", false, err
	}
	if _, err = relativeSlash(sourceRoot, absPath); err != nil {
		return "", false, err
	}

	infoObj, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("stat ref target %s: %w", absPath, err)
	}
	if infoObj.IsDir() {
		return "", false, fmt.Errorf("ref target is a directory: %s", absPath)
	}
	if err = rejectSymlinkPath(sourceRoot, absPath); err != nil {
		return "", false, err
	}

	return absPath, true, nil
}

func splitRef(refText string) (string, string) {
	refPath := refText
	fragmentText := ""

	if index := strings.Index(refText, "#"); index >= 0 {
		refPath = refText[:index]
		fragmentText = refText[index+1:]
	}

	return refPath, fragmentText
}

func isRemoteRef(refText string) bool {
	return strings.Contains(refText, "://") || strings.HasPrefix(refText, "//")
}

func resolvePointer(documentValue any, pointerText string) (any, error) {
	if pointerText == "" {
		return documentValue, nil
	}
	if !strings.HasPrefix(pointerText, "/") {
		return nil, fmt.Errorf("json pointer must start with /")
	}

	currentValue := documentValue
	partArr := strings.Split(pointerText[1:], "/")
	for _, rawPart := range partArr {
		partText := unescapePointerToken(rawPart)
		switch castedValue := currentValue.(type) {
		case map[string]any:
			nextValue, existsFlag := castedValue[partText]
			if !existsFlag {
				return nil, fmt.Errorf("object key not found: %s", partText)
			}
			currentValue = nextValue
		case []any:
			indexValue, ok := parsePointerIndex(partText, len(castedValue))
			if !ok {
				return nil, fmt.Errorf("array index not found: %s", partText)
			}
			currentValue = castedValue[indexValue]
		default:
			return nil, fmt.Errorf("cannot descend into %T at %s", currentValue, partText)
		}
	}

	return currentValue, nil
}

func resolveRootPointer(documentValue any, pointerText string) (any, error) {
	return resolvePointer(documentValue, canonicalRootPointer(documentValue, pointerText))
}

func canonicalRootPointer(documentValue any, pointerText string) string {
	tokenArr, ok := pointerTokenArr(pointerText)
	if !ok || len(tokenArr) < 3 || tokenArr[0] != "components" {
		return pointerText
	}

	componentsMap, ok := documentValue.(map[string]any)["components"].(map[string]any)
	if !ok {
		return pointerText
	}

	sectionName := tokenArr[1]
	sectionMap, ok := componentsMap[sectionName].(map[string]any)
	if !ok {
		return pointerText
	}

	componentName := tokenArr[2]
	if _, existsFlag := sectionMap[componentName]; existsFlag {
		return pointerText
	}

	nextName := appendComponentNameSuffix(componentName, sectionName)
	if nextName == componentName {
		return pointerText
	}
	if _, existsFlag := sectionMap[nextName]; !existsFlag {
		return pointerText
	}

	tokenArr[2] = nextName
	return pointerFromTokenArr(tokenArr)
}

func parsePointerIndex(rawText string, maxValue int) (int, bool) {
	if rawText == "" {
		return 0, false
	}

	resultValue := 0
	for _, symbol := range rawText {
		if symbol < '0' || symbol > '9' {
			return 0, false
		}
		resultValue = resultValue*10 + int(symbol-'0')
		if resultValue >= maxValue {
			return 0, false
		}
	}

	return resultValue, true
}

func unescapePointerToken(rawText string) string {
	rawText = strings.ReplaceAll(rawText, "~1", "/")
	rawText = strings.ReplaceAll(rawText, "~0", "~")
	return rawText
}

func displayTarget(loaderObj *loaderObj, filePath string, fragmentText string) string {
	relPath, err := relativeSlash(loaderObj.sourceRoot, filePath)
	if err != nil {
		relPath = filePath
	}
	if fragmentText == "" {
		return relPath
	}

	return relPath + "#" + fragmentText
}
