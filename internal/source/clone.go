package source

// // // // // // // // // //

func cloneAny(value any) any {
	switch castedValue := value.(type) {
	case map[string]any:
		resultMap := make(map[string]any, len(castedValue))
		for key, innerValue := range castedValue {
			resultMap[key] = cloneAny(innerValue)
		}
		return resultMap
	case []any:
		resultArr := make([]any, len(castedValue))
		for index, innerValue := range castedValue {
			resultArr[index] = cloneAny(innerValue)
		}
		return resultArr
	default:
		return value
	}
}
