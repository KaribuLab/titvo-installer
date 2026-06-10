package internal

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type OS string

const (
	Windows OS = "windows"
	Darwin  OS = "darwin"
	Linux   OS = "linux"
)

type Arch string

const (
	AMD64 Arch = "amd64"
	ARM64 Arch = "arm64"
)

func GetArch() (Arch, error) {
	switch runtime.GOARCH {
	case string(AMD64):
		return AMD64, nil
	case string(ARM64):
		return ARM64, nil
	default:
		return "", fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
	}
}

func GetOS() (OS, error) {
	switch runtime.GOOS {
	case string(Windows):
		return Windows, nil
	case string(Darwin):
		return Darwin, nil
	case string(Linux):
		return Linux, nil
	default:
		return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func IsWindows() bool {
	return runtime.GOOS == string(Windows)
}

func IsDarwin() bool {
	return runtime.GOOS == string(Darwin)
}

func IsLinux() bool {
	return runtime.GOOS == string(Linux)
}

type ExecuteOptions struct {
	WorkingDir string
	Env        map[string]string // Variables específicas para esta ejecución
}

const gitSSHAcceptNew = "ssh -o StrictHostKeyChecking=accept-new"

// gitExecuteEnv returns environment overrides for git commands that may reach
// hosts over SSH (e.g. submodules). accept-new auto-trusts a host on first
// contact without an interactive prompt; changed keys are still rejected.
func gitExecuteEnv() map[string]string {
	return map[string]string{"GIT_SSH_COMMAND": gitSSHAcceptNew}
}

func Execute(command string, args ...string) error {
	return ExecuteWithOptions(command, nil, args...)
}

// pathWithBinDirFirst prepends binDir to PATH so bundled tools win while system
// utilities (e.g. sh for npm lifecycle scripts) remain discoverable.
func pathWithBinDirFirst(binDir string) string {
	if binDir == "" {
		return os.Getenv("PATH")
	}
	sep := string(os.PathListSeparator)
	if existing := os.Getenv("PATH"); existing != "" {
		return binDir + sep + existing
	}
	return binDir
}

func mergeEnv(base []string, overrides map[string]string) []string {
	merged := make(map[string]string, len(base)+len(overrides))
	order := make([]string, 0, len(base)+len(overrides))

	for _, kv := range base {
		key, value, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if _, exists := merged[key]; !exists {
			order = append(order, key)
		}
		merged[key] = value
	}
	for key, value := range overrides {
		if _, exists := merged[key]; !exists {
			order = append(order, key)
		}
		merged[key] = value
	}

	env := make([]string, 0, len(order))
	for _, key := range order {
		env = append(env, key+"="+merged[key])
	}
	return env
}

func ExecuteWithOptions(command string, options *ExecuteOptions, args ...string) error {
	cmd := exec.Command(command, args...)

	// Siempre redirigir a stdout/stderr para output en vivo
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if options != nil {
		if options.WorkingDir != "" {
			cmd.Dir = options.WorkingDir
		}
		if options.Env != nil {
			cmd.Env = mergeEnv(os.Environ(), options.Env)
		}
	}

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("command failed: %v", err)
	}

	return nil
}
