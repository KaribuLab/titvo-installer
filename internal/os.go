package internal

import (
	"fmt"
	"os"
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
	Env        map[string]string // Variables específicas para esta ejecución
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
			// Comenzar con el entorno actual del proceso
			env := os.Environ()
			// Agregar/sobrescribir las variables específicas
			for key, value := range options.Env {
				env = append(env, fmt.Sprintf("%s=%s", key, value))
			}
			cmd.Env = env
		}
	}

	// CombinedOutput captura stdout y stderr juntos
	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	// Si hay error, incluir la salida en el mensaje de error
	if err != nil {
		if outputStr != "" {
			return outputStr, fmt.Errorf("command failed with exit code %v: %s", err, outputStr)
		}
		return outputStr, fmt.Errorf("command failed: %v", err)
	}

	return outputStr, nil
}
