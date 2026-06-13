package goopenapigen

import (
	"context"
	"time"

	"github.com/amazing-generators/goopenapigen/internal/source"
)

// // // // // // // // // //

// DefaultMaxRefDepth is the default $ref chain depth; single source of truth for the CLI.
const DefaultMaxRefDepth = source.DefaultMaxRefDepth

// Command selects a pipeline inside Run.
type Command string

const (
	CommandGenerate     Command = "generate"
	CommandJSON         Command = "json"
	CommandManifestSync Command = "manifest-sync"
	CommandMeta         Command = "meta"
)

// //

// ConfigObj is the shared input for all commands. Command decides which fields are relevant.
type ConfigObj struct {
	Context context.Context
	Command Command

	// Source is a root OpenAPI file or project directory.
	Source string

	// MetaPath is an explicit manifest path from -meta.
	MetaPath string

	// MaxRefDepth limits $ref chain depth.
	MaxRefDepth int

	OutputDir   string
	PackageName string

	OgenCommand string
	OgenTimeout time.Duration

	ManifestCreate bool
	ManifestFormat string
	Bump           string
	PreID          string

	RequireXFunc                  bool
	DisableComments               bool
	DisableCanonicalComponentRefs bool
	DisableOgen                   bool
	DisableRouter                 bool
	DisableHTTPDefaults           bool
	DisableMetaGo                 bool
	DisableOpenAPIJSONGo          bool
	Force                         bool
	Now                           time.Time
}

type ResultObj struct {
	Command              Command
	GeneratedFilePathArr []string
	WarningArr           []string
}
