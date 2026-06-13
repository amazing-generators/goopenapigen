package source

import (
	"crypto/sha256"
	"fmt"
	"os"
)

// // // // // // // // // //

type trackedFileObj struct {
	fileObj FileObj
	dataArr []byte
}

type loaderObj struct {
	sourceRoot string
	rootFile   string

	documentMap         map[string]any
	fileMap             map[string]*trackedFileObj
	rootRelativeFileMap map[string]struct{}

	assembledDocument any
}

// //

func newLoader(sourceRoot string, rootFile string) *loaderObj {
	return &loaderObj{
		sourceRoot:          sourceRoot,
		rootFile:            rootFile,
		documentMap:         make(map[string]any),
		fileMap:             make(map[string]*trackedFileObj),
		rootRelativeFileMap: make(map[string]struct{}),
	}
}

func (obj *loaderObj) markRootRelative(absPath string) {
	obj.rootRelativeFileMap[absPath] = struct{}{}
}

func (obj *loaderObj) isRootRelative(absPath string) bool {
	_, existsFlag := obj.rootRelativeFileMap[absPath]
	return existsFlag
}

func (obj *loaderObj) loadDocument(absPath string) (any, error) {
	absPath, err := cleanAbs(absPath)
	if err != nil {
		return nil, err
	}

	if cachedValue, existsFlag := obj.documentMap[absPath]; existsFlag {
		return cachedValue, nil
	}

	trackedObj, err := obj.trackFile(absPath)
	if err != nil {
		return nil, err
	}

	documentValue, err := decodeDocument(absPath, trackedObj.dataArr)
	if err != nil {
		return nil, err
	}

	obj.documentMap[absPath] = documentValue
	return documentValue, nil
}

func (obj *loaderObj) trackFile(absPath string) (*trackedFileObj, error) {
	absPath, err := cleanAbs(absPath)
	if err != nil {
		return nil, err
	}

	if cachedObj, existsFlag := obj.fileMap[absPath]; existsFlag {
		return cachedObj, nil
	}

	if err = rejectSymlinkPath(obj.sourceRoot, absPath); err != nil {
		return nil, err
	}

	infoObj, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("stat source file %s: %w", absPath, err)
	}
	if infoObj.IsDir() {
		return nil, fmt.Errorf("source file is a directory: %s", absPath)
	}

	dataArr, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read source file %s: %w", absPath, err)
	}

	relPath, err := relativeSlash(obj.sourceRoot, absPath)
	if err != nil {
		return nil, err
	}

	hashArr := sha256.Sum256(dataArr)
	trackedObj := &trackedFileObj{
		fileObj: FileObj{
			AbsPath: absPath,
			RelPath: relPath,
			Size:    int64(len(dataArr)),
			Hash:    fmt.Sprintf("%x", hashArr),
		},
		dataArr: dataArr,
	}

	obj.fileMap[absPath] = trackedObj
	return trackedObj, nil
}
