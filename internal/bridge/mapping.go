package bridge

import (
	"fmt"
	"go/token"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// // // // // // // // // //

var httpMethodMap = map[string]struct{}{
	"get": {}, "put": {}, "post": {}, "delete": {}, "options": {}, "head": {}, "patch": {}, "trace": {},
}

type mappingSourceObj struct {
	NameText    string
	DisplayText string
}

// //

func collectMappings(documentMap map[string]any, requireFlag bool) (*MappingObj, error) {
	resultObj := &MappingObj{
		OperationFuncMap: make(map[string]string),
		SecurityFuncMap:  make(map[string]string),
		FuncDocMap:       make(map[string]funcDocObj),
	}

	if err := collectOperationMappings(documentMap, resultObj, requireFlag); err != nil {
		return nil, err
	}
	if err := collectSecurityMappings(documentMap, resultObj, requireFlag); err != nil {
		return nil, err
	}

	return resultObj, nil
}

// ValidateMappings checks x-func mappings before the expensive generation step.
func ValidateMappings(documentMap map[string]any, requireFlag bool) error {
	mappingObj, err := collectMappings(documentMap, requireFlag)
	if err != nil {
		return err
	}

	return requireMappings(mappingObj, requireFlag)
}

func requireMappings(mappingObj *MappingObj, requireFlag bool) error {
	if requireFlag && len(mappingObj.MissingArr) > 0 {
		return fmt.Errorf("missing x-func mappings:\n%s", strings.Join(mappingObj.MissingArr, "\n"))
	}

	return nil
}

func collectOperationMappings(documentMap map[string]any, resultObj *MappingObj, requireFlag bool) error {
	sourceMap := make(map[string]mappingSourceObj)
	pathsMap, _ := documentMap["paths"].(map[string]any)
	pathArr := sortedKeys(pathsMap)
	for _, pathText := range pathArr {
		pathItemMap, _ := pathsMap[pathText].(map[string]any)
		methodArr := sortedKeys(pathItemMap)
		for _, methodText := range methodArr {
			if _, isMethodFlag := httpMethodMap[strings.ToLower(methodText)]; !isMethodFlag {
				continue
			}

			operationMap, _ := pathItemMap[methodText].(map[string]any)
			operationID, _ := operationMap["operationId"].(string)
			operationID = strings.TrimSpace(operationID)
			if operationID == "" {
				if requireFlag {
					resultObj.MissingArr = append(resultObj.MissingArr, fmt.Sprintf("- %s %s operationId is missing", strings.ToUpper(methodText), pathText))
				}
				continue
			}

			funcName, _ := operationMap["x-func"].(string)
			funcName = strings.TrimSpace(funcName)
			if funcName == "" {
				if requireFlag {
					resultObj.MissingArr = append(resultObj.MissingArr, fmt.Sprintf("- %s %s operationId %s", strings.ToUpper(methodText), pathText, operationID))
				}
				continue
			}

			normalizedName := normalizeName(operationID)
			if normalizedName == "" {
				return fmt.Errorf("operationId %q for %s %s has no alphanumeric characters", operationID, strings.ToUpper(methodText), pathText)
			}
			sourceObj := mappingSourceObj{
				NameText:    operationID,
				DisplayText: fmt.Sprintf("%s %s", strings.ToUpper(methodText), pathText),
			}
			if previousObj, existsFlag := sourceMap[normalizedName]; existsFlag {
				return fmt.Errorf("operationId normalization collision: %s operationId %q and %s operationId %q both normalize to %q", previousObj.DisplayText, previousObj.NameText, sourceObj.DisplayText, sourceObj.NameText, normalizedName)
			}

			sourceMap[normalizedName] = sourceObj
			resultObj.OperationFuncMap[normalizedName] = funcName

			if _, existsFlag := resultObj.FuncDocMap[funcName]; !existsFlag {
				summaryText, _ := operationMap["summary"].(string)
				descText, _ := operationMap["description"].(string)
				resultObj.FuncDocMap[funcName] = funcDocObj{
					Method:  strings.ToUpper(methodText),
					Path:    pathText,
					Summary: summaryText,
					Desc:    descText,
				}
			}
		}
	}

	return nil
}

func collectSecurityMappings(documentMap map[string]any, resultObj *MappingObj, requireFlag bool) error {
	sourceMap := make(map[string]mappingSourceObj)
	componentsMap, _ := documentMap["components"].(map[string]any)
	securitySchemesMap, _ := componentsMap["securitySchemes"].(map[string]any)

	schemeArr := sortedKeys(securitySchemesMap)
	for _, schemeName := range schemeArr {
		schemeMap, _ := securitySchemesMap[schemeName].(map[string]any)
		funcName, _ := schemeMap["x-func"].(string)
		funcName = strings.TrimSpace(funcName)
		if funcName == "" {
			if requireFlag {
				resultObj.MissingArr = append(resultObj.MissingArr, fmt.Sprintf("- security scheme %s", schemeName))
			}
			continue
		}

		if _, existsFlag := resultObj.FuncDocMap[funcName]; !existsFlag {
			descText, _ := schemeMap["description"].(string)
			resultObj.FuncDocMap[funcName] = funcDocObj{
				Security: true,
				Scheme:   schemeName,
				Desc:     descText,
			}
		}

		for _, sourceObj := range []mappingSourceObj{
			{NameText: schemeName, DisplayText: fmt.Sprintf("security scheme %s", schemeName)},
			{NameText: "Handle" + schemeName, DisplayText: fmt.Sprintf("security handler %s", schemeName)},
		} {
			normalizedName := normalizeName(sourceObj.NameText)
			if normalizedName == "" {
				return fmt.Errorf("%s has no alphanumeric characters", sourceObj.DisplayText)
			}
			if previousObj, existsFlag := sourceMap[normalizedName]; existsFlag {
				return fmt.Errorf("security scheme normalization collision: %s and %s both normalize to %q", previousObj.DisplayText, sourceObj.DisplayText, normalizedName)
			}

			sourceMap[normalizedName] = sourceObj
			resultObj.SecurityFuncMap[normalizedName] = funcName
		}
	}

	return nil
}

func normalizeName(rawText string) string {
	builderObj := strings.Builder{}
	for _, symbol := range rawText {
		if unicode.IsLetter(symbol) || unicode.IsDigit(symbol) {
			builderObj.WriteRune(unicode.ToLower(symbol))
		}
	}

	return builderObj.String()
}

func sortedKeys(valueMap map[string]any) []string {
	keyArr := make([]string, 0, len(valueMap))
	for key := range valueMap {
		keyArr = append(keyArr, key)
	}
	sort.Strings(keyArr)
	return keyArr
}

func validateFuncName(nameText string) bool {
	if !token.IsIdentifier(nameText) || token.Lookup(nameText).IsKeyword() {
		return false
	}

	firstRune, _ := utf8.DecodeRuneInString(nameText)
	return unicode.IsUpper(firstRune)
}

// //

// buildFuncDocLineArr builds the doc-comment lines: a header with the route or
// security scheme, followed by the summary/description carried over from the spec.
func buildFuncDocLineArr(funcName string, docObj funcDocObj) []string {
	headText := ""
	if docObj.Security {
		schemeText := strings.TrimSpace(docObj.Scheme)
		headText = fmt.Sprintf("%s authorizes the %q security scheme.", funcName, schemeText)
	} else {
		headText = fmt.Sprintf("%s handles %s %s.", funcName, docObj.Method, docObj.Path)
	}

	bodyArr := make([]string, 0)
	if summaryText := strings.TrimSpace(docObj.Summary); summaryText != "" {
		bodyArr = append(bodyArr, splitCommentLines(summaryText)...)
	}
	if descText := strings.TrimSpace(docObj.Desc); descText != "" {
		if len(bodyArr) > 0 {
			bodyArr = append(bodyArr, "")
		}
		bodyArr = append(bodyArr, splitCommentLines(descText)...)
	}

	lineArr := []string{headText}
	if len(bodyArr) > 0 {
		lineArr = append(lineArr, "")
		lineArr = append(lineArr, bodyArr...)
	}

	return lineArr
}

func splitCommentLines(rawText string) []string {
	rawText = strings.ReplaceAll(rawText, "\r\n", "\n")
	rawArr := strings.Split(rawText, "\n")
	resultArr := make([]string, 0, len(rawArr))
	for _, lineText := range rawArr {
		resultArr = append(resultArr, strings.TrimRight(lineText, " \t"))
	}

	return resultArr
}
