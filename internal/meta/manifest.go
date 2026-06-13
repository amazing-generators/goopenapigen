package meta

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// // // // // // // // // //

const (
	cManifestJSON        = "values.json"
	cManifestYML         = "values.yml"
	cManifestYAML        = "values.yaml"
	cProjectManifestJSON = "meta.json"
	cProjectManifestYML  = "meta.yml"
	cProjectManifestYAML = "meta.yaml"
)

// ValuesObj is shared by project meta.* files and self-version values.* files.
type ValuesObj struct {
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	Ver  string `json:"ver" yaml:"ver"`
	Hash string `json:"hash,omitempty" yaml:"hash,omitempty"`
}

// kindObj describes the two manifest kinds with one definition instead of duplicated
// logic: self-version (values.*, with Name, also searched in _run/) and project
// (meta.*, without Name).
type kindObj struct {
	jsonName  string
	ymlName   string
	yamlName  string
	stripName bool
	useRunDir bool
}

var selfKind = kindObj{cManifestJSON, cManifestYML, cManifestYAML, false, true}
var projectKind = kindObj{cProjectManifestJSON, cProjectManifestYML, cProjectManifestYAML, true, false}

// //

func (kind kindObj) fileNameArr() []string {
	return []string{kind.jsonName, kind.ymlName, kind.yamlName}
}

func (kind kindObj) defaultFileName(format string) string {
	if format == "yaml" {
		return kind.ymlName
	}

	return kind.jsonName
}

func cloneValues(valuesObj *ValuesObj) *ValuesObj {
	if valuesObj == nil {
		return nil
	}

	copyObj := *valuesObj
	return &copyObj
}

func normalizeValues(valuesObj *ValuesObj, stripName bool) ValuesObj {
	normalizedObj := ValuesObj{
		Name: strings.TrimSpace(valuesObj.Name),
		Ver:  strings.TrimSpace(valuesObj.Ver),
		Hash: strings.TrimSpace(valuesObj.Hash),
	}
	if stripName {
		normalizedObj.Name = ""
	}

	return normalizedObj
}

// //

func readManifestFile(filePath string, stripName bool) (*ValuesObj, error) {
	dataArr, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filePath, err)
	}

	valuesObj := ValuesObj{}

	switch filepath.Ext(filePath) {
	case ".json":
		if err = json.Unmarshal(dataArr, &valuesObj); err != nil {
			return nil, fmt.Errorf("decode json manifest: %w", err)
		}
	case ".yml", ".yaml":
		if err = yaml.Unmarshal(dataArr, &valuesObj); err != nil {
			return nil, fmt.Errorf("decode yaml manifest: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported manifest format: %s", filePath)
	}

	normalizedObj := normalizeValues(&valuesObj, stripName)
	if err = validateValues(normalizedObj, filePath, stripName); err != nil {
		return nil, err
	}

	return &normalizedObj, nil
}

func writeManifestFile(filePath string, valuesObj *ValuesObj, force bool, stripName bool) error {
	if valuesObj == nil {
		return fmt.Errorf("manifest value is nil")
	}

	normalizedObj := normalizeValues(valuesObj, stripName)
	if err := validateValues(normalizedObj, "", stripName); err != nil {
		return err
	}

	outputDir := filepath.Dir(filePath)
	if err := ensureDir(outputDir, force); err != nil {
		return err
	}

	dataArr, err := marshalManifest(&normalizedObj, filePath)
	if err != nil {
		return err
	}

	if existingArr, readErr := os.ReadFile(filePath); readErr == nil && bytes.Equal(existingArr, dataArr) {
		return nil
	}

	tempFile, err := os.CreateTemp(outputDir, ".goopenapigen-manifest-*")
	if err != nil {
		return fmt.Errorf("create temp manifest: %w", err)
	}

	tempPath := tempFile.Name()
	removeTempFlag := true

	defer func() {
		_ = tempFile.Close()
		if removeTempFlag {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err = tempFile.Write(dataArr); err != nil {
		return fmt.Errorf("write temp manifest: %w", err)
	}

	if err = tempFile.Close(); err != nil {
		return fmt.Errorf("close temp manifest: %w", err)
	}

	if err = os.Rename(tempPath, filePath); err != nil {
		return fmt.Errorf("replace manifest file: %w", err)
	}

	removeTempFlag = false
	return nil
}

// //

// find returns a manifest of the given kind next to the source or at an explicit path.
func find(sourceRoot string, explicitPath string, kind kindObj) (*Obj, bool, error) {
	if strings.TrimSpace(explicitPath) != "" {
		return findExplicit(sourceRoot, explicitPath, kind)
	}

	manifestPath, existsFlag, err := findManifest(sourceRoot, kind)
	if err != nil {
		return nil, false, err
	}
	if !existsFlag {
		return nil, false, nil
	}

	return &Obj{sourcePath: sourceRoot, manifestPath: manifestPath, kind: kind}, true, nil
}

func findExplicit(sourceRoot string, explicitPath string, kind kindObj) (*Obj, bool, error) {
	absPath, err := resolveInputPath(explicitPath)
	if err != nil {
		return nil, false, err
	}

	infoObj, statErr := os.Stat(absPath)
	if statErr == nil {
		if infoObj.IsDir() {
			manifestPath, existsFlag, err := findManifest(absPath, kind)
			if err != nil {
				return nil, false, err
			}
			if !existsFlag {
				return nil, false, fmt.Errorf("manifest file not found in %s", absPath)
			}

			return &Obj{sourcePath: absPath, manifestPath: manifestPath, kind: kind}, true, nil
		}
		if !isManifestExtension(filepath.Ext(absPath)) {
			return nil, false, fmt.Errorf("manifest file must use .json, .yml, or .yaml: %s", absPath)
		}

		return &Obj{sourcePath: filepath.Dir(absPath), manifestPath: absPath, kind: kind}, true, nil
	}
	if !errors.Is(statErr, os.ErrNotExist) {
		return nil, false, fmt.Errorf("stat manifest path: %w", statErr)
	}
	if !isManifestExtension(filepath.Ext(absPath)) {
		return nil, false, fmt.Errorf("manifest file must use .json, .yml, or .yaml: %s", absPath)
	}

	if strings.TrimSpace(sourceRoot) == "" {
		sourceRoot = filepath.Dir(absPath)
	}

	return &Obj{sourcePath: sourceRoot, manifestPath: absPath, kind: kind}, false, nil
}

// findManifest looks for a single manifest file in the directory (and in _run/ for self-version).
func findManifest(dirPath string, kind kindObj) (string, bool, error) {
	searchDirArr := []string{dirPath}
	if kind.useRunDir {
		if runDir := resolveRunDir(dirPath); runDir != filepath.Clean(dirPath) {
			searchDirArr = append(searchDirArr, runDir)
		}
	}

	foundArr := make([]string, 0, len(searchDirArr)*3)
	seenMap := make(map[string]struct{}, len(searchDirArr)*3)

	for _, baseDir := range searchDirArr {
		for _, fileName := range kind.fileNameArr() {
			filePath := filepath.Join(baseDir, fileName)
			if _, existsFlag := seenMap[filePath]; existsFlag {
				continue
			}
			seenMap[filePath] = struct{}{}

			infoObj, err := os.Stat(filePath)
			if err == nil {
				if infoObj.IsDir() {
					return "", false, fmt.Errorf("manifest path is a directory: %s", filePath)
				}

				foundArr = append(foundArr, filePath)
				continue
			}
			if !errors.Is(err, os.ErrNotExist) {
				return "", false, fmt.Errorf("stat manifest path %s: %w", filePath, err)
			}
		}
	}

	switch len(foundArr) {
	case 1:
		return foundArr[0], true, nil
	case 0:
		return "", false, nil
	default:
		return "", false, fmt.Errorf("multiple manifest files found in %s (use -meta)", dirPath)
	}
}

func FindProject(sourceRoot string, explicitPath string) (*Obj, bool, error) {
	return find(sourceRoot, explicitPath, projectKind)
}

func CreateProjectManifest(sourceRoot string, explicitPath string, format string, valuesObj *ValuesObj, force bool) (string, error) {
	manifestPath, err := resolveWritePath(sourceRoot, explicitPath, format, projectKind)
	if err != nil {
		return "", err
	}
	if err = writeManifestFile(manifestPath, valuesObj, force, projectKind.stripName); err != nil {
		return "", err
	}

	return manifestPath, nil
}

func (obj *Obj) Write(valuesObj *ValuesObj, force bool) error {
	if err := writeManifestFile(obj.manifestPath, valuesObj, force, obj.kind.stripName); err != nil {
		return err
	}

	normalizedObj := normalizeValues(valuesObj, obj.kind.stripName)

	obj.manifestObj = cloneValues(&normalizedObj)
	obj.manifestErr = nil
	obj.loadedFlag = true

	return nil
}

func (obj *Obj) Stat() (os.FileInfo, error) {
	infoObj, err := os.Stat(obj.manifestPath)
	if err != nil {
		return nil, fmt.Errorf("stat manifest file: %w", err)
	}

	return infoObj, nil
}

// //

// resolveWritePath picks the write path: a directory -> the default file name (honoring
// _run), otherwise the path itself with an extension check.
func resolveWritePath(sourceRoot string, targetPath string, format string, kind kindObj) (string, error) {
	manifestFormat, err := normalizeManifestFormat(format)
	if err != nil {
		return "", err
	}

	// Empty explicit path: write into the source with the default name (no _run subdir).
	if strings.TrimSpace(targetPath) == "" {
		return filepath.Join(sourceRoot, kind.defaultFileName(manifestFormat)), nil
	}

	absPath, err := resolveInputPath(targetPath)
	if err != nil {
		return "", err
	}

	defaultDir := func(dirPath string) string {
		if kind.useRunDir {
			return resolveRunDir(dirPath)
		}

		return dirPath
	}

	infoObj, statErr := os.Stat(absPath)
	switch {
	case statErr == nil && infoObj.IsDir():
		return filepath.Join(defaultDir(absPath), kind.defaultFileName(manifestFormat)), nil
	case statErr == nil:
		if !isManifestExtension(filepath.Ext(absPath)) {
			return "", fmt.Errorf("manifest file must use .json, .yml, or .yaml: %s", absPath)
		}

		return absPath, nil
	case !errors.Is(statErr, os.ErrNotExist):
		return "", fmt.Errorf("stat manifest output path: %w", statErr)
	}

	if isManifestExtension(filepath.Ext(absPath)) {
		return absPath, nil
	}
	if filepath.Ext(absPath) != "" {
		return "", fmt.Errorf("manifest file must use .json, .yml, or .yaml: %s", absPath)
	}

	return filepath.Join(defaultDir(absPath), kind.defaultFileName(manifestFormat)), nil
}

func validateValues(valuesObj ValuesObj, filePath string, stripName bool) error {
	valuesObj.Name = strings.TrimSpace(valuesObj.Name)
	valuesObj.Ver = strings.TrimSpace(valuesObj.Ver)

	if valuesObj.Name == "" && !stripName {
		if filePath == "" {
			return fmt.Errorf("manifest name is empty")
		}

		return fmt.Errorf("manifest name is empty: %s", filePath)
	}

	if valuesObj.Ver == "" {
		if filePath == "" {
			return fmt.Errorf("manifest ver is empty")
		}

		return fmt.Errorf("manifest ver is empty: %s", filePath)
	}

	if err := ValidateVersion(valuesObj.Ver); err != nil {
		if filePath == "" {
			return err
		}

		return fmt.Errorf("%w: %s", err, filePath)
	}

	return nil
}

func marshalManifest(valuesObj *ValuesObj, filePath string) ([]byte, error) {
	normalizedObj := ValuesObj{
		Name: strings.TrimSpace(valuesObj.Name),
		Ver:  strings.TrimSpace(valuesObj.Ver),
		Hash: strings.TrimSpace(valuesObj.Hash),
	}

	var (
		dataArr []byte
		err     error
	)

	switch filepath.Ext(filePath) {
	case ".json":
		dataArr, err = json.MarshalIndent(normalizedObj, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("encode json manifest: %w", err)
		}
	case ".yml", ".yaml":
		dataArr, err = yaml.Marshal(normalizedObj)
		if err != nil {
			return nil, fmt.Errorf("encode yaml manifest: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported manifest format: %s", filePath)
	}

	return append(dataArr, '\n'), nil
}

// //

func resolveInputPath(rawPath string) (string, error) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("read working directory: %w", err)
		}

		return cwd, nil
	}

	absPath, err := filepath.Abs(rawPath)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}

	return absPath, nil
}

func ensureDir(dirPath string, force bool) error {
	if force {
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			return fmt.Errorf("create directory: %w", err)
		}

		return nil
	}

	info, err := os.Stat(dirPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("directory does not exist: %s (use -force to create it)", dirPath)
		}

		return fmt.Errorf("stat directory: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", dirPath)
	}

	return nil
}

func normalizeManifestFormat(raw string) (string, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	switch raw {
	case "", "json":
		return "json", nil
	case "yml", "yaml":
		return "yaml", nil
	default:
		return "", fmt.Errorf("unsupported manifest format: %s", raw)
	}
}

// resolveRunDir hides the self-version manifest in the _run subdir unless the dir is already _run.
func resolveRunDir(dirPath string) string {
	cleanPath := filepath.Clean(dirPath)
	if filepath.Base(cleanPath) == "_run" {
		return cleanPath
	}

	return filepath.Join(cleanPath, "_run")
}

func isManifestExtension(ext string) bool {
	switch strings.ToLower(ext) {
	case ".json", ".yml", ".yaml":
		return true
	default:
		return false
	}
}
