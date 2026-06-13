package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/amazing-generators/goopenapigen"
	"github.com/amazing-generators/goopenapigen/internal/meta"
)

// // // // // // // // // //

const cDefaultOgenTimeout = 2 * time.Minute

// //

func runCLI(argsArr []string) error {
	if len(argsArr) == 0 {
		printRootUsage()
		return nil
	}

	switch argsArr[0] {
	case "generate":
		return runGenerateCommand(argsArr[1:])
	case "json":
		return runJSONCommand(argsArr[1:])
	case "meta":
		return runMetaCommand(argsArr[1:])
	case "manifest":
		return runManifestCommand(argsArr[1:])
	case "version":
		return runVersionCommand(argsArr[1:])
	case "help", "-h", "--help":
		printRootUsage()
		return nil
	default:
		return fmt.Errorf("unknown command: %s", argsArr[0])
	}
}

// //

// notifyContext cancels long-running commands on SIGINT/SIGTERM.
func notifyContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

func runGenerateCommand(argsArr []string) error {
	flagSet := newFlagSet("generate")
	config := goopenapigen.ConfigObj{
		Command: goopenapigen.CommandGenerate,
	}
	ogenFlag := true
	routerFlag := true
	httpDefaultsFlag := true
	metaGoFlag := true
	openAPIJSONGoFlag := true
	commentsFlag := true
	canonicalComponentRefsFlag := true

	contextObj, stopFunc := notifyContext()
	defer stopFunc()
	config.Context = contextObj

	flagSet.StringVar(&config.Source, "source", "", "root OpenAPI file or project directory")
	flagSet.StringVar(&config.OutputDir, "out", "", "output directory for generated Go files")
	flagSet.StringVar(&config.PackageName, "pkg", "", "Go package name; defaults to the output directory name")
	flagSet.StringVar(&config.MetaPath, "meta", "", "explicit manifest path")
	flagSet.IntVar(&config.MaxRefDepth, "max-ref-depth", goopenapigen.DefaultMaxRefDepth, "maximum $ref chain depth")
	flagSet.StringVar(&config.OgenCommand, "ogen-command", "", "explicit ogen binary path")
	flagSet.DurationVar(&config.OgenTimeout, "ogen-timeout", cDefaultOgenTimeout, "ogen execution timeout")
	flagSet.BoolVar(&ogenFlag, "ogen", true, "generate ogen package")
	flagSet.BoolVar(&routerFlag, "router", true, "generate router bridge; disabled automatically when -ogen=false")
	flagSet.BoolVar(&httpDefaultsFlag, "http-defaults", true, "generate default HTTP handlers; disabled automatically when -router=false")
	flagSet.BoolVar(&metaGoFlag, "meta-go", true, "generate OpenAPI metadata Go file")
	flagSet.BoolVar(&config.RequireXFunc, "require-x-func", false, "require x-func mappings for all operations and security schemes")
	flagSet.BoolVar(&commentsFlag, "comments", true, "carry route summary/description into FuncInterface doc comments")
	flagSet.BoolVar(&openAPIJSONGoFlag, "openapi-json-go", true, "generate compressed OpenAPI JSON Go file next to router bridge")
	flagSet.BoolVar(&canonicalComponentRefsFlag, "canonical-component-refs", true, "keep component file references as canonical #/components refs")
	flagSet.BoolVar(&config.Force, "force", false, "create missing output directories")
	flagSet.Usage = usageFunc(flagSet, "goopenapigen generate [flags]")

	if err := flagSet.Parse(argsArr); err != nil {
		return err
	}
	config.DisableOgen = !ogenFlag
	config.DisableRouter = !routerFlag
	config.DisableHTTPDefaults = !httpDefaultsFlag
	config.DisableMetaGo = !metaGoFlag
	config.DisableOpenAPIJSONGo = !openAPIJSONGoFlag
	config.DisableComments = !commentsFlag
	config.DisableCanonicalComponentRefs = !canonicalComponentRefsFlag

	resultObj, err := goopenapigen.Run(config)
	if err != nil {
		return err
	}

	reportGenerated(resultObj)
	return nil
}

func runJSONCommand(argsArr []string) error {
	if len(argsArr) == 0 || argsArr[0] != "generate" {
		return fmt.Errorf("usage: goopenapigen json generate [flags]")
	}

	flagSet := newFlagSet("json generate")
	config := goopenapigen.ConfigObj{Command: goopenapigen.CommandJSON}
	canonicalComponentRefsFlag := true

	contextObj, stopFunc := notifyContext()
	defer stopFunc()
	config.Context = contextObj

	flagSet.StringVar(&config.Source, "source", "", "root OpenAPI file or project directory")
	flagSet.StringVar(&config.OutputDir, "out", "", "output directory for openapi.json")
	flagSet.StringVar(&config.MetaPath, "meta", "", "explicit manifest path")
	flagSet.IntVar(&config.MaxRefDepth, "max-ref-depth", goopenapigen.DefaultMaxRefDepth, "maximum $ref chain depth")
	flagSet.BoolVar(&canonicalComponentRefsFlag, "canonical-component-refs", true, "keep component file references as canonical #/components refs")
	flagSet.BoolVar(&config.Force, "force", false, "create missing output directory")
	flagSet.Usage = usageFunc(flagSet, "goopenapigen json generate [flags]")

	if err := flagSet.Parse(argsArr[1:]); err != nil {
		return err
	}
	config.DisableCanonicalComponentRefs = !canonicalComponentRefsFlag

	resultObj, err := goopenapigen.Run(config)
	if err != nil {
		return err
	}

	reportGenerated(resultObj)
	return nil
}

func runMetaCommand(argsArr []string) error {
	if len(argsArr) == 0 || argsArr[0] != "generate" {
		return fmt.Errorf("usage: goopenapigen meta generate [flags]")
	}

	flagSet := newFlagSet("meta generate")
	config := goopenapigen.ConfigObj{Command: goopenapigen.CommandMeta}

	contextObj, stopFunc := notifyContext()
	defer stopFunc()
	config.Context = contextObj

	flagSet.StringVar(&config.Source, "source", "", "root OpenAPI file or project directory")
	flagSet.StringVar(&config.MetaPath, "meta", "", "explicit manifest path")
	flagSet.IntVar(&config.MaxRefDepth, "max-ref-depth", goopenapigen.DefaultMaxRefDepth, "maximum $ref chain depth")
	flagSet.StringVar(&config.OutputDir, "out", "", "output directory for openapi_meta_gen.go")
	flagSet.StringVar(&config.PackageName, "pkg", "", "Go package name; defaults to the output directory name")
	flagSet.BoolVar(&config.Force, "force", false, "create missing output directories")
	flagSet.Usage = usageFunc(flagSet, "goopenapigen meta generate [flags]")

	if err := flagSet.Parse(argsArr[1:]); err != nil {
		return err
	}

	resultObj, err := goopenapigen.Run(config)
	if err != nil {
		return err
	}

	reportGenerated(resultObj)
	return nil
}

func runManifestCommand(argsArr []string) error {
	if len(argsArr) == 0 {
		return fmt.Errorf("manifest subcommand is required: get or sync")
	}

	switch argsArr[0] {
	case "get":
		return runManifestGetCommand(argsArr[1:])
	case "sync":
		return runManifestSyncCommand(argsArr[1:])
	default:
		return fmt.Errorf("unknown manifest subcommand: %s", argsArr[0])
	}
}

func runManifestSyncCommand(argsArr []string) error {
	flagSet := newFlagSet("manifest sync")
	config := goopenapigen.ConfigObj{Command: goopenapigen.CommandManifestSync}

	contextObj, stopFunc := notifyContext()
	defer stopFunc()
	config.Context = contextObj

	flagSet.StringVar(&config.Source, "source", "", "project directory containing the root OpenAPI file")
	flagSet.StringVar(&config.MetaPath, "meta", "", "explicit manifest path")
	flagSet.IntVar(&config.MaxRefDepth, "max-ref-depth", goopenapigen.DefaultMaxRefDepth, "maximum $ref chain depth")
	flagSet.BoolVar(&config.ManifestCreate, "create", false, "create missing OpenAPI project manifest")
	flagSet.StringVar(&config.ManifestFormat, "format", "yaml", "created manifest format: json, yaml, or yml")
	flagSet.StringVar(&config.Bump, "bump", "", "version bump: major, minor, patch, or prerelease")
	flagSet.StringVar(&config.PreID, "preid", "", "prerelease identifier prefix")
	flagSet.BoolVar(&config.Force, "force", false, "create missing output directories")
	flagSet.Usage = usageFunc(flagSet, "goopenapigen manifest sync [flags]")

	if err := flagSet.Parse(argsArr); err != nil {
		return err
	}

	_, err := goopenapigen.Run(config)
	return err
}

// runManifestGetCommand reads one manifest field for scripts.
func runManifestGetCommand(argsArr []string) error {
	flagSet := newFlagSet("manifest get")
	sourcePath := ""
	fieldValue := ""

	flagSet.StringVar(&sourcePath, "source", "", "manifest file or project directory")
	flagSet.StringVar(&fieldValue, "field", "", "manifest field: ver, hash, or values name")
	flagSet.Usage = usageFunc(flagSet, "goopenapigen manifest get [flags]")

	if err := flagSet.Parse(argsArr); err != nil {
		return err
	}

	metaObj, err := meta.NewProject(sourcePath)
	if err != nil {
		return err
	}

	valuesObj, err := metaObj.Manifest()
	if err != nil {
		return err
	}

	switch strings.ToLower(strings.TrimSpace(fieldValue)) {
	case "name":
		if strings.TrimSpace(valuesObj.Name) == "" {
			return fmt.Errorf("manifest name is empty")
		}
		_, _ = fmt.Fprintln(os.Stdout, valuesObj.Name)
	case "ver":
		_, _ = fmt.Fprintln(os.Stdout, valuesObj.Ver)
	case "hash":
		_, _ = fmt.Fprintln(os.Stdout, valuesObj.Hash)
	default:
		return fmt.Errorf("unsupported manifest field: %s", fieldValue)
	}

	return nil
}

// runVersionCommand updates this tool version through its manifest (_run/values.yml).
func runVersionCommand(argsArr []string) error {
	if len(argsArr) == 0 {
		return fmt.Errorf("version subcommand is required")
	}

	subCommand := argsArr[0]
	subArgs := argsArr[1:]

	switch subCommand {
	case "print":
		return runVersionMutationCommand(subCommand, subArgs, false, func(metaObj *meta.Obj, _ string) (string, error) {
			return metaObj.VersionString()
		})
	case "major":
		return runVersionMutationCommand(subCommand, subArgs, false, func(metaObj *meta.Obj, _ string) (string, error) {
			return metaObj.IncrementMajor()
		})
	case "minor":
		return runVersionMutationCommand(subCommand, subArgs, false, func(metaObj *meta.Obj, _ string) (string, error) {
			return metaObj.IncrementMinor()
		})
	case "patch":
		return runVersionMutationCommand(subCommand, subArgs, false, func(metaObj *meta.Obj, _ string) (string, error) {
			return metaObj.IncrementPatch()
		})
	case "premajor":
		return runVersionMutationCommand(subCommand, subArgs, true, func(metaObj *meta.Obj, preID string) (string, error) {
			return metaObj.PreMajor(preID)
		})
	case "preminor":
		return runVersionMutationCommand(subCommand, subArgs, true, func(metaObj *meta.Obj, preID string) (string, error) {
			return metaObj.PreMinor(preID)
		})
	case "prepatch":
		return runVersionMutationCommand(subCommand, subArgs, true, func(metaObj *meta.Obj, preID string) (string, error) {
			return metaObj.PrePatch(preID)
		})
	case "prerelease":
		return runVersionMutationCommand(subCommand, subArgs, true, func(metaObj *meta.Obj, preID string) (string, error) {
			return metaObj.Prerelease(preID)
		})
	default:
		return fmt.Errorf("unknown version subcommand: %s", subCommand)
	}
}

func runVersionMutationCommand(
	name string,
	argsArr []string,
	usePreID bool,
	runFunc func(metaObj *meta.Obj, preID string) (string, error),
) error {
	flagSet := newFlagSet("version " + name)
	sourcePath := ""
	preIDValue := ""

	flagSet.StringVar(&sourcePath, "source", "", "manifest file or project directory")
	if usePreID {
		flagSet.StringVar(&preIDValue, "preid", "", "prerelease identifier prefix")
	}
	flagSet.Usage = usageFunc(flagSet, "goopenapigen version "+name+" [flags]")

	if err := flagSet.Parse(argsArr); err != nil {
		return err
	}

	metaObj, err := meta.New(sourcePath)
	if err != nil {
		return err
	}

	versionValue, err := runFunc(metaObj, preIDValue)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(os.Stdout, versionValue)
	return nil
}

// //

func newFlagSet(name string) *flag.FlagSet {
	flagSet := flag.NewFlagSet(name, flag.ContinueOnError)
	flagSet.SetOutput(os.Stderr)
	return flagSet
}

func usageFunc(flagSet *flag.FlagSet, usageLine string) func() {
	return func() {
		_, _ = fmt.Fprintln(os.Stderr, "Usage: "+usageLine)
		flagSet.PrintDefaults()
	}
}

func reportGenerated(resultObj *goopenapigen.ResultObj) {
	if resultObj == nil {
		return
	}

	for _, warningText := range resultObj.WarningArr {
		_, _ = fmt.Fprintln(os.Stderr, "Warning:", warningText)
	}

	for _, pathValue := range resultObj.GeneratedFilePathArr {
		_, _ = fmt.Fprintln(os.Stdout, "Generated:", pathValue)
	}
}

func printRootUsage() {
	_, _ = fmt.Fprintln(os.Stderr, "Usage:")
	_, _ = fmt.Fprintln(os.Stderr, "  goopenapigen generate [flags]")
	_, _ = fmt.Fprintln(os.Stderr, "  goopenapigen json generate [flags]")
	_, _ = fmt.Fprintln(os.Stderr, "  goopenapigen meta generate [flags]")
	_, _ = fmt.Fprintln(os.Stderr, "  goopenapigen manifest sync [flags]")
	_, _ = fmt.Fprintln(os.Stderr, "  goopenapigen manifest get [flags]")
	_, _ = fmt.Fprintln(os.Stderr, "  goopenapigen version print [flags]")
	_, _ = fmt.Fprintln(os.Stderr, "  goopenapigen version major|minor|patch [flags]")
	_, _ = fmt.Fprintln(os.Stderr, "  goopenapigen version premajor|preminor|prepatch|prerelease [flags]")
}
