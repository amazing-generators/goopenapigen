package goopenapigen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/amazing-generators/goopenapigen/internal/bridge"
	"github.com/amazing-generators/goopenapigen/internal/goident"
	"github.com/amazing-generators/goopenapigen/internal/meta"
	"github.com/amazing-generators/goopenapigen/internal/source"
	"github.com/amazing-generators/goopenapigen/internal/write"
)

// // // // // // // // // //

const (
	cPublicOpenAPIJSONFileName = "openapi.json"
	cRouterGoFileName          = "router_gen.go"
	cHTTPDefaultsGoFileName    = "http_defaults_gen.go"
	cOpenAPIJSONGoFileName     = "openapi_json_gen.go"
	cOpenAPIMetaGoFileName     = "openapi_meta_gen.go"
)

type generateSelectionObj struct {
	Ogen          bool
	Router        bool
	HTTPDefaults  bool
	OpenAPIJSONGo bool
	MetaGo        bool
}

// //

// Run is the shared entry point; config.Command selects the concrete pipeline.
func Run(config ConfigObj) (*ResultObj, error) {
	switch config.Command {
	case CommandGenerate:
		return runGenerate(config)
	case CommandJSON:
		return runJSON(config)
	case CommandManifestSync:
		return runManifestSync(config)
	case CommandMeta:
		return runMeta(config)
	default:
		return nil, fmt.Errorf("unknown command: %q", config.Command)
	}
}

// //

func runGenerate(config ConfigObj) (*ResultObj, error) {
	selectionObj := resolveGenerateSelection(config)
	if !hasGenerateOutput(selectionObj) {
		return nil, fmt.Errorf("no generation output selected")
	}

	outputDir, err := absOutputDir(config.OutputDir)
	if err != nil {
		return nil, err
	}
	packageName, err := resolvePackageName(config.PackageName, outputDir)
	if err != nil {
		return nil, err
	}
	if err = write.PrepareDir(outputDir, config.Force); err != nil {
		return nil, err
	}

	graphObj, err := source.Load(source.OptionsObj{
		Context:     config.Context,
		Source:      config.Source,
		MaxRefDepth: config.MaxRefDepth,
	})
	if err != nil {
		return nil, err
	}

	versionObj, err := resolveEffectiveVersion(config, graphObj)
	if err != nil {
		return nil, err
	}

	resultObj := &ResultObj{
		Command:    config.Command,
		WarningArr: append([]string{}, versionObj.WarningTextArr...),
	}

	if selectionObj.Router {
		if err = bridge.ValidateMappings(graphObj.Document, config.RequireXFunc); err != nil {
			return nil, err
		}
	}

	publicJSONArr, err := buildPublicJSONIfNeeded(config, selectionObj, graphObj, versionObj)
	if err != nil {
		return nil, err
	}

	if selectionObj.Ogen {
		ogenInputPath, cleanupFunc, err := writeTempJSON(publicJSONArr)
		if err != nil {
			return nil, err
		}
		defer cleanupFunc()

		if err = bridge.RunOgen(bridge.OgenConfigObj{
			Context:     config.Context,
			Command:     config.OgenCommand,
			InputPath:   ogenInputPath,
			TargetDir:   outputDir,
			PackageName: packageName,
			Timeout:     config.OgenTimeout,
			Force:       config.Force,
		}); err != nil {
			return nil, err
		}

		resultObj.GeneratedFilePathArr = append(resultObj.GeneratedFilePathArr, outputDir)
	}

	keepNameMap := make(map[string]struct{})
	if selectionObj.Router {
		outputPath := filepath.Join(outputDir, cRouterGoFileName)
		routerDataArr, err := bridge.Generate(bridge.ConfigObj{
			OgenDir:      outputDir,
			PackageName:  packageName,
			Document:     graphObj.Document,
			RequireXFunc: config.RequireXFunc,
			Comments:     !config.DisableComments,
		})
		if err != nil {
			return nil, err
		}

		if err = write.File(outputPath, routerDataArr, config.Force); err != nil {
			return nil, err
		}

		keepNameMap[filepath.Base(outputPath)] = struct{}{}
		resultObj.GeneratedFilePathArr = append(resultObj.GeneratedFilePathArr, outputPath)
	}

	if selectionObj.OpenAPIJSONGo {
		outputPath := filepath.Join(outputDir, cOpenAPIJSONGoFileName)
		openAPIJSONDataArr, err := bridge.GenerateOpenAPIJSON(bridge.ConfigObj{
			PackageName: packageName,
			OpenAPIJSON: publicJSONArr,
		})
		if err != nil {
			return nil, err
		}
		if err = write.File(outputPath, openAPIJSONDataArr, config.Force); err != nil {
			return nil, err
		}

		keepNameMap[filepath.Base(outputPath)] = struct{}{}
		resultObj.GeneratedFilePathArr = append(resultObj.GeneratedFilePathArr, outputPath)
	}

	if selectionObj.HTTPDefaults {
		outputPath := filepath.Join(outputDir, cHTTPDefaultsGoFileName)
		httpDefaultsDataArr, err := bridge.GenerateHTTPDefaults(bridge.ConfigObj{
			OgenDir:        outputDir,
			PackageName:    packageName,
			Document:       graphObj.Document,
			RequireXFunc:   config.RequireXFunc,
			HasOpenAPIJSON: selectionObj.OpenAPIJSONGo,
		})
		if err != nil {
			return nil, err
		}
		if err = write.File(outputPath, httpDefaultsDataArr, config.Force); err != nil {
			return nil, err
		}

		keepNameMap[filepath.Base(outputPath)] = struct{}{}
		resultObj.GeneratedFilePathArr = append(resultObj.GeneratedFilePathArr, outputPath)
	}

	if selectionObj.MetaGo {
		outputPath := filepath.Join(outputDir, cOpenAPIMetaGoFileName)
		dataArr, err := renderMetaGo(packageName, versionObj)
		if err != nil {
			return nil, err
		}
		if err = write.File(outputPath, dataArr, config.Force); err != nil {
			return nil, err
		}

		keepNameMap[filepath.Base(outputPath)] = struct{}{}
		resultObj.GeneratedFilePathArr = append(resultObj.GeneratedFilePathArr, outputPath)
	}

	if err = write.CleanupStale(outputDir, keepNameMap, config.Force); err != nil {
		return nil, err
	}

	return resultObj, nil
}

func runJSON(config ConfigObj) (*ResultObj, error) {
	outputDir, err := absOutputDir(config.OutputDir)
	if err != nil {
		return nil, err
	}

	graphObj, err := source.Load(source.OptionsObj{
		Context:     config.Context,
		Source:      config.Source,
		MaxRefDepth: config.MaxRefDepth,
	})
	if err != nil {
		return nil, err
	}

	versionObj, err := resolveEffectiveVersion(config, graphObj)
	if err != nil {
		return nil, err
	}

	dataArr, err := buildPublicJSONWithOptions(graphObj, versionObj, publicOptionsFromConfig(config))
	if err != nil {
		return nil, err
	}

	outputPath := filepath.Join(outputDir, cPublicOpenAPIJSONFileName)
	if err = write.File(outputPath, dataArr, config.Force); err != nil {
		return nil, err
	}

	return &ResultObj{
		Command:              config.Command,
		GeneratedFilePathArr: []string{outputPath},
		WarningArr:           append([]string{}, versionObj.WarningTextArr...),
	}, nil
}

func runManifestSync(config ConfigObj) (*ResultObj, error) {
	if err := ensureManifestSyncSourceDir(config.Source); err != nil {
		return nil, err
	}

	graphObj, err := source.Load(source.OptionsObj{
		Context:     config.Context,
		Source:      config.Source,
		MaxRefDepth: config.MaxRefDepth,
	})
	if err != nil {
		return nil, err
	}

	manifestPath, err := syncManifest(config, graphObj)
	if err != nil {
		return nil, err
	}

	return &ResultObj{
		Command:              config.Command,
		GeneratedFilePathArr: []string{manifestPath},
	}, nil
}

func runMeta(config ConfigObj) (*ResultObj, error) {
	outputDir, err := absOutputDir(config.OutputDir)
	if err != nil {
		return nil, err
	}
	packageName, err := resolvePackageName(config.PackageName, outputDir)
	if err != nil {
		return nil, err
	}

	graphObj, err := source.Load(source.OptionsObj{
		Context:     config.Context,
		Source:      config.Source,
		MaxRefDepth: config.MaxRefDepth,
	})
	if err != nil {
		return nil, err
	}

	versionObj, err := resolveEffectiveVersion(config, graphObj)
	if err != nil {
		return nil, err
	}

	dataArr, err := renderMetaGo(packageName, versionObj)
	if err != nil {
		return nil, err
	}

	outputPath := filepath.Join(outputDir, cOpenAPIMetaGoFileName)
	if err = write.File(outputPath, dataArr, config.Force); err != nil {
		return nil, err
	}

	return &ResultObj{
		Command:              config.Command,
		GeneratedFilePathArr: []string{outputPath},
		WarningArr:           append([]string{}, versionObj.WarningTextArr...),
	}, nil
}

func resolveGenerateSelection(config ConfigObj) generateSelectionObj {
	selectionObj := generateSelectionObj{
		Ogen:          !config.DisableOgen,
		Router:        !config.DisableRouter,
		HTTPDefaults:  !config.DisableHTTPDefaults,
		OpenAPIJSONGo: !config.DisableOpenAPIJSONGo,
		MetaGo:        !config.DisableMetaGo,
	}
	if !selectionObj.Ogen {
		selectionObj.Router = false
	}
	if !selectionObj.Router {
		selectionObj.HTTPDefaults = false
	}

	return selectionObj
}

func hasGenerateOutput(selectionObj generateSelectionObj) bool {
	return selectionObj.Ogen ||
		selectionObj.Router ||
		selectionObj.HTTPDefaults ||
		selectionObj.OpenAPIJSONGo ||
		selectionObj.MetaGo
}

func buildPublicJSONIfNeeded(config ConfigObj, selectionObj generateSelectionObj, graphObj *source.GraphObj, versionObj *effectiveVersionObj) ([]byte, error) {
	if !selectionObj.Ogen && !selectionObj.OpenAPIJSONGo {
		return nil, nil
	}

	return buildPublicJSONWithOptions(graphObj, versionObj, publicOptionsFromConfig(config))
}

func buildPublicJSON(graphObj *source.GraphObj, versionObj *effectiveVersionObj) ([]byte, error) {
	return buildPublicJSONWithOptions(graphObj, versionObj, source.PublicOptionsObj{
		CanonicalComponentRefs: true,
	})
}

func buildPublicJSONWithOptions(graphObj *source.GraphObj, versionObj *effectiveVersionObj, optionsObj source.PublicOptionsObj) ([]byte, error) {
	documentMap, err := graphObj.PublicDocumentWithOptions(versionObj.Version, optionsObj)
	if err != nil {
		return nil, err
	}

	dataArr, err := source.RenderJSON(documentMap)
	if err != nil {
		return nil, err
	}

	return dataArr, nil
}

func publicOptionsFromConfig(config ConfigObj) source.PublicOptionsObj {
	return source.PublicOptionsObj{
		CanonicalComponentRefs: !config.DisableCanonicalComponentRefs,
	}
}

func renderMetaGo(packageName string, versionObj *effectiveVersionObj) ([]byte, error) {
	return meta.GenerateGo(meta.GoConfigObj{
		PackageName:  packageName,
		Name:         versionObj.Name,
		Version:      versionObj.Version,
		VersionMajor: versionObj.VersionMajor,
		VersionMinor: versionObj.VersionMinor,
		VersionPatch: versionObj.VersionPatch,
		Hash:         versionObj.Hash,
		DateUpdate:   versionObj.DateUpdate,
	})
}

func writeTempJSON(dataArr []byte) (string, func(), error) {
	tempFile, err := os.CreateTemp("", "goopenapigen-openapi-*.json")
	if err != nil {
		return "", nil, fmt.Errorf("create temporary OpenAPI JSON: %w", err)
	}

	tempPath := tempFile.Name()
	cleanupFunc := func() {
		_ = os.Remove(tempPath)
	}

	if _, err = tempFile.Write(dataArr); err != nil {
		_ = tempFile.Close()
		cleanupFunc()
		return "", nil, fmt.Errorf("write temporary OpenAPI JSON: %w", err)
	}
	if err = tempFile.Close(); err != nil {
		cleanupFunc()
		return "", nil, fmt.Errorf("close temporary OpenAPI JSON: %w", err)
	}

	return tempPath, cleanupFunc, nil
}

func absOutputDir(rawPath string) (string, error) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return "", fmt.Errorf("output directory is empty")
	}

	absPath, err := filepath.Abs(rawPath)
	if err != nil {
		return "", fmt.Errorf("resolve output directory: %w", err)
	}

	return filepath.Clean(absPath), nil
}

func resolvePackageName(rawPackageName string, outputDir string) (string, error) {
	packageName := strings.TrimSpace(rawPackageName)
	if packageName == "" {
		packageName = filepath.Base(outputDir)
	}
	if !goident.IsPackageName(packageName) {
		return "", fmt.Errorf("invalid Go package name: %s", packageName)
	}
	return packageName, nil
}

func ensureManifestSyncSourceDir(rawSource string) error {
	sourcePath := strings.TrimSpace(rawSource)
	if sourcePath == "" {
		sourcePath = "."
	}

	infoObj, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}
	if !infoObj.IsDir() {
		return fmt.Errorf("manifest sync source must be a directory")
	}

	return nil
}
