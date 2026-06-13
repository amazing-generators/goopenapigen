package goopenapigen

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/amazing-generators/goopenapigen/internal/meta"
	"github.com/amazing-generators/goopenapigen/internal/source"
)

// // // // // // // // // //

type effectiveVersionObj struct {
	Name           string
	Version        string
	VersionMajor   uint16
	VersionMinor   uint16
	VersionPatch   uint16
	Hash           string
	DateUpdate     time.Time
	ManifestObj    *meta.Obj
	WarningTextArr []string
}

type openAPIInfoObj struct {
	Title   string
	Version string
}

type projectVersionObj struct {
	Raw   string
	Major uint16
	Minor uint16
	Patch uint16
}

// //

func resolveEffectiveVersion(config ConfigObj, graphObj *source.GraphObj) (*effectiveVersionObj, error) {
	nowValue := config.Now
	if nowValue.IsZero() {
		nowValue = time.Now()
	}

	infoObj, err := readOpenAPIInfo(graphObj.Document)
	if err != nil {
		return nil, err
	}

	manifestObj, manifestExistsFlag, err := meta.FindProject(graphObj.SourceRoot, config.MetaPath)
	if err != nil {
		return nil, err
	}

	resultObj := &effectiveVersionObj{
		Name:        infoObj.Title,
		Hash:        graphObj.Hash,
		DateUpdate:  nowValue,
		ManifestObj: manifestObj,
	}

	if !manifestExistsFlag {
		if strings.TrimSpace(config.MetaPath) != "" {
			return nil, fmt.Errorf("manifest file not found: %s", config.MetaPath)
		}
		if strings.TrimSpace(infoObj.Version) == "" {
			return nil, fmt.Errorf("OpenAPI info.version is empty and manifest is missing")
		}

		projectVersionObj, err := parseProjectVersion(infoObj.Version)
		if err != nil {
			return nil, err
		}

		resultObj.Version = projectVersionObj.Raw
		resultObj.VersionMajor = projectVersionObj.Major
		resultObj.VersionMinor = projectVersionObj.Minor
		resultObj.VersionPatch = projectVersionObj.Patch
		return resultObj, nil
	}

	valuesObj, err := manifestObj.Manifest()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(valuesObj.Hash) == "" {
		return nil, fmt.Errorf("manifest hash is empty: %s", manifestObj.ManifestPath())
	}

	projectVersionObj, err := parseProjectVersion(valuesObj.Ver)
	if err != nil {
		return nil, err
	}

	resultObj.Version = projectVersionObj.Raw
	resultObj.VersionMajor = projectVersionObj.Major
	resultObj.VersionMinor = projectVersionObj.Minor
	resultObj.VersionPatch = projectVersionObj.Patch

	if strings.TrimSpace(infoObj.Version) != "" {
		resultObj.WarningTextArr = append(resultObj.WarningTextArr, "manifest version overrides OpenAPI info.version")
	}

	if valuesObj.Hash == graphObj.Hash {
		infoFileObj, err := manifestObj.Stat()
		if err != nil {
			return nil, err
		}
		resultObj.DateUpdate = infoFileObj.ModTime()
		return resultObj, nil
	}

	resultObj.Version = appendBuildHash(projectVersionObj.Raw, graphObj.HashShort)
	resultObj.WarningTextArr = append(resultObj.WarningTextArr, fmt.Sprintf(
		"manifest hash %s differs from current hash %s; effective version is %s",
		valuesObj.Hash,
		graphObj.Hash,
		resultObj.Version,
	))
	return resultObj, nil
}

func readOpenAPIInfo(documentMap map[string]any) (*openAPIInfoObj, error) {
	rawInfoValue, existsFlag := documentMap["info"]
	if !existsFlag {
		return nil, fmt.Errorf("OpenAPI info object is missing")
	}

	infoMap, ok := rawInfoValue.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("OpenAPI info field must be object")
	}

	titleText, _ := infoMap["title"].(string)
	versionText, _ := infoMap["version"].(string)

	return &openAPIInfoObj{
		Title:   strings.TrimSpace(titleText),
		Version: strings.TrimSpace(versionText),
	}, nil
}

func parseProjectVersion(rawText string) (*projectVersionObj, error) {
	versionObj, err := meta.ParseVersion(rawText)
	if err != nil {
		return nil, err
	}

	if !isStrictUintText(versionObj.Major) {
		return nil, fmt.Errorf("project version major must be numeric: %s", rawText)
	}

	majorValue, err := strconv.ParseUint(versionObj.Major, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("project version major must be numeric: %s", rawText)
	}
	if majorValue > uint64(^uint16(0)) {
		return nil, fmt.Errorf("project version major exceeds uint16: %s", rawText)
	}
	if versionObj.Minor > uint64(^uint16(0)) {
		return nil, fmt.Errorf("project version minor exceeds uint16: %s", rawText)
	}
	if versionObj.Patch > uint64(^uint16(0)) {
		return nil, fmt.Errorf("project version patch exceeds uint16: %s", rawText)
	}

	return &projectVersionObj{
		Raw:   versionObj.Raw,
		Major: uint16(majorValue),
		Minor: uint16(versionObj.Minor),
		Patch: uint16(versionObj.Patch),
	}, nil
}

func isStrictUintText(rawText string) bool {
	if rawText == "" {
		return false
	}
	if len(rawText) > 1 && rawText[0] == '0' {
		return false
	}

	_, err := strconv.ParseUint(rawText, 10, 64)
	return err == nil
}

func appendBuildHash(versionText string, shortHashText string) string {
	if strings.Contains(versionText, "+") {
		return versionText + "." + shortHashText
	}

	return versionText + "+" + shortHashText
}
