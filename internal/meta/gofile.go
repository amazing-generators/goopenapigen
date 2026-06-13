package meta

import (
	"bytes"
	_ "embed"
	"fmt"
	"go/format"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/amazing-generators/goopenapigen/internal/goident"
	"github.com/amazing-generators/goopenapigen/internal/write"
)

// // // // // // // // // //

//go:embed templates/openapi_meta.go.tmpl
var goFileTemplateText string

var goFileTemplateObj = template.Must(template.New("openapi_meta.go.tmpl").Funcs(template.FuncMap{
	"quote": strconv.Quote,
}).Parse(goFileTemplateText))

type GoConfigObj struct {
	PackageName  string
	Name         string
	Version      string
	VersionMajor uint16
	VersionMinor uint16
	VersionPatch uint16
	Hash         string
	DateUpdate   time.Time
}

type goFileTemplateDataObj struct {
	Header       string
	PackageName  string
	Name         string
	Version      string
	VersionMajor uint16
	VersionMinor uint16
	VersionPatch uint16
	Hash         string
	DateUpdate   string
}

// //

// GenerateGo renders the project metadata file (Name/Version/Hash/DateUpdate).
func GenerateGo(configObj GoConfigObj) ([]byte, error) {
	packageName := strings.TrimSpace(configObj.PackageName)
	if !goident.IsPackageName(packageName) {
		return nil, fmt.Errorf("invalid Go package name: %s", packageName)
	}

	bufferObj := bytes.NewBuffer(nil)
	if err := goFileTemplateObj.Execute(bufferObj, goFileTemplateDataObj{
		Header:       write.GeneratedHeader,
		PackageName:  packageName,
		Name:         configObj.Name,
		Version:      configObj.Version,
		VersionMajor: configObj.VersionMajor,
		VersionMinor: configObj.VersionMinor,
		VersionPatch: configObj.VersionPatch,
		Hash:         configObj.Hash,
		DateUpdate:   configObj.DateUpdate.Format("2006-01-02"),
	}); err != nil {
		return nil, fmt.Errorf("render metadata template: %w", err)
	}

	formattedArr, err := format.Source(bufferObj.Bytes())
	if err != nil {
		return nil, fmt.Errorf("format metadata Go: %w\n%s", err, bufferObj.String())
	}

	return formattedArr, nil
}
