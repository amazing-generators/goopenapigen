package bridge

import (
	"bytes"
	"compress/flate"
	_ "embed"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"path"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/amazing-generators/goopenapigen/internal/write"
)

// // // // // // // // // //

//go:embed templates/router.go.tmpl
var routerTemplateText string

//go:embed templates/openapi_json.go.tmpl
var openAPIJSONTemplateText string

//go:embed templates/http_defaults.go.tmpl
var httpDefaultsTemplateText string

var routerTemplateObj = template.Must(template.New("router.go.tmpl").Parse(routerTemplateText))

var openAPIJSONTemplateObj = template.Must(template.New("openapi_json.go.tmpl").Parse(openAPIJSONTemplateText))

var httpDefaultsTemplateObj = template.Must(template.New("http_defaults.go.tmpl").Parse(httpDefaultsTemplateText))

type routerTemplateDataObj struct {
	Header            string
	PackageName       string
	ImportArr         []string
	HandlerInterface  string
	SecurityInterface string
	ServerOption      string
	HasFuncInterface  bool
	HasSecurity       bool
	FuncMethodArr     []routerMethodTemplateDataObj
	HandlerMethodArr  []routerMethodTemplateDataObj
	SecurityMethodArr []routerMethodTemplateDataObj
}

type openAPIJSONTemplateDataObj struct {
	Header                string
	PackageName           string
	CompressedJSONLiteral string
}

type httpDefaultsTemplateDataObj struct {
	Header           string
	PackageName      string
	ServerOption     string
	HasFuncInterface bool
	HasOpenAPIJSON   bool
	DefaultError     defaultErrorTemplateDataObj
}

type routerMethodTemplateDataObj struct {
	ReceiverType    string
	ReceiverName    string
	Name            string
	ParamSignature  string
	ResultSignature string
	Body            string
	Declaration     string
}

type funcSignatureObj struct {
	SourceName      string
	ParamSignature  string
	ResultSignature string
}

type defaultErrorTemplateDataObj struct {
	DeclareFallback bool
	DeclareAlias    bool
	AliasTargetName string
	TypeName        string
	StatusFieldName string
	StatusValue     string
	MessageField    string
}

// //

func Generate(configObj ConfigObj) ([]byte, error) {
	if strings.TrimSpace(configObj.PackageName) == "" {
		return nil, fmt.Errorf("router package name is empty")
	}
	if strings.TrimSpace(configObj.OgenDir) == "" {
		return nil, fmt.Errorf("ogen output directory is empty")
	}

	parseObj, err := parseOgenInterfaces(configObj.OgenDir)
	if err != nil {
		return nil, err
	}

	mappingObj, err := collectMappings(configObj.Document, configObj.RequireXFunc)
	if err != nil {
		return nil, err
	}
	if err = requireMappings(mappingObj, configObj.RequireXFunc); err != nil {
		return nil, err
	}

	handlerMethodArr, handlerUsesCallFlag, handlerNeedsErrorFlag, err := buildRenderMethods(parseObj.HandlerMethodArr, mappingObj.OperationFuncMap)
	if err != nil {
		return nil, err
	}
	securityMethodArr, securityUsesCallFlag, securityNeedsErrorFlag, err := buildRenderMethods(parseObj.SecurityMethodArr, mappingObj.SecurityFuncMap)
	if err != nil {
		return nil, err
	}

	funcMethodArr, err := buildFuncMethodArr(handlerMethodArr, securityMethodArr, mappingObj.FuncDocMap, configObj.Comments)
	if err != nil {
		return nil, err
	}

	sourceText, err := renderSource(configObj, parseObj, funcMethodArr, handlerMethodArr, securityMethodArr, handlerUsesCallFlag || securityUsesCallFlag, handlerNeedsErrorFlag || securityNeedsErrorFlag)
	if err != nil {
		return nil, err
	}

	formattedArr, err := format.Source([]byte(sourceText))
	if err != nil {
		return nil, fmt.Errorf("format router bridge: %w\n%s", err, sourceText)
	}

	return formattedArr, nil
}

func GenerateOpenAPIJSON(configObj ConfigObj) ([]byte, error) {
	if strings.TrimSpace(configObj.PackageName) == "" {
		return nil, fmt.Errorf("OpenAPI JSON package name is empty")
	}
	if len(configObj.OpenAPIJSON) == 0 {
		return nil, fmt.Errorf("OpenAPI JSON content is empty")
	}

	sourceText, err := renderOpenAPIJSONSource(configObj)
	if err != nil {
		return nil, err
	}

	formattedArr, err := format.Source([]byte(sourceText))
	if err != nil {
		return nil, fmt.Errorf("format OpenAPI JSON Go file: %w\n%s", err, sourceText)
	}

	return formattedArr, nil
}

func GenerateHTTPDefaults(configObj ConfigObj) ([]byte, error) {
	if strings.TrimSpace(configObj.PackageName) == "" {
		return nil, fmt.Errorf("HTTP defaults package name is empty")
	}
	if strings.TrimSpace(configObj.OgenDir) == "" {
		return nil, fmt.Errorf("ogen output directory is empty")
	}

	parseObj, err := parseOgenInterfaces(configObj.OgenDir)
	if err != nil {
		return nil, err
	}

	mappingObj, err := collectMappings(configObj.Document, configObj.RequireXFunc)
	if err != nil {
		return nil, err
	}
	if err = requireMappings(mappingObj, configObj.RequireXFunc); err != nil {
		return nil, err
	}

	handlerMethodArr, handlerUsesCallFlag, _, err := buildRenderMethods(parseObj.HandlerMethodArr, mappingObj.OperationFuncMap)
	if err != nil {
		return nil, err
	}
	securityMethodArr, securityUsesCallFlag, _, err := buildRenderMethods(parseObj.SecurityMethodArr, mappingObj.SecurityFuncMap)
	if err != nil {
		return nil, err
	}
	if _, err = buildFuncMethodArr(handlerMethodArr, securityMethodArr, mappingObj.FuncDocMap, false); err != nil {
		return nil, err
	}

	configObj.ServerOptionName = parseObj.ServerOptionName
	defaultErrorObj, err := buildDefaultErrorTemplateData(configObj.Document, parseObj)
	if err != nil {
		return nil, err
	}

	sourceText, err := renderHTTPDefaultsSource(configObj, handlerUsesCallFlag || securityUsesCallFlag, defaultErrorObj)
	if err != nil {
		return nil, err
	}

	formattedArr, err := format.Source([]byte(sourceText))
	if err != nil {
		return nil, fmt.Errorf("format HTTP defaults Go file: %w\n%s", err, sourceText)
	}

	return formattedArr, nil
}

func buildRenderMethods(methodArr []MethodObj, funcMap map[string]string) ([]RenderMethodObj, bool, bool, error) {
	resultArr := make([]RenderMethodObj, 0, len(methodArr))
	usesCallFlag := false
	needsErrorFlag := false

	for _, methodObj := range methodArr {
		funcName := strings.TrimSpace(funcMap[normalizeName(methodObj.Name)])
		renderObj := RenderMethodObj{MethodObj: methodObj, FuncName: funcName}
		if funcName != "" {
			if !validateFuncName(funcName) {
				return nil, false, false, fmt.Errorf("invalid x-func for %s: %s", methodObj.Name, funcName)
			}

			renderObj.UseCall = true
			usesCallFlag = true
		} else if hasErrorResult(methodObj.ResultArr) {
			renderObj.NeedError = true
			needsErrorFlag = true
		}

		resultArr = append(resultArr, renderObj)
	}

	return resultArr, usesCallFlag, needsErrorFlag, nil
}

func buildFuncMethodArr(handlerMethodArr []RenderMethodObj, securityMethodArr []RenderMethodObj, docMap map[string]funcDocObj, commentsFlag bool) ([]routerMethodTemplateDataObj, error) {
	signatureMap := make(map[string]funcSignatureObj)
	methodMap := make(map[string]routerMethodTemplateDataObj)
	for _, renderObj := range append(handlerMethodArr, securityMethodArr...) {
		if !renderObj.UseCall {
			continue
		}

		methodObj := renderObj.MethodObj
		paramSignatureText := buildParamSignature(methodObj.ParamArr)
		resultSignatureText := buildResultSignature(methodObj.ResultArr)
		currentObj := funcSignatureObj{
			SourceName:      methodObj.Name,
			ParamSignature:  paramSignatureText,
			ResultSignature: resultSignatureText,
		}

		existingObj, existsFlag := signatureMap[renderObj.FuncName]
		if existsFlag {
			if existingObj.ParamSignature != currentObj.ParamSignature || existingObj.ResultSignature != currentObj.ResultSignature {
				return nil, fmt.Errorf("x-func collision for %s: %s and %s have different signatures", renderObj.FuncName, existingObj.SourceName, currentObj.SourceName)
			}
			continue
		}

		signatureMap[renderObj.FuncName] = currentObj
		methodMap[renderObj.FuncName] = routerMethodTemplateDataObj{
			Name:            renderObj.FuncName,
			ParamSignature:  paramSignatureText,
			ResultSignature: resultSignatureText,
			Declaration:     buildFuncDeclaration(renderObj.FuncName, paramSignatureText, resultSignatureText, docMap[renderObj.FuncName], commentsFlag),
		}
	}

	nameArr := make([]string, 0, len(methodMap))
	for nameText := range methodMap {
		nameArr = append(nameArr, nameText)
	}
	sort.Strings(nameArr)

	resultArr := make([]routerMethodTemplateDataObj, 0, len(nameArr))
	for _, nameText := range nameArr {
		resultArr = append(resultArr, methodMap[nameText])
	}
	return resultArr, nil
}

// buildFuncDeclaration builds the interface method declaration together with its doc
// comment. The comment is composed from the route/scheme and the operation summary/description.
func buildFuncDeclaration(funcName string, paramSignature string, resultSignature string, docObj funcDocObj, commentsFlag bool) string {
	builderObj := strings.Builder{}
	if commentsFlag {
		for _, lineText := range buildFuncDocLineArr(funcName, docObj) {
			if lineText == "" {
				builderObj.WriteString("//\n")
				continue
			}

			builderObj.WriteString("// ")
			builderObj.WriteString(lineText)
			builderObj.WriteString("\n")
		}
	}

	builderObj.WriteString(funcName)
	builderObj.WriteString("(")
	builderObj.WriteString(paramSignature)
	builderObj.WriteString(")")
	if resultSignature != "" {
		builderObj.WriteString(" ")
		builderObj.WriteString(resultSignature)
	}

	return builderObj.String()
}

func buildDefaultErrorTemplateData(documentMap map[string]any, parseObj *ParseResultObj) (defaultErrorTemplateDataObj, error) {
	fallbackObj := defaultErrorTemplateDataObj{
		DeclareFallback: true,
		TypeName:        "ErrorType",
		StatusFieldName: "Error",
		StatusValue:     "http.StatusText(statusCode)",
		MessageField:    "Message",
	}

	schemaNameArr := defaultErrorSchemaNameArr(documentMap)
	for _, schemaName := range schemaNameArr {
		statusJSONName, statusValue, ok := defaultErrorSchemaFields(documentMap, schemaName)
		if !ok {
			continue
		}

		typeName, fieldMap, ok := findGeneratedSchemaStruct(parseObj, schemaName)
		if !ok {
			continue
		}

		statusField, statusOK := fieldMap[statusJSONName]
		messageField, messageOK := fieldMap["message"]
		if !statusOK || !messageOK {
			continue
		}
		if !isDefaultErrorStatusFieldType(statusJSONName, statusField.Type) {
			continue
		}
		if messageField.Type != "string" {
			continue
		}

		resultObj := defaultErrorTemplateDataObj{
			DeclareAlias:    typeName != "ErrorType",
			AliasTargetName: typeName,
			TypeName:        typeName,
			StatusFieldName: statusField.Name,
			StatusValue:     statusValue,
			MessageField:    messageField.Name,
		}
		if resultObj.DeclareAlias {
			resultObj.TypeName = "ErrorType"
		}

		return resultObj, nil
	}

	return fallbackObj, nil
}

func defaultErrorSchemaNameArr(documentMap map[string]any) []string {
	schemasMap := schemaComponentsMap(documentMap)
	if len(schemasMap) == 0 {
		return nil
	}

	priorityArr := []string{"defaulterrorobj", "defaulterror", "errorobj", "error"}
	resultArr := make([]string, 0, len(priorityArr))
	for _, priorityText := range priorityArr {
		for _, schemaName := range sortedKeys(schemasMap) {
			if normalizeName(schemaName) == priorityText {
				resultArr = append(resultArr, schemaName)
			}
		}
	}

	return resultArr
}

func defaultErrorSchemaFields(documentMap map[string]any, schemaName string) (string, string, bool) {
	schemaMap, _ := schemaComponentsMap(documentMap)[schemaName].(map[string]any)
	if len(schemaMap) == 0 {
		return "", "", false
	}
	typeText, _ := schemaMap["type"].(string)
	if strings.TrimSpace(typeText) != "" && strings.TrimSpace(typeText) != "object" {
		return "", "", false
	}

	requiredMap := stringSetFromArray(schemaMap["required"])

	propertiesMap, _ := schemaMap["properties"].(map[string]any)
	messageMap, _ := propertiesMap["message"].(map[string]any)
	if !isStringSchema(messageMap) {
		return "", "", false
	}
	if errorMap, ok := propertiesMap["error"].(map[string]any); ok && isStringSchema(errorMap) && requiredOnly(requiredMap, "error", "message") {
		return "error", "http.StatusText(statusCode)", true
	}
	if codeMap, ok := propertiesMap["code"].(map[string]any); ok && isIntegerSchema(codeMap) && requiredOnly(requiredMap, "code", "message") {
		return "code", codeStatusValue(codeMap), true
	}

	return "", "", false
}

func requiredOnly(requiredMap map[string]struct{}, allowedArr ...string) bool {
	allowedMap := make(map[string]struct{}, len(allowedArr))
	for _, allowedText := range allowedArr {
		allowedMap[allowedText] = struct{}{}
	}
	for requiredName := range requiredMap {
		if _, existsFlag := allowedMap[requiredName]; !existsFlag {
			return false
		}
	}

	return true
}

func schemaComponentsMap(documentMap map[string]any) map[string]any {
	componentsMap, _ := documentMap["components"].(map[string]any)
	schemasMap, _ := componentsMap["schemas"].(map[string]any)
	return schemasMap
}

func stringSetFromArray(rawValue any) map[string]struct{} {
	resultMap := make(map[string]struct{})
	valueArr, _ := rawValue.([]any)
	for _, value := range valueArr {
		text, _ := value.(string)
		text = strings.TrimSpace(text)
		if text != "" {
			resultMap[text] = struct{}{}
		}
	}

	return resultMap
}

func isStringSchema(schemaMap map[string]any) bool {
	typeText, _ := schemaMap["type"].(string)
	return strings.TrimSpace(typeText) == "string"
}

func isIntegerSchema(schemaMap map[string]any) bool {
	typeText, _ := schemaMap["type"].(string)
	return strings.TrimSpace(typeText) == "integer"
}

func codeStatusValue(schemaMap map[string]any) string {
	formatText, _ := schemaMap["format"].(string)
	switch strings.TrimSpace(formatText) {
	case "int64":
		return "int64(statusCode)"
	default:
		return "int32(statusCode)"
	}
}

func findGeneratedSchemaStruct(parseObj *ParseResultObj, schemaName string) (string, map[string]StructFieldObj, bool) {
	expectedName := normalizeName(schemaName + "Obj")
	fallbackName := normalizeName(schemaName)
	typeNameArr := make([]string, 0, len(parseObj.StructFieldMap))
	for typeName := range parseObj.StructFieldMap {
		typeNameArr = append(typeNameArr, typeName)
	}
	sort.Strings(typeNameArr)

	for _, typeName := range typeNameArr {
		normalizedType := normalizeName(typeName)
		if normalizedType == expectedName || normalizedType == fallbackName {
			return typeName, parseObj.StructFieldMap[typeName], true
		}
	}

	return "", nil, false
}

func isDefaultErrorStatusFieldType(jsonName string, typeText string) bool {
	switch jsonName {
	case "error":
		return typeText == "string"
	case "code":
		return typeText == "int" || typeText == "int32" || typeText == "int64"
	default:
		return false
	}
}

func renderSource(
	configObj ConfigObj,
	parseObj *ParseResultObj,
	funcMethodArr []routerMethodTemplateDataObj,
	handlerMethodArr []RenderMethodObj,
	securityMethodArr []RenderMethodObj,
	hasFuncInterfaceFlag bool,
	needErrorsFlag bool,
) (string, error) {
	dataObj := routerTemplateDataObj{
		Header:            write.GeneratedHeader,
		PackageName:       configObj.PackageName,
		ImportArr:         buildImportArr(parseObj, handlerMethodArr, securityMethodArr, hasFuncInterfaceFlag, needErrorsFlag),
		HandlerInterface:  parseObj.HandlerInterfaceName,
		SecurityInterface: parseObj.SecurityInterfaceName,
		ServerOption:      parseObj.ServerOptionName,
		HasFuncInterface:  hasFuncInterfaceFlag,
		HasSecurity:       len(securityMethodArr) > 0,
		FuncMethodArr:     funcMethodArr,
		HandlerMethodArr:  buildTemplateMethodArr("HandlerObj", "obj", handlerMethodArr),
		SecurityMethodArr: buildTemplateMethodArr("SecurityObj", "obj", securityMethodArr),
	}

	bufferObj := bytes.NewBuffer(nil)
	if err := routerTemplateObj.Execute(bufferObj, dataObj); err != nil {
		return "", fmt.Errorf("render router template: %w", err)
	}

	return bufferObj.String(), nil
}

func renderOpenAPIJSONSource(configObj ConfigObj) (string, error) {
	compressedLiteralText, err := buildCompressedJSONLiteral(configObj.OpenAPIJSON)
	if err != nil {
		return "", err
	}

	dataObj := openAPIJSONTemplateDataObj{
		Header:                write.GeneratedHeader,
		PackageName:           configObj.PackageName,
		CompressedJSONLiteral: compressedLiteralText,
	}

	bufferObj := bytes.NewBuffer(nil)
	if err = openAPIJSONTemplateObj.Execute(bufferObj, dataObj); err != nil {
		return "", fmt.Errorf("render OpenAPI JSON template: %w", err)
	}

	return bufferObj.String(), nil
}

func renderHTTPDefaultsSource(configObj ConfigObj, hasFuncInterfaceFlag bool, defaultErrorObj defaultErrorTemplateDataObj) (string, error) {
	dataObj := httpDefaultsTemplateDataObj{
		Header:           write.GeneratedHeader,
		PackageName:      configObj.PackageName,
		ServerOption:     configObj.ServerOptionName,
		HasFuncInterface: hasFuncInterfaceFlag,
		HasOpenAPIJSON:   configObj.HasOpenAPIJSON,
		DefaultError:     defaultErrorObj,
	}

	bufferObj := bytes.NewBuffer(nil)
	if err := httpDefaultsTemplateObj.Execute(bufferObj, dataObj); err != nil {
		return "", fmt.Errorf("render HTTP defaults template: %w", err)
	}

	return bufferObj.String(), nil
}

func buildTemplateMethodArr(receiverType string, receiverName string, methodArr []RenderMethodObj) []routerMethodTemplateDataObj {
	resultArr := make([]routerMethodTemplateDataObj, 0, len(methodArr))
	for _, renderObj := range methodArr {
		methodObj := renderObj.MethodObj
		resultArr = append(resultArr, routerMethodTemplateDataObj{
			ReceiverType:    receiverType,
			ReceiverName:    receiverName,
			Name:            methodObj.Name,
			ParamSignature:  buildParamSignature(methodObj.ParamArr),
			ResultSignature: buildResultSignature(methodObj.ResultArr),
			Body:            buildMethodBody(receiverName, renderObj),
		})
	}

	return resultArr
}

func buildImportArr(
	parseObj *ParseResultObj,
	handlerMethodArr []RenderMethodObj,
	securityMethodArr []RenderMethodObj,
	hasFuncInterfaceFlag bool,
	needErrorsFlag bool,
) []string {
	importMap := map[string]string{
		"net/http": "net/http",
	}
	if hasFuncInterfaceFlag || needErrorsFlag {
		importMap["errors"] = "errors"
	}

	// Take qualifiers from parsed types rather than substring matching, so that
	// "runtime." does not pull in a false "time" import.
	for qualifierText := range methodTypeQualifiers(handlerMethodArr, securityMethodArr) {
		if qualifierText == "api" {
			continue
		}
		if importPath, existsFlag := parseObj.ImportMap[qualifierText]; existsFlag {
			importMap[qualifierText] = importPath
		}
	}

	resultArr := make([]string, 0, len(importMap))
	for aliasText, importPath := range importMap {
		if aliasText == importPath || aliasText == path.Base(importPath) {
			resultArr = append(resultArr, strconv.Quote(importPath))
			continue
		}

		resultArr = append(resultArr, aliasText+" "+strconv.Quote(importPath))
	}
	sort.Strings(resultArr)
	return resultArr
}

// methodTypeQualifiers collects the package qualifiers actually used (the left side of
// SelectorExpr) from the parameter and result types of all methods.
func methodTypeQualifiers(handlerArr []RenderMethodObj, securityArr []RenderMethodObj) map[string]struct{} {
	resultMap := make(map[string]struct{})
	for _, renderObj := range append(handlerArr, securityArr...) {
		for _, fieldObj := range renderObj.MethodObj.ParamArr {
			collectTypeQualifiers(fieldObj.Type, resultMap)
		}
		for _, fieldObj := range renderObj.MethodObj.ResultArr {
			collectTypeQualifiers(fieldObj.Type, resultMap)
		}
	}

	return resultMap
}

func collectTypeQualifiers(typeText string, resultMap map[string]struct{}) {
	exprObj, err := parser.ParseExpr(typeText)
	if err != nil {
		return
	}

	ast.Inspect(exprObj, func(nodeObj ast.Node) bool {
		selectorObj, ok := nodeObj.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if identObj, ok := selectorObj.X.(*ast.Ident); ok {
			resultMap[identObj.Name] = struct{}{}
		}

		return true
	})
}

func buildMethodBody(receiverName string, renderObj RenderMethodObj) string {
	methodObj := renderObj.MethodObj
	builderObj := strings.Builder{}

	if renderObj.UseCall {
		callArgsText := buildCallArgs(methodObj.ParamArr)
		if len(methodObj.ResultArr) > 0 {
			builderObj.WriteString("\treturn ")
			builderObj.WriteString(receiverName)
			builderObj.WriteString(".FuncObj.")
			builderObj.WriteString(renderObj.FuncName)
			builderObj.WriteString("(")
			builderObj.WriteString(callArgsText)
			builderObj.WriteString(")\n")
		} else {
			builderObj.WriteString("\t")
			builderObj.WriteString(receiverName)
			builderObj.WriteString(".FuncObj.")
			builderObj.WriteString(renderObj.FuncName)
			builderObj.WriteString("(")
			builderObj.WriteString(callArgsText)
			builderObj.WriteString(")\n\treturn\n")
		}

		return builderObj.String()
	}

	return buildStubBody(methodObj)
}

func buildStubBody(methodObj MethodObj) string {
	builderObj := strings.Builder{}
	for _, paramObj := range methodObj.ParamArr {
		builderObj.WriteString("\t_ = ")
		builderObj.WriteString(paramObj.Name)
		builderObj.WriteString("\n")
	}
	if len(methodObj.ResultArr) == 0 {
		builderObj.WriteString("\treturn\n")
		return builderObj.String()
	}

	returnArr := make([]string, 0, len(methodObj.ResultArr))
	for index, resultObj := range methodObj.ResultArr {
		if resultObj.Type == "error" {
			returnArr = append(returnArr, "errors.New(\"not implemented\")")
			continue
		}

		nameText := fmt.Sprintf("result%d", index)
		builderObj.WriteString("\tvar ")
		builderObj.WriteString(nameText)
		builderObj.WriteString(" ")
		builderObj.WriteString(resultObj.Type)
		builderObj.WriteString("\n")
		returnArr = append(returnArr, nameText)
	}

	builderObj.WriteString("\treturn ")
	builderObj.WriteString(strings.Join(returnArr, ", "))
	builderObj.WriteString("\n")
	return builderObj.String()
}

func buildCompressedJSONLiteral(jsonArr []byte) (string, error) {
	bufferObj := bytes.NewBuffer(nil)
	// Raw DEFLATE at the best level: smaller than gzip (no header/CRC) and with no
	// third-party dependencies in the generated code.
	flateObj, err := flate.NewWriter(bufferObj, flate.BestCompression)
	if err != nil {
		return "", fmt.Errorf("create OpenAPI JSON compressor: %w", err)
	}
	if _, err = flateObj.Write(jsonArr); err != nil {
		return "", fmt.Errorf("compress OpenAPI JSON: %w", err)
	}
	if err = flateObj.Close(); err != nil {
		return "", fmt.Errorf("close OpenAPI JSON compressor: %w", err)
	}

	builderObj := strings.Builder{}
	for index, value := range bufferObj.Bytes() {
		if index%16 == 0 {
			builderObj.WriteString("\t")
		}
		builderObj.WriteString(fmt.Sprintf("0x%02x, ", value))
		if index%16 == 15 {
			builderObj.WriteString("\n")
		}
	}
	if builderObj.Len() > 0 && !strings.HasSuffix(builderObj.String(), "\n") {
		builderObj.WriteString("\n")
	}

	return builderObj.String(), nil
}

func buildParamSignature(fieldArr []FieldObj) string {
	partArr := make([]string, 0, len(fieldArr))
	for _, fieldObj := range fieldArr {
		partArr = append(partArr, fieldObj.Name+" "+fieldObj.Type)
	}

	return strings.Join(partArr, ", ")
}

func buildResultSignature(fieldArr []FieldObj) string {
	if len(fieldArr) == 0 {
		return ""
	}

	partArr := make([]string, 0, len(fieldArr))
	for _, fieldObj := range fieldArr {
		if fieldObj.Named && fieldObj.Name != "" {
			partArr = append(partArr, fieldObj.Name+" "+fieldObj.Type)
		} else {
			partArr = append(partArr, fieldObj.Type)
		}
	}
	if len(partArr) == 1 {
		return partArr[0]
	}

	return "(" + strings.Join(partArr, ", ") + ")"
}

func buildCallArgs(fieldArr []FieldObj) string {
	argArr := make([]string, 0, len(fieldArr))
	for _, fieldObj := range fieldArr {
		argArr = append(argArr, fieldObj.Name)
	}

	return strings.Join(argArr, ", ")
}

func hasErrorResult(fieldArr []FieldObj) bool {
	for _, fieldObj := range fieldArr {
		if fieldObj.Type == "error" {
			return true
		}
	}

	return false
}
