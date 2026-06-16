package goopenapigen

import (
	"fmt"
	"strings"

	"github.com/amazing-generators/goopenapigen/internal/meta"
	"github.com/amazing-generators/goopenapigen/internal/source"
)

// // // // // // // // // //

func resolveManifestBump(valuesObj *meta.ValuesObj, graphObj *source.GraphObj, config ConfigObj) (string, error) {
	if err := validateManifestBump(valuesObj.Ver, config); err != nil {
		return "", err
	}

	bumpText := strings.TrimSpace(config.Bump)
	autoBumpText := strings.TrimSpace(config.AutoBump)
	if autoBumpText == "" {
		return bumpText, nil
	}
	if valuesObj.Hash == graphObj.Hash {
		return "", nil
	}

	return autoBumpText, nil
}

func validateManifestBump(versionText string, config ConfigObj) error {
	bumpText := strings.TrimSpace(config.Bump)
	autoBumpText := strings.TrimSpace(config.AutoBump)
	if bumpText != "" && autoBumpText != "" {
		return fmt.Errorf("manifest sync cannot use -bump and -auto-bump together")
	}

	validateText := bumpText
	if validateText == "" {
		validateText = autoBumpText
	}
	if validateText == "" {
		return nil
	}

	_, err := bumpVersionIfRequested(versionText, validateText, config.PreID)
	return err
}

func bumpVersionIfRequested(versionText string, bumpText string, preIDText string) (string, error) {
	bumpText = strings.TrimSpace(strings.ToLower(bumpText))
	if bumpText == "" {
		return versionText, nil
	}

	versionObj, err := meta.ParseVersion(versionText)
	if err != nil {
		return "", err
	}

	switch bumpText {
	case "major":
		err = versionObj.IncrementMajor()
	case "minor":
		err = versionObj.IncrementMinor()
	case "patch":
		err = versionObj.IncrementPatch()
	case "premajor":
		err = versionObj.PreMajor(preIDText)
	case "preminor":
		err = versionObj.PreMinor(preIDText)
	case "prepatch":
		err = versionObj.PrePatch(preIDText)
	case "prerelease":
		err = versionObj.IncrementPrerelease(preIDText)
	default:
		return "", fmt.Errorf("unsupported version bump: %s", bumpText)
	}
	if err != nil {
		return "", err
	}

	return versionObj.String(), nil
}

func syncManifest(config ConfigObj, graphObj *source.GraphObj) (string, error) {
	infoObj, err := readOpenAPIInfo(graphObj.Document)
	if err != nil {
		return "", err
	}

	manifestObj, manifestExistsFlag, err := meta.FindProject(graphObj.SourceRoot, config.MetaPath)
	if err != nil {
		return "", err
	}

	if !manifestExistsFlag {
		if !config.ManifestCreate {
			return "", fmt.Errorf("manifest file not found (use -create to create it)")
		}
		if infoObj.Version == "" {
			return "", fmt.Errorf("OpenAPI info.version is empty")
		}
		if _, err = parseProjectVersion(infoObj.Version); err != nil {
			return "", err
		}
		if err = validateManifestBump(infoObj.Version, config); err != nil {
			return "", err
		}

		return meta.CreateProjectManifest(graphObj.SourceRoot, config.MetaPath, config.ManifestFormat, &meta.ValuesObj{
			Ver:  infoObj.Version,
			Hash: graphObj.Hash,
		}, true)
	}

	valuesObj, err := manifestObj.Manifest()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(valuesObj.Ver) == "" {
		return "", fmt.Errorf("manifest ver is empty: %s", manifestObj.ManifestPath())
	}

	bumpText, err := resolveManifestBump(valuesObj, graphObj, config)
	if err != nil {
		return "", err
	}
	nextVersionText, err := bumpVersionIfRequested(valuesObj.Ver, bumpText, config.PreID)
	if err != nil {
		return "", err
	}
	valuesObj.Ver = nextVersionText
	valuesObj.Hash = graphObj.Hash

	if _, err = parseProjectVersion(valuesObj.Ver); err != nil {
		return "", err
	}
	if err = manifestObj.Write(valuesObj, true); err != nil {
		return "", err
	}

	return manifestObj.ManifestPath(), nil
}
