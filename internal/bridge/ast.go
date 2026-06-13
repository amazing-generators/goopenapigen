package bridge

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/amazing-generators/goopenapigen/internal/write"
)

// // // // // // // // // //

// serviceHandlerMethodMap holds ogen Handler interface methods that are not API
// operations. NewError appears when ogen convenient errors are enabled and must be
// implemented separately rather than as an operation, so the bridge excludes it.
var serviceHandlerMethodMap = map[string]struct{}{
	"NewError": {},
}

// //

func dropServiceMethods(methodArr []MethodObj) []MethodObj {
	resultArr := make([]MethodObj, 0, len(methodArr))
	for _, methodObj := range methodArr {
		if _, skipFlag := serviceHandlerMethodMap[methodObj.Name]; skipFlag {
			continue
		}

		resultArr = append(resultArr, methodObj)
	}

	return resultArr
}

// //

func parseOgenInterfaces(ogenDir string) (*ParseResultObj, error) {
	filePathArr, err := listGoFiles(ogenDir)
	if err != nil {
		return nil, err
	}

	resultObj := &ParseResultObj{
		HandlerInterfaceName:  "Handler",
		SecurityInterfaceName: "SecurityHandler",
		ServerOptionName:      "ServerOption",
		ImportMap:             make(map[string]string),
		StructFieldMap:        make(map[string]map[string]StructFieldObj),
	}
	for _, filePath := range filePathArr {
		generatedFlag, err := write.HasGeneratedHeader(filePath)
		if err != nil {
			return nil, err
		}
		if generatedFlag {
			continue
		}

		parsedFile, err := parser.ParseFile(token.NewFileSet(), filePath, nil, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("parse generated Go file %s: %w", filePath, err)
		}

		mergeImports(resultObj.ImportMap, parsedFile)
		mergeStructFields(resultObj.StructFieldMap, parsedFile)
		handlerName, handlerArr, securityName, securityArr, serverOptionName, err := parseInterfacesFromFile(parsedFile)
		if err != nil {
			return nil, fmt.Errorf("parse generated interfaces in %s: %w", filePath, err)
		}
		handlerArr = dropServiceMethods(handlerArr)
		if len(handlerArr) > 0 {
			resultObj.HandlerInterfaceName = handlerName
			resultObj.HandlerMethodArr = handlerArr
		}
		if len(securityArr) > 0 {
			resultObj.SecurityInterfaceName = securityName
			resultObj.SecurityMethodArr = securityArr
		}
		if serverOptionName != "" {
			resultObj.ServerOptionName = serverOptionName
		}
	}

	if len(resultObj.HandlerMethodArr) == 0 {
		return nil, fmt.Errorf("ogen Handler interface not found in %s", ogenDir)
	}

	return resultObj, nil
}

func listGoFiles(ogenDir string) ([]string, error) {
	entryArr, err := os.ReadDir(ogenDir)
	if err != nil {
		return nil, fmt.Errorf("read ogen output directory: %w", err)
	}

	resultArr := make([]string, 0, len(entryArr))
	for _, entryObj := range entryArr {
		if entryObj.IsDir() {
			continue
		}
		nameText := entryObj.Name()
		if !strings.HasSuffix(nameText, ".go") || strings.HasSuffix(nameText, "_test.go") {
			continue
		}

		resultArr = append(resultArr, filepath.Join(ogenDir, nameText))
	}

	sort.Strings(resultArr)
	return resultArr, nil
}

func mergeImports(targetMap map[string]string, parsedFile *ast.File) {
	for _, importSpec := range parsedFile.Imports {
		importPath := strings.Trim(importSpec.Path.Value, `"`)
		aliasText := filepath.Base(importPath)
		if importSpec.Name != nil && importSpec.Name.Name != "" && importSpec.Name.Name != "." && importSpec.Name.Name != "_" {
			aliasText = importSpec.Name.Name
		}

		targetMap[aliasText] = importPath
	}
}

func mergeStructFields(targetMap map[string]map[string]StructFieldObj, parsedFile *ast.File) {
	for _, declarationObj := range parsedFile.Decls {
		genDeclObj, ok := declarationObj.(*ast.GenDecl)
		if !ok || genDeclObj.Tok != token.TYPE {
			continue
		}

		for _, specObj := range genDeclObj.Specs {
			typeSpecObj, ok := specObj.(*ast.TypeSpec)
			if !ok {
				continue
			}

			structObj, ok := typeSpecObj.Type.(*ast.StructType)
			if !ok {
				continue
			}

			fieldMap := structFieldsByJSONName(structObj)
			if len(fieldMap) > 0 {
				targetMap[typeSpecObj.Name.Name] = fieldMap
			}
		}
	}
}

func structFieldsByJSONName(structObj *ast.StructType) map[string]StructFieldObj {
	resultMap := make(map[string]StructFieldObj)
	for _, fieldObj := range structObj.Fields.List {
		if len(fieldObj.Names) == 0 {
			continue
		}

		typeText, err := exprToString(fieldObj.Type)
		if err != nil {
			continue
		}

		for _, nameObj := range fieldObj.Names {
			jsonName := jsonFieldName(fieldObj, nameObj.Name)
			if jsonName == "" || jsonName == "-" {
				continue
			}

			resultMap[jsonName] = StructFieldObj{
				Name: nameObj.Name,
				Type: typeText,
			}
		}
	}

	return resultMap
}

func jsonFieldName(fieldObj *ast.Field, fallbackName string) string {
	if fieldObj.Tag == nil {
		return fallbackName
	}

	tagText := strings.Trim(fieldObj.Tag.Value, "`")
	jsonText := reflect.StructTag(tagText).Get("json")
	if jsonText == "" {
		return fallbackName
	}

	nameText, _, _ := strings.Cut(jsonText, ",")
	return nameText
}

func parseInterfacesFromFile(parsedFile *ast.File) (string, []MethodObj, string, []MethodObj, string, error) {
	handlerName := ""
	var handlerArr []MethodObj
	securityName := ""
	var securityArr []MethodObj
	serverOptionName := ""

	for _, declarationObj := range parsedFile.Decls {
		genDeclObj, ok := declarationObj.(*ast.GenDecl)
		if !ok || genDeclObj.Tok != token.TYPE {
			continue
		}

		for _, specObj := range genDeclObj.Specs {
			typeSpecObj, ok := specObj.(*ast.TypeSpec)
			if !ok {
				continue
			}

			interfaceObj, ok := typeSpecObj.Type.(*ast.InterfaceType)
			if !ok {
				continue
			}
			typeName := typeSpecObj.Name.Name
			if typeName == "ServerOption" || typeName == "ServerOptionInterface" {
				serverOptionName = typeName
			}

			methodArr, err := buildInterfaceMethods(interfaceObj)
			if err != nil {
				return "", nil, "", nil, "", err
			}

			switch typeName {
			case "Handler", "HandlerInterface":
				handlerName = typeName
				handlerArr = methodArr
			case "SecurityHandler", "SecurityHandlerInterface":
				securityName = typeName
				securityArr = methodArr
			}
		}
	}

	return handlerName, handlerArr, securityName, securityArr, serverOptionName, nil
}

func buildInterfaceMethods(interfaceObj *ast.InterfaceType) ([]MethodObj, error) {
	resultArr := make([]MethodObj, 0, len(interfaceObj.Methods.List))
	for _, fieldObj := range interfaceObj.Methods.List {
		funcObj, ok := fieldObj.Type.(*ast.FuncType)
		if !ok || len(fieldObj.Names) == 0 {
			continue
		}

		for _, nameObj := range fieldObj.Names {
			paramArr, err := parseFieldList(funcObj.Params, true)
			if err != nil {
				return nil, fmt.Errorf("parse params for %s: %w", nameObj.Name, err)
			}

			resultFieldArr, err := parseFieldList(funcObj.Results, false)
			if err != nil {
				return nil, fmt.Errorf("parse results for %s: %w", nameObj.Name, err)
			}

			resultArr = append(resultArr, MethodObj{
				Name:      nameObj.Name,
				ParamArr:  paramArr,
				ResultArr: resultFieldArr,
			})
		}
	}

	return resultArr, nil
}

func parseFieldList(fieldListObj *ast.FieldList, forceNameFlag bool) ([]FieldObj, error) {
	if fieldListObj == nil || len(fieldListObj.List) == 0 {
		return nil, nil
	}

	resultArr := make([]FieldObj, 0)
	generatedIndex := 0
	for _, fieldObj := range fieldListObj.List {
		typeText, err := exprToString(fieldObj.Type)
		if err != nil {
			return nil, err
		}

		if len(fieldObj.Names) == 0 {
			nameText := ""
			namedFlag := false
			if forceNameFlag {
				nameText = fmt.Sprintf("arg%d", generatedIndex)
				generatedIndex++
				namedFlag = true
			}

			resultArr = append(resultArr, FieldObj{Name: nameText, Type: typeText, Named: namedFlag})
			continue
		}

		for _, nameObj := range fieldObj.Names {
			nameText := nameObj.Name
			if nameText == "" || nameText == "_" {
				nameText = fmt.Sprintf("arg%d", generatedIndex)
				generatedIndex++
			}

			resultArr = append(resultArr, FieldObj{Name: nameText, Type: typeText, Named: true})
		}
	}

	return resultArr, nil
}

func exprToString(exprObj ast.Expr) (string, error) {
	bufferObj := bytes.NewBuffer(nil)
	if err := printer.Fprint(bufferObj, token.NewFileSet(), exprObj); err != nil {
		return "", err
	}

	return bufferObj.String(), nil
}
