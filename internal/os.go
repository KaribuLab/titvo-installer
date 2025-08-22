package internal

import (
	"fmt"
	"os/exec"
	"runtime"
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
	Env        []string
}

func Execute(command string, args ...string) (string, error) {
	return ExecuteWithOptions(command, nil, args...)
}

func ExecuteWithOptions(command string, options *ExecuteOptions, args ...string) (string, error) {
	cmd := exec.Command(command, args...)

	if options != nil {
		if options.WorkingDir != "" {
			cmd.Dir = options.WorkingDir
		}
		if options.Env != nil {
			cmd.Env = options.Env
		}
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return string(output), nil
}
