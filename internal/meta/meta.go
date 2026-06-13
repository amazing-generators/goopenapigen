package meta

import (
	"fmt"
	"os"
	"path/filepath"
)

// // // // // // // // // //

type Obj struct {
	sourcePath   string
	manifestPath string
	kind         kindObj
	manifestObj  *ValuesObj
	manifestErr  error
	loadedFlag   bool
}

// //

// New opens a self-version manifest (values.*) by file or directory.
func New(sourcePath string) (*Obj, error) {
	absPath, err := resolveInputPath(sourcePath)
	if err != nil {
		return nil, err
	}

	infoObj, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("stat manifest source: %w", err)
	}
	if infoObj.IsDir() {
		manifestPath, existsFlag, err := findManifest(absPath, selfKind)
		if err != nil {
			return nil, err
		}
		if !existsFlag {
			return nil, fmt.Errorf("manifest file not found in %s", absPath)
		}

		return &Obj{sourcePath: absPath, manifestPath: manifestPath, kind: selfKind}, nil
	}

	if !isManifestExtension(filepath.Ext(absPath)) {
		return nil, fmt.Errorf("manifest file must use .json, .yml, or .yaml: %s", absPath)
	}

	return &Obj{sourcePath: filepath.Dir(absPath), manifestPath: absPath, kind: selfKind}, nil
}

// NewProject opens a project manifest (meta.*) by file or directory.
func NewProject(sourcePath string) (*Obj, error) {
	absPath, err := resolveInputPath(sourcePath)
	if err != nil {
		return nil, err
	}

	infoObj, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("stat manifest source: %w", err)
	}
	if !infoObj.IsDir() {
		if !isManifestExtension(filepath.Ext(absPath)) {
			return nil, fmt.Errorf("manifest file must use .json, .yml, or .yaml: %s", absPath)
		}

		return &Obj{sourcePath: filepath.Dir(absPath), manifestPath: absPath, kind: projectKind}, nil
	}

	manifestObj, existsFlag, err := FindProject(absPath, "")
	if err != nil {
		return nil, err
	}
	if !existsFlag {
		return nil, fmt.Errorf("project manifest file not found in %s", absPath)
	}

	return manifestObj, nil
}

func (obj *Obj) SourcePath() string {
	return obj.sourcePath
}

func (obj *Obj) ManifestPath() string {
	return obj.manifestPath
}

func (obj *Obj) Manifest() (*ValuesObj, error) {
	if !obj.loadedFlag {
		obj.manifestObj, obj.manifestErr = readManifestFile(obj.manifestPath, obj.kind.stripName)
		obj.loadedFlag = true
	}

	if obj.manifestErr != nil {
		return nil, obj.manifestErr
	}

	return cloneValues(obj.manifestObj), nil
}

func (obj *Obj) Validate() error {
	_, err := obj.Manifest()
	return err
}

func (obj *Obj) Name() (string, error) {
	valuesObj, err := obj.Manifest()
	if err != nil {
		return "", err
	}

	if err = validateNameLen(valuesObj.Name); err != nil {
		return "", err
	}

	return valuesObj.Name, nil
}

func (obj *Obj) Version() (*VersionObj, error) {
	valuesObj, err := obj.Manifest()
	if err != nil {
		return nil, err
	}

	return ParseVersion(valuesObj.Ver)
}

func (obj *Obj) VersionString() (string, error) {
	versionObj, err := obj.Version()
	if err != nil {
		return "", err
	}

	return versionObj.Raw, nil
}

func (obj *Obj) IncrementMinor() (string, error) {
	versionObj, err := obj.Version()
	if err != nil {
		return "", err
	}

	if err = versionObj.IncrementMinor(); err != nil {
		return "", err
	}

	return obj.writeVersion(versionObj)
}

func (obj *Obj) IncrementPatch() (string, error) {
	versionObj, err := obj.Version()
	if err != nil {
		return "", err
	}

	if err = versionObj.IncrementPatch(); err != nil {
		return "", err
	}

	return obj.writeVersion(versionObj)
}

func (obj *Obj) IncrementMajor() (string, error) {
	versionObj, err := obj.Version()
	if err != nil {
		return "", err
	}

	if err = versionObj.IncrementMajor(); err != nil {
		return "", err
	}

	return obj.writeVersion(versionObj)
}

func (obj *Obj) PreMajor(preID string) (string, error) {
	versionObj, err := obj.Version()
	if err != nil {
		return "", err
	}

	if err = versionObj.PreMajor(preID); err != nil {
		return "", err
	}

	return obj.writeVersion(versionObj)
}

func (obj *Obj) PreMinor(preID string) (string, error) {
	versionObj, err := obj.Version()
	if err != nil {
		return "", err
	}

	if err = versionObj.PreMinor(preID); err != nil {
		return "", err
	}

	return obj.writeVersion(versionObj)
}

func (obj *Obj) PrePatch(preID string) (string, error) {
	versionObj, err := obj.Version()
	if err != nil {
		return "", err
	}

	if err = versionObj.PrePatch(preID); err != nil {
		return "", err
	}

	return obj.writeVersion(versionObj)
}

func (obj *Obj) Prerelease(preID string) (string, error) {
	versionObj, err := obj.Version()
	if err != nil {
		return "", err
	}

	if err = versionObj.IncrementPrerelease(preID); err != nil {
		return "", err
	}

	return obj.writeVersion(versionObj)
}

// writeVersion stores the new version without losing other allowed fields.
func (obj *Obj) writeVersion(versionObj *VersionObj) (string, error) {
	valuesObj, err := obj.Manifest()
	if err != nil {
		return "", err
	}

	versionObj.Raw = versionObj.String()
	if err = ValidateVersion(versionObj.Raw); err != nil {
		return "", err
	}

	valuesObj.Ver = versionObj.Raw

	if err = writeManifestFile(obj.manifestPath, valuesObj, true, obj.kind.stripName); err != nil {
		return "", err
	}

	normalizedObj := normalizeValues(valuesObj, obj.kind.stripName)

	obj.manifestObj = cloneValues(&normalizedObj)
	obj.manifestErr = nil
	obj.loadedFlag = true

	return versionObj.Raw, nil
}

func validateNameLen(value string) error {
	if len([]rune(value)) > 40 {
		return fmt.Errorf("manifest name exceeds 40 characters")
	}

	return nil
}
