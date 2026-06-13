package bridge

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/amazing-generators/goopenapigen/internal/write"
)

// // // // // // // // // //

type OgenConfigObj struct {
	Context     context.Context
	Command     string
	InputPath   string
	TargetDir   string
	PackageName string
	Timeout     time.Duration
	Force       bool
}

// //

func RunOgen(configObj OgenConfigObj) error {
	if strings.TrimSpace(configObj.InputPath) == "" {
		return fmt.Errorf("ogen input path is empty")
	}
	if strings.TrimSpace(configObj.TargetDir) == "" {
		return fmt.Errorf("ogen output directory is empty")
	}
	if strings.TrimSpace(configObj.PackageName) == "" {
		return fmt.Errorf("ogen package name is empty")
	}

	if err := write.PrepareDir(configObj.TargetDir, configObj.Force); err != nil {
		return err
	}

	commandPath, err := resolveOgenCommand(configObj.Command)
	if err != nil {
		return err
	}

	configPath, cleanupFunc, err := writeTempOgenConfig()
	if err != nil {
		return err
	}
	defer cleanupFunc()

	contextObj := configObj.Context
	if contextObj == nil {
		contextObj = context.Background()
	}
	if configObj.Timeout > 0 {
		var cancelFunc context.CancelFunc
		contextObj, cancelFunc = context.WithTimeout(contextObj, configObj.Timeout)
		defer cancelFunc()
	}

	argsArr := []string{
		"-config", configPath,
		"-target", configObj.TargetDir,
		"-package", configObj.PackageName,
		configObj.InputPath,
	}

	return runOgenCommand(contextObj, commandPath, argsArr)
}

func resolveOgenCommand(explicitCommand string) (string, error) {
	explicitCommand = strings.TrimSpace(explicitCommand)
	if explicitCommand != "" {
		return explicitCommand, nil
	}

	if commandPath, err := exec.LookPath("ogen"); err == nil {
		return commandPath, nil
	}

	if homeDir, err := os.UserHomeDir(); err == nil {
		candidatePath := filepath.Join(homeDir, "go", "bin", "ogen")
		if isExecutableFile(candidatePath) {
			return candidatePath, nil
		}
	}

	goPath := os.Getenv("GOPATH")
	if goPath == "" {
		if valueText, err := goEnv("GOPATH"); err == nil {
			goPath = valueText
		}
	}
	for _, pathEntry := range filepath.SplitList(goPath) {
		if pathEntry == "" {
			continue
		}

		candidatePath := filepath.Join(pathEntry, "bin", "ogen")
		if isExecutableFile(candidatePath) {
			return candidatePath, nil
		}
	}

	return "", fmt.Errorf("ogen binary not found in PATH or GOPATH/bin")
}

// writeTempOgenConfig writes a minimal ogen config. The source is already bundled by us
// into a single JSON, so remote refs are disallowed and ogen expand is not used.
func writeTempOgenConfig() (string, func(), error) {
	tempFile, err := os.CreateTemp("", "goopenapigen-ogen-*.yaml")
	if err != nil {
		return "", nil, fmt.Errorf("create temporary ogen config: %w", err)
	}

	tempPath := tempFile.Name()
	cleanupFunc := func() {
		_ = os.Remove(tempPath)
	}

	configText := "parser:\n" +
		"  allow_remote: false\n" +
		"generator:\n" +
		"  ignore_not_implemented: [\"openIdConnect security\"]\n"

	if _, err = tempFile.WriteString(configText); err != nil {
		_ = tempFile.Close()
		cleanupFunc()
		return "", nil, fmt.Errorf("write temporary ogen config: %w", err)
	}
	if err = tempFile.Close(); err != nil {
		cleanupFunc()
		return "", nil, fmt.Errorf("close temporary ogen config: %w", err)
	}

	return tempPath, cleanupFunc, nil
}

func runOgenCommand(contextObj context.Context, commandPath string, argsArr []string) error {
	commandObj := exec.CommandContext(contextObj, commandPath, argsArr...)
	outputBufferObj := bytes.NewBuffer(nil)
	commandObj.Stdout = outputBufferObj
	commandObj.Stderr = outputBufferObj

	if err := commandObj.Run(); err != nil {
		outputText := strings.TrimSpace(outputBufferObj.String())
		if outputText == "" {
			return fmt.Errorf("run ogen: %w", err)
		}

		return fmt.Errorf("run ogen: %w\n%s", err, outputText)
	}

	return nil
}

func isExecutableFile(pathText string) bool {
	infoObj, err := os.Stat(pathText)
	if err != nil {
		return false
	}

	return !infoObj.IsDir()
}

func goEnv(keyText string) (string, error) {
	commandObj := exec.Command("go", "env", keyText)
	outputArr, err := commandObj.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(outputArr)), nil
}
