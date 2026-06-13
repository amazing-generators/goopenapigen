package goopenapigen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// // // // // // // // // //

func TestRunJSONGeneratePublicJSON(t *testing.T) {
	rootPath := t.TempDir()
	writeRunTestSource(t, rootPath)

	outputDir := filepath.Join(rootPath, "target")
	resultObj, err := Run(ConfigObj{
		Command:   CommandJSON,
		Source:    rootPath,
		OutputDir: outputDir,
		Force:     true,
	})
	if err != nil {
		t.Fatalf("run json generate: %v", err)
	}
	if len(resultObj.GeneratedFilePathArr) != 1 {
		t.Fatalf("unexpected generated files: %v", resultObj.GeneratedFilePathArr)
	}

	outputPath := filepath.Join(outputDir, "openapi.json")
	dataArr, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read public json: %v", err)
	}
	if strings.Contains(string(dataArr), "x-func") {
		t.Fatalf("public json contains x-func")
	}

	documentMap := map[string]any{}
	if err = json.Unmarshal(dataArr, &documentMap); err != nil {
		t.Fatalf("decode public json: %v", err)
	}
	if _, existsFlag := documentMap["tags"]; !existsFlag {
		t.Fatalf("public json does not contain sidecar tags")
	}
}

func TestRunManifestSyncCreate(t *testing.T) {
	rootPath := t.TempDir()
	writeRunTestSource(t, rootPath)

	resultObj, err := Run(ConfigObj{
		Command:        CommandManifestSync,
		Source:         rootPath,
		ManifestCreate: true,
		ManifestFormat: "yaml",
	})
	if err != nil {
		t.Fatalf("run manifest sync: %v", err)
	}
	if len(resultObj.GeneratedFilePathArr) != 1 {
		t.Fatalf("unexpected generated files: %v", resultObj.GeneratedFilePathArr)
	}
	if filepath.Base(resultObj.GeneratedFilePathArr[0]) != "meta.yml" {
		t.Fatalf("manifest was created with unexpected name: %s", resultObj.GeneratedFilePathArr[0])
	}

	dataArr, err := os.ReadFile(resultObj.GeneratedFilePathArr[0])
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if !strings.Contains(string(dataArr), "hash:") {
		t.Fatalf("created manifest has no hash: %s", string(dataArr))
	}
	if strings.Contains(string(dataArr), "name:") {
		t.Fatalf("created project manifest must not contain name: %s", string(dataArr))
	}
}

func TestRunManifestSyncRemovesProjectName(t *testing.T) {
	rootPath := t.TempDir()
	writeRunTestSource(t, rootPath)
	writeRunTestFile(t, rootPath, "meta.yml", `
name: Old API
ver: 1.2.3
hash: old
`)

	_, err := Run(ConfigObj{
		Command: CommandManifestSync,
		Source:  rootPath,
	})
	if err != nil {
		t.Fatalf("run manifest sync: %v", err)
	}

	dataArr, err := os.ReadFile(filepath.Join(rootPath, "meta.yml"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if strings.Contains(string(dataArr), "name:") {
		t.Fatalf("updated project manifest must not contain name: %s", string(dataArr))
	}
	if !strings.Contains(string(dataArr), "ver: 1.2.3") {
		t.Fatalf("updated project manifest lost version: %s", string(dataArr))
	}
	if !strings.Contains(string(dataArr), "hash:") {
		t.Fatalf("updated project manifest has no hash: %s", string(dataArr))
	}
}

func TestRunMetaGenerate(t *testing.T) {
	rootPath := t.TempDir()
	writeRunTestSource(t, rootPath)

	outputDir := filepath.Join(rootPath, "target")
	_, err := Run(ConfigObj{
		Command:   CommandMeta,
		Source:    rootPath,
		OutputDir: outputDir,
		Force:     true,
	})
	if err != nil {
		t.Fatalf("run meta generate: %v", err)
	}

	outputPath := filepath.Join(outputDir, "openapi_meta_gen.go")
	dataArr, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read meta output: %v", err)
	}
	if !strings.Contains(string(dataArr), `Version      string = "1.2.3"`) {
		t.Fatalf("unexpected meta output:\n%s", string(dataArr))
	}
}

func TestRunGenerateMetadataFile(t *testing.T) {
	rootPath := t.TempDir()
	writeRunTestSource(t, rootPath)

	outputDir := filepath.Join(rootPath, "target", "api")
	_, err := Run(ConfigObj{
		Command:              CommandGenerate,
		Source:               rootPath,
		OutputDir:            outputDir,
		DisableOgen:          true,
		DisableOpenAPIJSONGo: true,
		Force:                true,
	})
	if err != nil {
		t.Fatalf("run generate with metadata output: %v", err)
	}

	outputPath := filepath.Join(outputDir, "openapi_meta_gen.go")
	dataArr, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read generated metadata output: %v", err)
	}
	outputText := string(dataArr)
	for _, expectedText := range []string{
		"package api",
		`Name string = "Test API"`,
		`Version      string = "1.2.3"`,
	} {
		if !strings.Contains(outputText, expectedText) {
			t.Fatalf("generated metadata output does not contain %q:\n%s", expectedText, outputText)
		}
	}
}

func TestRunGenerateRequireXFuncPreflightLeavesNoOgenOutput(t *testing.T) {
	rootPath := t.TempDir()
	writeRunTestFile(t, rootPath, "openapi.yaml", `
openapi: 3.0.3
info:
  title: Test API
  version: 1.2.3
paths:
  /test:
    get:
      operationId: testGet
      responses:
        "200":
          description: OK
`)

	outputDir := filepath.Join(rootPath, "target", "api")
	_, err := Run(ConfigObj{
		Command:      CommandGenerate,
		Source:       rootPath,
		OutputDir:    outputDir,
		RequireXFunc: true,
		Force:        true,
	})
	if err == nil || !strings.Contains(err.Error(), "missing x-func mappings") {
		t.Fatalf("expected missing x-func error, got %v", err)
	}

	entryArr, readErr := os.ReadDir(outputDir)
	if readErr != nil {
		t.Fatalf("read output directory: %v", readErr)
	}
	if len(entryArr) != 0 {
		t.Fatalf("strict preflight left generated files: %v", entryArr)
	}
}

func TestResolveGenerateSelectionHTTPDefaults(t *testing.T) {
	selectionObj := resolveGenerateSelection(ConfigObj{})
	if !selectionObj.HTTPDefaults {
		t.Fatalf("HTTP defaults must be enabled by default")
	}

	selectionObj = resolveGenerateSelection(ConfigObj{DisableHTTPDefaults: true})
	if selectionObj.HTTPDefaults {
		t.Fatalf("HTTP defaults must be disabled by explicit config")
	}

	selectionObj = resolveGenerateSelection(ConfigObj{DisableRouter: true})
	if selectionObj.HTTPDefaults {
		t.Fatalf("HTTP defaults must be disabled with router")
	}

	selectionObj = resolveGenerateSelection(ConfigObj{DisableOgen: true})
	if selectionObj.Router || selectionObj.HTTPDefaults {
		t.Fatalf("router and HTTP defaults must be disabled with ogen")
	}
}

func writeRunTestSource(t *testing.T, rootPath string) {
	t.Helper()

	writeRunTestFile(t, rootPath, "openapi.yaml", `
openapi: 3.0.3
info:
  title: Test API
  version: 1.2.3
`)
	writeRunTestFile(t, rootPath, "tags.yml", `
- name: test
`)
	writeRunTestFile(t, rootPath, "paths/test.yml", `
/test:
  get:
    operationId: testGet
    x-func: TestGet
    responses:
      "200":
        description: OK
`)
}

func writeRunTestFile(t *testing.T, rootPath string, relPath string, contentText string) {
	t.Helper()

	filePath := filepath.Join(rootPath, relPath)
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("create test directory: %v", err)
	}
	if err := os.WriteFile(filePath, []byte(strings.TrimLeft(contentText, "\n")), 0o644); err != nil {
		t.Fatalf("write test file %s: %v", relPath, err)
	}
}
