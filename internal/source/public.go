package source

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// // // // // // // // // //

// PublicDocument returns the bundled document with internal extensions stripped
// and the version set — ready for publishing and for ogen.
func (obj *GraphObj) PublicDocument(versionText string) (map[string]any, error) {
	return obj.PublicDocumentWithOptions(versionText, PublicOptionsObj{
		CanonicalComponentRefs: true,
	})
}

// PublicDocumentWithOptions returns the publishable document with a configurable
// $ref strategy — useful for compatibility with older contracts.
func (obj *GraphObj) PublicDocumentWithOptions(versionText string, optionsObj PublicOptionsObj) (map[string]any, error) {
	documentMap, err := obj.BundleDocumentWithOptions(BundleOptionsObj{
		CanonicalComponentRefs: optionsObj.CanonicalComponentRefs,
	})
	if err != nil {
		return nil, err
	}

	removeInternalExtensions(documentMap)
	if err = setInfoVersion(documentMap, versionText); err != nil {
		return nil, err
	}

	return documentMap, nil
}

// RenderJSON prints the document as stable, readable JSON without HTML escaping.
func RenderJSON(documentMap map[string]any) ([]byte, error) {
	bufferObj := bytes.NewBuffer(nil)
	encoderObj := json.NewEncoder(bufferObj)
	encoderObj.SetEscapeHTML(false)
	encoderObj.SetIndent("", "  ")

	if err := encoderObj.Encode(documentMap); err != nil {
		return nil, fmt.Errorf("encode public OpenAPI JSON: %w", err)
	}

	return bufferObj.Bytes(), nil
}

// //

func setInfoVersion(documentMap map[string]any, versionText string) error {
	rawInfoValue, existsFlag := documentMap["info"]
	if !existsFlag {
		return fmt.Errorf("OpenAPI info object is missing")
	}

	infoMap, ok := rawInfoValue.(map[string]any)
	if !ok {
		return fmt.Errorf("OpenAPI info field must be object")
	}

	infoMap["version"] = versionText
	return nil
}

func removeInternalExtensions(value any) {
	switch castedValue := value.(type) {
	case map[string]any:
		delete(castedValue, "x-func")
		for _, innerValue := range castedValue {
			removeInternalExtensions(innerValue)
		}
	case []any:
		for _, innerValue := range castedValue {
			removeInternalExtensions(innerValue)
		}
	}
}
