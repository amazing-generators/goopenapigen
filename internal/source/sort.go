package source

import "sort"

// // // // // // // // // //

func sortedMapKeys(valueMap map[string]any) []string {
	keyArr := make([]string, 0, len(valueMap))
	for key := range valueMap {
		keyArr = append(keyArr, key)
	}
	sort.Strings(keyArr)
	return keyArr
}
