package source

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// // // // // // // // // //

func TestLoadRootSidecarsAndMerge(t *testing.T) {
	rootPath := t.TempDir()
	writeTestFile(t, rootPath, "openapi.yaml", `
openapi: 3.0.3
info:
  title: Test API
  version: 1.2.3
`)
	writeTestFile(t, rootPath, "tags.yml", `
- name: tasks
`)
	writeTestFile(t, rootPath, "servers.json", `[{"url":"https://api.example.test"}]`)
	writeTestFile(t, rootPath, "security.yaml", `
- BearerAuth: []
`)
	writeTestFile(t, rootPath, "paths/tasks.yml", `
/tasks:
  get:
    operationId: tasksList
    x-func: TasksList
    responses:
      "200":
        description: OK
`)
	writeTestFile(t, rootPath, "components/auth.yaml", `
securitySchemes:
  BearerAuth:
    type: http
    scheme: bearer
    x-func: VerifyBearer
`)

	graphObj, err := Load(OptionsObj{Source: rootPath})
	if err != nil {
		t.Fatalf("load source: %v", err)
	}

	if _, existsFlag := graphObj.Document["tags"]; !existsFlag {
		t.Fatalf("tags sidecar was not applied")
	}
	if _, existsFlag := graphObj.Document["servers"]; !existsFlag {
		t.Fatalf("servers sidecar was not applied")
	}
	if _, existsFlag := graphObj.Document["security"]; !existsFlag {
		t.Fatalf("security sidecar was not applied")
	}

	pathsMap, ok := graphObj.Document["paths"].(map[string]any)
	if !ok {
		t.Fatalf("paths field has unexpected type %T", graphObj.Document["paths"])
	}
	if _, existsFlag := pathsMap["/tasks"]; !existsFlag {
		t.Fatalf("paths merge did not include /tasks")
	}
}

func TestRootSidecarCollision(t *testing.T) {
	rootPath := t.TempDir()
	writeTestFile(t, rootPath, "openapi.yaml", `
openapi: 3.0.3
info:
  title: Test API
  version: 1.2.3
tags: []
`)
	writeTestFile(t, rootPath, "tags.yml", `
- name: tasks
`)

	_, err := Load(OptionsObj{Source: rootPath})
	if err == nil || !strings.Contains(err.Error(), "root sidecar collision") {
		t.Fatalf("expected sidecar collision, got %v", err)
	}
}

func TestRootTagsSidecarNamedMap(t *testing.T) {
	rootPath := t.TempDir()
	writeTestFile(t, rootPath, "openapi.yaml", `
openapi: 3.0.3
info:
  title: Test API
  version: 1.2.3
`)
	writeTestFile(t, rootPath, "tags.yaml", `
tasks:
  description: Task operations.
auth:
  name: auth-custom
  description: Auth operations.
`)

	graphObj, err := Load(OptionsObj{Source: rootPath})
	if err != nil {
		t.Fatalf("load source: %v", err)
	}

	tagArr, ok := graphObj.Document["tags"].([]any)
	if !ok {
		t.Fatalf("tags sidecar has unexpected type %T", graphObj.Document["tags"])
	}
	if len(tagArr) != 2 {
		t.Fatalf("unexpected tags count: %d", len(tagArr))
	}

	firstMap, _ := tagArr[0].(map[string]any)
	secondMap, _ := tagArr[1].(map[string]any)
	if firstMap["name"] != "auth-custom" || secondMap["name"] != "tasks" {
		t.Fatalf("unexpected normalized tags: %#v", tagArr)
	}
}

func TestRootSidecarWrapperRejected(t *testing.T) {
	rootPath := t.TempDir()
	writeTestFile(t, rootPath, "openapi.yaml", `
openapi: 3.0.3
info:
  title: Test API
  version: 1.2.3
`)
	writeTestFile(t, rootPath, "tags.yml", `
tags:
  - name: tasks
`)

	_, err := Load(OptionsObj{Source: rootPath})
	if err == nil || !strings.Contains(err.Error(), "must contain field value directly") {
		t.Fatalf("expected wrapper error, got %v", err)
	}
}

func TestLegacyComponentRootFilesMergeToSections(t *testing.T) {
	rootPath := t.TempDir()
	writeTestFile(t, rootPath, "openapi.yaml", `
openapi: 3.0.3
info:
  title: Test API
  version: 1.2.3
`)
	writeTestFile(t, rootPath, "components/parameters.yaml", `
TaskID:
  name: task_id
  in: path
  required: true
  schema:
    type: string
`)
	writeTestFile(t, rootPath, "components/security_schemes.yaml", `
ApiKeyHeader:
  type: apiKey
  in: header
  name: X-Api-Key
`)

	graphObj, err := Load(OptionsObj{Source: rootPath})
	if err != nil {
		t.Fatalf("load source: %v", err)
	}

	componentsMap, _ := graphObj.Document["components"].(map[string]any)
	parametersMap, _ := componentsMap["parameters"].(map[string]any)
	securityMap, _ := componentsMap["securitySchemes"].(map[string]any)
	if _, existsFlag := parametersMap["TaskID"]; !existsFlag {
		t.Fatalf("legacy parameters file was not merged into components.parameters: %#v", componentsMap)
	}
	if _, existsFlag := securityMap["ApiKeyHeader"]; !existsFlag {
		t.Fatalf("legacy security_schemes file was not merged into components.securitySchemes: %#v", componentsMap)
	}
}

func TestLoadLegacyPathItemFiles(t *testing.T) {
	rootPath := t.TempDir()
	writeTestFile(t, rootPath, "openapi.yaml", `
openapi: 3.0.3
info:
  title: Test API
  version: 1.2.3
`)
	writeTestFile(t, rootPath, "paths/auth/login.yaml", `
post:
  operationId: authLogin
  responses:
    "200":
      description: OK
`)
	writeTestFile(t, rootPath, "paths/systems/[system_id]/status.yaml", `
get:
  operationId: systemStatus
  responses:
    "200":
      description: OK
`)
	writeTestFile(t, rootPath, "paths/.well-known/openid_configuration.yaml", `
get:
  operationId: oidcConfiguration
  responses:
    "200":
      description: OK
`)

	graphObj, err := Load(OptionsObj{Source: rootPath})
	if err != nil {
		t.Fatalf("load source: %v", err)
	}

	pathsMap, ok := graphObj.Document["paths"].(map[string]any)
	if !ok {
		t.Fatalf("paths field has unexpected type %T", graphObj.Document["paths"])
	}

	for _, pathText := range []string{
		"/auth/login",
		"/systems/{system_id}/status",
		"/.well-known/openid_configuration",
	} {
		if _, existsFlag := pathsMap[pathText]; !existsFlag {
			t.Fatalf("paths merge did not include %s: %v", pathText, pathsMap)
		}
	}
}

func TestUnreferencedFileOutsideGraphDoesNotChangeHash(t *testing.T) {
	rootPath := t.TempDir()
	writeMinimalSource(t, rootPath)

	firstGraphObj, err := Load(OptionsObj{Source: rootPath})
	if err != nil {
		t.Fatalf("load first graph: %v", err)
	}

	writeTestFile(t, rootPath, "unused/schema.yaml", `
type: object
`)

	secondGraphObj, err := Load(OptionsObj{Source: rootPath})
	if err != nil {
		t.Fatalf("load second graph: %v", err)
	}

	if firstGraphObj.Hash != secondGraphObj.Hash {
		t.Fatalf("hash changed after adding unreferenced file: %s != %s", firstGraphObj.Hash, secondGraphObj.Hash)
	}
}

func TestRefOutsideSourceRootFails(t *testing.T) {
	parentPath := t.TempDir()
	rootPath := filepath.Join(parentPath, "api")
	if err := os.MkdirAll(rootPath, 0o755); err != nil {
		t.Fatalf("create api dir: %v", err)
	}

	writeTestFile(t, parentPath, "outside.yaml", `
type: object
`)
	writeTestFile(t, rootPath, "openapi.yaml", `
openapi: 3.0.3
info:
  title: Test API
  version: 1.2.3
paths:
  /x:
    get:
      operationId: xGet
      responses:
        "200":
          description: OK
          content:
            application/json:
              schema:
                $ref: ../outside.yaml
`)

	_, err := Load(OptionsObj{Source: rootPath})
	if err == nil || !strings.Contains(err.Error(), "escapes source root") {
		t.Fatalf("expected source root escape error, got %v", err)
	}
}

func TestPublicDocumentUsesCanonicalComponentRefs(t *testing.T) {
	rootPath := t.TempDir()
	writeTestFile(t, rootPath, "openapi.yaml", `
openapi: 3.0.3
info:
  title: Test API
  version: 1.2.3
`)
	writeTestFile(t, rootPath, "paths/tasks.yaml", `
get:
  operationId: tasksList
  responses:
    "200":
      $ref: responses/task_list
`)
	writeTestFile(t, rootPath, "components/responses/task_list.yaml", `
description: Task list.
content:
  application/json:
    schema:
      $ref: schemas/task
`)
	writeTestFile(t, rootPath, "components/schemas/task.yaml", `
type: object
properties:
  id:
    type: string
`)

	graphObj, err := Load(OptionsObj{Source: rootPath})
	if err != nil {
		t.Fatalf("load source: %v", err)
	}

	documentMap, err := graphObj.PublicDocument("1.2.3")
	if err != nil {
		t.Fatalf("build public document: %v", err)
	}

	pathsMap, _ := documentMap["paths"].(map[string]any)
	pathMap, _ := pathsMap["/tasks"].(map[string]any)
	getMap, _ := pathMap["get"].(map[string]any)
	responsesMap, _ := getMap["responses"].(map[string]any)
	okMap, _ := responsesMap["200"].(map[string]any)
	if okMap["$ref"] != "#/components/responses/TaskListRespObj" {
		t.Fatalf("unexpected response ref: %#v", okMap["$ref"])
	}

	componentsMap, _ := documentMap["components"].(map[string]any)
	componentResponsesMap, _ := componentsMap["responses"].(map[string]any)
	taskListMap, _ := componentResponsesMap["TaskListRespObj"].(map[string]any)
	contentMap, _ := taskListMap["content"].(map[string]any)
	jsonMap, _ := contentMap["application/json"].(map[string]any)
	schemaMap, _ := jsonMap["schema"].(map[string]any)
	if schemaMap["$ref"] != "#/components/schemas/TaskObj" {
		t.Fatalf("unexpected schema ref: %#v", schemaMap["$ref"])
	}
}

func TestRootComponentPointerUsesSuffixedComponentAlias(t *testing.T) {
	rootPath := t.TempDir()
	writeTestFile(t, rootPath, "openapi.yaml", `
openapi: 3.0.3
info:
  title: Test API
  version: 1.2.3
`)
	writeTestFile(t, rootPath, "components/common.yaml", `
responses:
  Problem:
    description: Error response.
    content:
      application/json:
        schema:
          $ref: '#/components/schemas/DefaultError'
`)
	writeTestFile(t, rootPath, "components/schemas/default_error.yaml", `
type: object
required: [code, message]
properties:
  code:
    type: integer
  message:
    type: string
`)

	graphObj, err := Load(OptionsObj{Source: rootPath})
	if err != nil {
		t.Fatalf("load source: %v", err)
	}

	documentMap, err := graphObj.PublicDocument("1.2.3")
	if err != nil {
		t.Fatalf("build public document: %v", err)
	}

	componentsMap, _ := documentMap["components"].(map[string]any)
	responsesMap, _ := componentsMap["responses"].(map[string]any)
	problemMap, _ := responsesMap["ProblemRespObj"].(map[string]any)
	contentMap, _ := problemMap["content"].(map[string]any)
	jsonMap, _ := contentMap["application/json"].(map[string]any)
	schemaMap, _ := jsonMap["schema"].(map[string]any)
	if schemaMap["$ref"] != "#/components/schemas/DefaultErrorObj" {
		t.Fatalf("unexpected schema ref: %#v", schemaMap["$ref"])
	}

	documentMap, err = graphObj.PublicDocumentWithOptions("1.2.3", PublicOptionsObj{
		CanonicalComponentRefs: false,
	})
	if err != nil {
		t.Fatalf("build public document without canonical file refs: %v", err)
	}

	componentsMap, _ = documentMap["components"].(map[string]any)
	responsesMap, _ = componentsMap["responses"].(map[string]any)
	problemMap, _ = responsesMap["ProblemRespObj"].(map[string]any)
	contentMap, _ = problemMap["content"].(map[string]any)
	jsonMap, _ = contentMap["application/json"].(map[string]any)
	schemaMap, _ = jsonMap["schema"].(map[string]any)
	if schemaMap["$ref"] != "#/components/schemas/DefaultErrorObj" {
		t.Fatalf("unexpected schema ref without canonical file refs: %#v", schemaMap["$ref"])
	}
}

func TestInlineComponentNamesGetSuffix(t *testing.T) {
	rootPath := t.TempDir()
	writeTestFile(t, rootPath, "openapi.yaml", `
openapi: 3.0.3
info:
  title: Test API
  version: 1.0.0
`)
	writeTestFile(t, rootPath, "components/common.yaml", `
schemas:
  Status:
    type: object
    properties:
      detail:
        $ref: '#/components/schemas/Detail'
  Detail:
    type: object
    properties:
      code:
        type: integer
`)

	graphObj, err := Load(OptionsObj{Source: rootPath})
	if err != nil {
		t.Fatalf("load source: %v", err)
	}

	documentMap, err := graphObj.PublicDocument("1.0.0")
	if err != nil {
		t.Fatalf("build public document: %v", err)
	}

	schemasMap, _ := documentMap["components"].(map[string]any)["schemas"].(map[string]any)
	if _, existsFlag := schemasMap["StatusObj"]; !existsFlag {
		t.Fatalf("inline schema Status was not suffixed: %v", sortedMapKeys(schemasMap))
	}
	if _, existsFlag := schemasMap["DetailObj"]; !existsFlag {
		t.Fatalf("inline schema Detail was not suffixed: %v", sortedMapKeys(schemasMap))
	}
	if _, existsFlag := schemasMap["Status"]; existsFlag {
		t.Fatalf("unsuffixed inline schema name should not remain")
	}

	statusMap, _ := schemasMap["StatusObj"].(map[string]any)
	propsMap, _ := statusMap["properties"].(map[string]any)
	detailMap, _ := propsMap["detail"].(map[string]any)
	if detailMap["$ref"] != "#/components/schemas/DetailObj" {
		t.Fatalf("inline cross-ref not rewritten: %#v", detailMap["$ref"])
	}
}

func TestResolveRootRelativeShorthandRefs(t *testing.T) {
	rootPath := t.TempDir()
	writeTestFile(t, rootPath, "openapi.yaml", `
openapi: 3.0.3
info:
  title: Test API
  version: 1.2.3
`)
	writeTestFile(t, rootPath, "paths/.well-known/openid_configuration.yaml", `
get:
  operationId: oidcConfiguration
  responses:
    "200":
      $ref: responses/error
`)
	writeTestFile(t, rootPath, "components/responses/error.yaml", `
description: Error response
content:
  application/json:
    schema:
      $ref: schemas/error
`)
	writeTestFile(t, rootPath, "components/schemas/error.yaml", `
type: object
`)

	loaderObj := newLoader(rootPath, filepath.Join(rootPath, "openapi.yaml"))
	pathFile := filepath.Join(rootPath, "paths/.well-known/openid_configuration.yaml")
	responseFile := filepath.Join(rootPath, "components/responses/error.yaml")
	schemaFile := filepath.Join(rootPath, "components/schemas/error.yaml")

	resolvedResponseFile, err := resolveRefPath(loaderObj, pathFile, "responses/error")
	if err != nil {
		t.Fatalf("resolve response shorthand: %v", err)
	}
	if resolvedResponseFile != responseFile {
		t.Fatalf("unexpected response ref target: %s", resolvedResponseFile)
	}

	resolvedSchemaFile, err := resolveRefPath(loaderObj, responseFile, "schemas/error")
	if err != nil {
		t.Fatalf("resolve schema shorthand: %v", err)
	}
	if resolvedSchemaFile != schemaFile {
		t.Fatalf("unexpected schema ref target: %s", resolvedSchemaFile)
	}
}

func TestResolveRelativeRefCanStayInsideSourceRoot(t *testing.T) {
	rootPath := t.TempDir()
	writeTestFile(t, rootPath, "openapi.yaml", `
openapi: 3.0.3
info:
  title: Test API
  version: 1.2.3
`)
	writeTestFile(t, rootPath, "components/responses/error.yaml", `
description: Error response
`)
	writeTestFile(t, rootPath, "components/schemas/error.yaml", `
type: object
`)

	loaderObj := newLoader(rootPath, filepath.Join(rootPath, "openapi.yaml"))
	responseFile := filepath.Join(rootPath, "components/responses/error.yaml")
	schemaFile := filepath.Join(rootPath, "components/schemas/error.yaml")

	resolvedSchemaFile, err := resolveRefPath(loaderObj, responseFile, "../schemas/error")
	if err != nil {
		t.Fatalf("resolve parent relative ref: %v", err)
	}
	if resolvedSchemaFile != schemaFile {
		t.Fatalf("unexpected parent relative ref target: %s", resolvedSchemaFile)
	}
}

func TestResolveRootRelativeRefFallsBackToCurrentFile(t *testing.T) {
	rootPath := t.TempDir()
	writeTestFile(t, rootPath, "openapi.yaml", `
openapi: 3.0.3
info:
  title: Test API
  version: 1.2.3
`)
	writeTestFile(t, rootPath, "paths/item.yaml", `
get:
  operationId: itemGet
`)
	writeTestFile(t, rootPath, "paths/local/thing.yaml", `
type: object
`)

	loaderObj := newLoader(rootPath, filepath.Join(rootPath, "openapi.yaml"))
	currentFile := filepath.Join(rootPath, "paths/item.yaml")
	expectedFile := filepath.Join(rootPath, "paths/local/thing.yaml")

	resolvedFile, err := resolveRefPath(loaderObj, currentFile, "local/thing")
	if err != nil {
		t.Fatalf("resolve local fallback ref: %v", err)
	}
	if resolvedFile != expectedFile {
		t.Fatalf("unexpected local fallback target: %s", resolvedFile)
	}
}

func TestInvalidPathsMergeFileFailsClearly(t *testing.T) {
	rootPath := t.TempDir()
	writeTestFile(t, rootPath, "openapi.yaml", `
openapi: 3.0.3
info:
  title: Test API
  version: 1.2.3
`)
	writeTestFile(t, rootPath, "paths/bad.yaml", `
notAPath:
  value: true
`)

	_, err := Load(OptionsObj{Source: rootPath})
	if err == nil || !strings.Contains(err.Error(), "must contain a path item object or explicit /path keys") {
		t.Fatalf("expected clear paths merge error, got %v", err)
	}
}

func TestNestedComponentFileInfersNameFromPath(t *testing.T) {
	rootPath := t.TempDir()
	writeTestFile(t, rootPath, "openapi.yaml", `
openapi: 3.0.3
info:
  title: Test API
  version: 1.2.3
`)
	writeTestFile(t, rootPath, "components/responses/error.yaml", `
description: Error response
content:
  application/json:
    schema:
      type: object
`)
	writeTestFile(t, rootPath, "components/responses/already_resp_obj.yaml", `
description: Already suffixed response
content:
  application/json:
    schema:
      type: object
`)

	graphObj, err := Load(OptionsObj{Source: rootPath})
	if err != nil {
		t.Fatalf("load source: %v", err)
	}

	componentsMap, _ := graphObj.Document["components"].(map[string]any)
	responsesMap, _ := componentsMap["responses"].(map[string]any)
	if _, existsFlag := responsesMap["ErrorRespObj"]; !existsFlag {
		t.Fatalf("expected inferred Error response component, got %v", responsesMap)
	}
	if _, existsFlag := responsesMap["AlreadyRespObj"]; !existsFlag {
		t.Fatalf("expected already suffixed response component, got %v", responsesMap)
	}
}

func TestRecursiveRefFails(t *testing.T) {
	rootPath := t.TempDir()
	writeTestFile(t, rootPath, "openapi.yaml", `
openapi: 3.0.3
info:
  title: Test API
  version: 1.2.3
components:
  schemas:
    A:
      $ref: '#/components/schemas/B'
    B:
      $ref: '#/components/schemas/A'
`)

	_, err := Load(OptionsObj{Source: rootPath})
	if err == nil || !strings.Contains(err.Error(), "recursive $ref detected") {
		t.Fatalf("expected recursive ref error, got %v", err)
	}
}

func writeMinimalSource(t *testing.T, rootPath string) {
	t.Helper()

	writeTestFile(t, rootPath, "openapi.yaml", `
openapi: 3.0.3
info:
  title: Test API
  version: 1.2.3
`)
	writeTestFile(t, rootPath, "paths/health.yaml", `
/health:
  get:
    operationId: healthGet
    responses:
      "200":
        description: OK
`)
}

func writeTestFile(t *testing.T, rootPath string, relPath string, contentText string) {
	t.Helper()

	filePath := filepath.Join(rootPath, relPath)
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("create test directory: %v", err)
	}
	if err := os.WriteFile(filePath, []byte(strings.TrimLeft(contentText, "\n")), 0o644); err != nil {
		t.Fatalf("write test file %s: %v", relPath, err)
	}
}
