package bridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// // // // // // // // // //

func TestGenerateBridgeFromOgenInterfaces(t *testing.T) {
	ogenDir := t.TempDir()
	writeBridgeTestFile(t, ogenDir, "oas_server_gen.go", `
package api

import "context"

type Handler interface {
	TestGet(ctx context.Context) (TestGetOK, error)
	TestDelete(ctx context.Context) error
}

type SecurityHandler interface {
	HandleBearerAuth(ctx context.Context, operationName string) error
}

type TestGetOK struct{}
`)

	dataArr, err := Generate(ConfigObj{
		OgenDir:     ogenDir,
		PackageName: "api",
		Document: map[string]any{
			"paths": map[string]any{
				"/test": map[string]any{
					"get": map[string]any{
						"operationId": "testGet",
						"x-func":      "TestGet",
					},
					"delete": map[string]any{
						"operationId": "testDelete",
					},
				},
			},
			"components": map[string]any{
				"securitySchemes": map[string]any{
					"BearerAuth": map[string]any{
						"x-func": "VerifyBearer",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("generate bridge: %v", err)
	}

	sourceText := string(dataArr)
	for _, expectedText := range []string{
		"type FuncInterface interface {",
		"TestGet(ctx context.Context) (TestGetOK, error)",
		"VerifyBearer(ctx context.Context, operationName string) error",
		"func NewHandler(funcObj FuncInterface, optionsArr ...ServerOption) (http.Handler, error)",
		"errors.New(\"func implementation is nil\")",
		"obj.FuncObj.TestGet(ctx)",
		"errors.New(\"not implemented\")",
		"obj.FuncObj.VerifyBearer(ctx, operationName)",
	} {
		if !strings.Contains(sourceText, expectedText) {
			t.Fatalf("generated bridge does not contain %q:\n%s", expectedText, sourceText)
		}
	}
	for _, unexpectedText := range []string{
		"example.com/project/src",
		"example.com/project/include",
		"handler.",
		"RuntimeObj",
		"example.com/project/target/api",
		"api.",
		"OpenAPIJSON",
		"openAPIJSONCompressedArr",
	} {
		if strings.Contains(sourceText, unexpectedText) {
			t.Fatalf("generated bridge contains obsolete reference %q:\n%s", unexpectedText, sourceText)
		}
	}
}

func TestGenerateBridgeSkipsNewErrorServiceMethod(t *testing.T) {
	ogenDir := t.TempDir()
	writeBridgeTestFile(t, ogenDir, "oas_server_gen.go", `
package api

import "context"

type Handler interface {
	TestGet(ctx context.Context) (TestGetOK, error)
	NewError(ctx context.Context, err error) *ErrorStatusCode
}

type TestGetOK struct{}
type ErrorStatusCode struct{}
`)

	dataArr, err := Generate(ConfigObj{
		OgenDir:     ogenDir,
		PackageName: "api",
		Document: map[string]any{
			"paths": map[string]any{
				"/test": map[string]any{
					"get": map[string]any{
						"operationId": "testGet",
						"x-func":      "TestGet",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("generate bridge: %v", err)
	}

	sourceText := string(dataArr)
	if !strings.Contains(sourceText, "obj.FuncObj.TestGet(ctx)") {
		t.Fatalf("generated bridge lost the mapped operation:\n%s", sourceText)
	}
	for _, unexpectedText := range []string{
		"func (obj *HandlerObj) NewError(",
		"NewError(ctx context.Context, err error)",
	} {
		if strings.Contains(sourceText, unexpectedText) {
			t.Fatalf("generated bridge must skip NewError service method, found %q:\n%s", unexpectedText, sourceText)
		}
	}
}

func TestGenerateBridgeFromRenamedOgenInterfaces(t *testing.T) {
	ogenDir := t.TempDir()
	writeBridgeTestFile(t, ogenDir, "oas_server_gen.go", `
package api

import "context"

type HandlerInterface interface {
	TestGet(ctx context.Context, req *TestRequestObj) (TestGetResInterface, error)
}

type SecurityHandlerInterface interface {
	HandleBearerAuth(ctx context.Context, operationName string) error
}

type ServerOptionInterface interface {
	applyServer(*serverConfigObj)
}

type serverConfigObj struct{}
type TestRequestObj struct{}

type TestGetResInterface interface {
	testGetRes()
}
`)

	dataArr, err := Generate(ConfigObj{
		OgenDir:     ogenDir,
		PackageName: "api",
		Document: map[string]any{
			"paths": map[string]any{
				"/test": map[string]any{
					"get": map[string]any{
						"operationId": "testGet",
						"x-func":      "TestGet",
					},
				},
			},
			"components": map[string]any{
				"securitySchemes": map[string]any{
					"BearerAuth": map[string]any{
						"x-func": "VerifyBearer",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("generate bridge: %v", err)
	}

	sourceText := string(dataArr)
	for _, expectedText := range []string{
		"var _ HandlerInterface = (*HandlerObj)(nil)",
		"var _ SecurityHandlerInterface = (*SecurityObj)(nil)",
		"TestGet(ctx context.Context, req *TestRequestObj) (TestGetResInterface, error)",
		"func NewHandler(funcObj FuncInterface, optionsArr ...ServerOptionInterface) (http.Handler, error)",
	} {
		if !strings.Contains(sourceText, expectedText) {
			t.Fatalf("generated bridge does not contain %q:\n%s", expectedText, sourceText)
		}
	}
}

func TestGenerateOpenAPIJSON(t *testing.T) {
	dataArr, err := GenerateOpenAPIJSON(ConfigObj{
		PackageName: "api",
		OpenAPIJSON: []byte(`{"openapi":"3.0.3"}`),
	})
	if err != nil {
		t.Fatalf("generate OpenAPI JSON Go file: %v", err)
	}

	sourceText := string(dataArr)
	for _, expectedText := range []string{
		"package api",
		"func OpenAPIJSON() ([]byte, error)",
		"var openAPIJSONCompressedArr = []byte{",
		"sync.OnceValues(func() ([]byte, error) {",
		"flate.NewReader(bytes.NewReader(openAPIJSONCompressedArr))",
		"return openAPIJSONFunc()",
	} {
		if !strings.Contains(sourceText, expectedText) {
			t.Fatalf("generated OpenAPI JSON Go file does not contain %q:\n%s", expectedText, sourceText)
		}
	}
	for _, unexpectedText := range []string{
		"openAPIJSONOnceObj",
		"openAPIJSONArr",
		"openAPIJSONErr",
	} {
		if strings.Contains(sourceText, unexpectedText) {
			t.Fatalf("generated OpenAPI JSON Go file contains obsolete state %q:\n%s", unexpectedText, sourceText)
		}
	}
}

func TestGenerateHTTPDefaultsWithFuncInterface(t *testing.T) {
	ogenDir := t.TempDir()
	writeBridgeTestFile(t, ogenDir, "oas_server_gen.go", `
package api

import "context"

type Handler interface {
	TestGet(ctx context.Context) error
}

type ServerOptionInterface interface {
	applyServer(*serverConfigObj)
}

type serverConfigObj struct{}
`)

	dataArr, err := GenerateHTTPDefaults(ConfigObj{
		OgenDir:        ogenDir,
		PackageName:    "api",
		HasOpenAPIJSON: true,
		Document: map[string]any{
			"paths": map[string]any{
				"/test": map[string]any{
					"get": map[string]any{
						"operationId": "testGet",
						"x-func":      "TestGet",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("generate HTTP defaults: %v", err)
	}

	sourceText := string(dataArr)
	for _, expectedText := range []string{
		"func NewDefaultHandler(funcObj FuncInterface, optionsArr ...ServerOptionInterface) (http.Handler, error)",
		"defaultOptionArr := []ServerOptionInterface{",
		"WithErrorHandler(defaultErrorHandler)",
		"WithNotFound(defaultNotFoundHandler)",
		"WithMethodNotAllowed(defaultMethodNotAllowedHandler)",
		"handlerObj, err := NewHandler(funcObj, defaultOptionArr...)",
		"case cDefaultOpenAPIJSONPath:",
		"dataArr, err := OpenAPIJSON()",
		"type ErrorType struct",
		"`json:\"error\"`",
		"`json:\"message\"`",
		"ErrorType{",
	} {
		if !strings.Contains(sourceText, expectedText) {
			t.Fatalf("generated HTTP defaults do not contain %q:\n%s", expectedText, sourceText)
		}
	}
}

func TestGenerateHTTPDefaultsUsesGeneratedDefaultError(t *testing.T) {
	ogenDir := t.TempDir()
	writeBridgeTestFile(t, ogenDir, "oas_server_gen.go", `
package api

type Handler interface {
	TestGet() error
}

type DefaultErrorObj struct {
	Error string `+"`json:\"error\"`"+`
	Message string `+"`json:\"message\"`"+`
}
`)

	dataArr, err := GenerateHTTPDefaults(ConfigObj{
		OgenDir:     ogenDir,
		PackageName: "api",
		Document: map[string]any{
			"paths": map[string]any{
				"/test": map[string]any{
					"get": map[string]any{
						"operationId": "testGet",
					},
				},
			},
			"components": map[string]any{
				"schemas": map[string]any{
					"default-error": map[string]any{
						"type":     "object",
						"required": []any{"error", "message"},
						"properties": map[string]any{
							"error": map[string]any{
								"type": "string",
							},
							"message": map[string]any{
								"type": "string",
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("generate HTTP defaults: %v", err)
	}

	sourceText := string(dataArr)
	for _, expectedText := range []string{
		"type ErrorType = DefaultErrorObj",
		"ErrorType{",
		"Error:   http.StatusText(statusCode)",
		"Message: messageText",
	} {
		if !strings.Contains(sourceText, expectedText) {
			t.Fatalf("generated HTTP defaults do not contain %q:\n%s", expectedText, sourceText)
		}
	}
	if strings.Contains(sourceText, "type ErrorType struct") {
		t.Fatalf("generated HTTP defaults contain fallback error type:\n%s", sourceText)
	}
}

func TestGenerateHTTPDefaultsUsesGeneratedCodeMessageError(t *testing.T) {
	ogenDir := t.TempDir()
	writeBridgeTestFile(t, ogenDir, "oas_server_gen.go", `
package api

type Handler interface {
	TestGet() error
}

type ErrorObj struct {
	Code int32 `+"`json:\"code\"`"+`
	Message string `+"`json:\"message\"`"+`
}
`)

	dataArr, err := GenerateHTTPDefaults(ConfigObj{
		OgenDir:     ogenDir,
		PackageName: "api",
		Document: map[string]any{
			"paths": map[string]any{
				"/test": map[string]any{
					"get": map[string]any{
						"operationId": "testGet",
					},
				},
			},
			"components": map[string]any{
				"schemas": map[string]any{
					"Error": map[string]any{
						"type":     "object",
						"required": []any{"code", "message"},
						"properties": map[string]any{
							"code": map[string]any{
								"type":   "integer",
								"format": "int32",
							},
							"message": map[string]any{
								"type": "string",
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("generate HTTP defaults: %v", err)
	}

	sourceText := string(dataArr)
	for _, expectedText := range []string{
		"type ErrorType = ErrorObj",
		"ErrorType{",
		"Code:    int32(statusCode)",
		"Message: messageText",
	} {
		if !strings.Contains(sourceText, expectedText) {
			t.Fatalf("generated HTTP defaults do not contain %q:\n%s", expectedText, sourceText)
		}
	}
}

func TestGenerateHTTPDefaultsWithoutFuncInterface(t *testing.T) {
	ogenDir := t.TempDir()
	writeBridgeTestFile(t, ogenDir, "oas_server_gen.go", `
package api

type Handler interface {
	TestGet() error
}
`)

	dataArr, err := GenerateHTTPDefaults(ConfigObj{
		OgenDir:     ogenDir,
		PackageName: "api",
		Document: map[string]any{
			"paths": map[string]any{
				"/test": map[string]any{
					"get": map[string]any{
						"operationId": "testGet",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("generate HTTP defaults: %v", err)
	}

	sourceText := string(dataArr)
	for _, expectedText := range []string{
		"func NewDefaultHandler(optionsArr ...ServerOption) (http.Handler, error)",
		"handlerObj, err := NewHandler(defaultOptionArr...)",
		"return handlerObj, nil",
	} {
		if !strings.Contains(sourceText, expectedText) {
			t.Fatalf("generated HTTP defaults do not contain %q:\n%s", expectedText, sourceText)
		}
	}
	for _, unexpectedText := range []string{
		"FuncInterface",
		"OpenAPIJSON()",
		"cDefaultOpenAPIJSONPath:",
	} {
		if strings.Contains(sourceText, unexpectedText) {
			t.Fatalf("generated HTTP defaults contain unexpected %q:\n%s", unexpectedText, sourceText)
		}
	}
}

func TestGenerateBridgeXFuncCollision(t *testing.T) {
	ogenDir := t.TempDir()
	writeBridgeTestFile(t, ogenDir, "oas_server_gen.go", `
package api

import "context"

type Handler interface {
	First(ctx context.Context) error
	Second(ctx context.Context, id string) error
}
`)

	_, err := Generate(ConfigObj{
		OgenDir:     ogenDir,
		PackageName: "api",
		Document: map[string]any{
			"paths": map[string]any{
				"/first": map[string]any{
					"get": map[string]any{
						"operationId": "first",
						"x-func":      "Do",
					},
				},
				"/second": map[string]any{
					"get": map[string]any{
						"operationId": "second",
						"x-func":      "Do",
					},
				},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "x-func collision for Do") {
		t.Fatalf("expected x-func collision error, got %v", err)
	}
}

func TestGenerateBridgeRejectsOperationIDNormalizationCollision(t *testing.T) {
	ogenDir := t.TempDir()
	writeBridgeTestFile(t, ogenDir, "oas_server_gen.go", `
package api

type Handler interface {
	TestGet() error
}
`)

	_, err := Generate(ConfigObj{
		OgenDir:     ogenDir,
		PackageName: "api",
		Document: map[string]any{
			"paths": map[string]any{
				"/first": map[string]any{
					"get": map[string]any{
						"operationId": "foo-bar",
						"x-func":      "First",
					},
				},
				"/second": map[string]any{
					"post": map[string]any{
						"operationId": "foo_bar",
						"x-func":      "Second",
					},
				},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "operationId normalization collision") {
		t.Fatalf("expected operationId normalization collision error, got %v", err)
	}
}

func TestGenerateBridgeRejectsSecurityNormalizationCollision(t *testing.T) {
	ogenDir := t.TempDir()
	writeBridgeTestFile(t, ogenDir, "oas_server_gen.go", `
package api

type Handler interface {
	TestGet() error
}
`)

	_, err := Generate(ConfigObj{
		OgenDir:     ogenDir,
		PackageName: "api",
		Document: map[string]any{
			"paths": map[string]any{
				"/test": map[string]any{
					"get": map[string]any{
						"operationId": "testGet",
					},
				},
			},
			"components": map[string]any{
				"securitySchemes": map[string]any{
					"Api-Key": map[string]any{
						"x-func": "First",
					},
					"Api_Key": map[string]any{
						"x-func": "Second",
					},
				},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "security scheme normalization collision") {
		t.Fatalf("expected security normalization collision error, got %v", err)
	}
}

func TestGenerateBridgeRequireXFunc(t *testing.T) {
	ogenDir := t.TempDir()
	writeBridgeTestFile(t, ogenDir, "oas_server_gen.go", `
package api

type Handler interface {
	TestGet() error
}
`)

	_, err := Generate(ConfigObj{
		OgenDir:      ogenDir,
		PackageName:  "api",
		RequireXFunc: true,
		Document: map[string]any{
			"paths": map[string]any{
				"/test": map[string]any{
					"get": map[string]any{
						"operationId": "testGet",
					},
				},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "missing x-func mappings") {
		t.Fatalf("expected missing x-func error, got %v", err)
	}
}

func TestGenerateBridgeRejectsInvalidXFunc(t *testing.T) {
	for _, funcName := range []string{"testGet", "func", "Test-Get"} {
		t.Run(funcName, func(t *testing.T) {
			ogenDir := t.TempDir()
			writeBridgeTestFile(t, ogenDir, "oas_server_gen.go", `
package api

type Handler interface {
	TestGet() error
}
`)

			_, err := Generate(ConfigObj{
				OgenDir:     ogenDir,
				PackageName: "api",
				Document: map[string]any{
					"paths": map[string]any{
						"/test": map[string]any{
							"get": map[string]any{
								"operationId": "testGet",
								"x-func":      funcName,
							},
						},
					},
				},
			})
			if err == nil || !strings.Contains(err.Error(), "invalid x-func") {
				t.Fatalf("expected invalid x-func error, got %v", err)
			}
		})
	}
}

func writeBridgeTestFile(t *testing.T, rootPath string, relPath string, contentText string) {
	t.Helper()

	filePath := filepath.Join(rootPath, relPath)
	if err := os.WriteFile(filePath, []byte(strings.TrimLeft(contentText, "\n")), 0o644); err != nil {
		t.Fatalf("write bridge test file: %v", err)
	}
}
