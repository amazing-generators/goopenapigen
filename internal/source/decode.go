package source

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// // // // // // // // // //

func decodeDocument(pathText string, dataArr []byte) (any, error) {
	var documentValue any

	switch strings.ToLower(filepath.Ext(pathText)) {
	case ".json":
		decoderObj := json.NewDecoder(bytes.NewReader(dataArr))
		decoderObj.UseNumber()
		if err := decoderObj.Decode(&documentValue); err != nil {
			return nil, fmt.Errorf("decode json %s: %w", pathText, err)
		}
	case ".yml", ".yaml":
		if err := yaml.Unmarshal(dataArr, &documentValue); err != nil {
			return nil, fmt.Errorf("decode yaml %s: %w", pathText, err)
		}
	default:
		return nil, fmt.Errorf("unsupported source file format: %s", pathText)
	}

	return normalizeAny(documentValue)
}

func normalizeAny(value any) (any, error) {
	switch castedValue := value.(type) {
	case map[string]any:
		resultMap := make(map[string]any, len(castedValue))
		for key, innerValue := range castedValue {
			normalizedValue, err := normalizeAny(innerValue)
			if err != nil {
				return nil, err
			}
			resultMap[key] = normalizedValue
		}
		return resultMap, nil
	case map[any]any:
		resultMap := make(map[string]any, len(castedValue))
		for rawKey, innerValue := range castedValue {
			keyText, ok := rawKey.(string)
			if !ok {
				return nil, fmt.Errorf("yaml object key must be string: %T", rawKey)
			}

			normalizedValue, err := normalizeAny(innerValue)
			if err != nil {
				return nil, err
			}
			resultMap[keyText] = normalizedValue
		}
		return resultMap, nil
	case []any:
		resultArr := make([]any, len(castedValue))
		for index, innerValue := range castedValue {
			normalizedValue, err := normalizeAny(innerValue)
			if err != nil {
				return nil, err
			}
			resultArr[index] = normalizedValue
		}
		return resultArr, nil
	default:
		return value, nil
	}
}

func isSourceExt(pathText string) bool {
	switch strings.ToLower(filepath.Ext(pathText)) {
	case ".json", ".yml", ".yaml":
		return true
	default:
		return false
	}
}
