package source

import (
	"crypto/sha256"
	"fmt"
	"sort"
)

// // // // // // // // // //

func calculateHash(loaderObj *loaderObj) ([]FileObj, string, string, error) {
	fileArr := make([]FileObj, 0, len(loaderObj.fileMap))
	for _, trackedObj := range loaderObj.fileMap {
		fileArr = append(fileArr, trackedObj.fileObj)
	}

	sort.Slice(fileArr, func(leftIndex int, rightIndex int) bool {
		return fileArr[leftIndex].RelPath < fileArr[rightIndex].RelPath
	})

	hashObj := sha256.New()
	for _, fileObj := range fileArr {
		if _, err := fmt.Fprintf(hashObj, "%s\n%d\n%s\n", fileObj.RelPath, fileObj.Size, fileObj.Hash); err != nil {
			return nil, "", "", err
		}
	}

	hashValue := fmt.Sprintf("%x", hashObj.Sum(nil))
	shortValue := hashValue
	if len(shortValue) > 4 {
		shortValue = shortValue[:4]
	}

	return fileArr, hashValue, shortValue, nil
}
