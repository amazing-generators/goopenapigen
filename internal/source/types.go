package source

import (
	"context"
)

// // // // // // // // // //

type OptionsObj struct {
	Context     context.Context
	Source      string
	MaxRefDepth int
}

type FileObj struct {
	AbsPath string
	RelPath string
	Size    int64
	Hash    string
}

type GraphObj struct {
	SourceRoot string
	RootFile   string
	Document   map[string]any
	Hash       string
	HashShort  string
	FileArr    []FileObj

	loaderObj *loaderObj
}

type PublicOptionsObj struct {
	CanonicalComponentRefs bool
}

type BundleOptionsObj struct {
	CanonicalComponentRefs bool
}

// //

func (obj *GraphObj) CloneDocument() map[string]any {
	clonedValue := cloneAny(obj.Document)
	clonedMap, _ := clonedValue.(map[string]any)
	return clonedMap
}

func (obj *GraphObj) FilePathArr() []string {
	resultArr := make([]string, 0, len(obj.FileArr))
	for _, fileObj := range obj.FileArr {
		resultArr = append(resultArr, fileObj.AbsPath)
	}

	return resultArr
}
