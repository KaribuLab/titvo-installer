package internal

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
)

// https://github.com/gruntwork-io/terragrunt/releases/download/v0.69.1/terragrunt_darwin_amd64
// https://github.com/gruntwork-io/terragrunt/releases/download/v0.69.1/terragrunt_darwin_arm64
// https://github.com/gruntwork-io/terragrunt/releases/download/v0.69.1/terragrunt_windows_amd64.exe
// https://github.com/gruntwork-io/terragrunt/releases/download/v0.69.1/terragrunt_linux_amd64
const terragruntUrl = "https://github.com/gruntwork-io/terragrunt/releases/download/v%s/terragrunt_%s_%s"

// https://releases.hashicorp.com/terraform/1.13.0/terraform_1.13.0_darwin_amd64.zip
// https://releases.hashicorp.com/terraform/1.13.0/terraform_1.13.0_darwin_arm64.zip
// https://releases.hashicorp.com/terraform/1.13.0/terraform_1.13.0_windows_amd64.zip
// https://releases.hashicorp.com/terraform/1.13.0/terraform_1.13.0_linux_amd64.zip
const terraformUrl = "https://releases.hashicorp.com/terraform/%s/terraform_%s_%s_%s.%s"

// https://nodejs.org/download/release/v20.19.4/node-v20.19.4-darwin-x64.tar.gz
// https://nodejs.org/download/release/v20.19.4/node-v20.19.4-darwin-arm64.tar.gz
// https://nodejs.org/download/release/v20.19.4/node-v20.19.4-linux-x64.tar.gz
// https://nodejs.org/download/release/v20.19.4/node-v20.19.4-win-x64.zip
const nodeUrl = "https://nodejs.org/download/release/v%s/node-v%s-%s-%s.%s"

func toolExecutable(name string, osType OS) string {
	if osType == Windows {
		return name + ".exe"
	}
	return name
}

func TerragruntBinaryPath(binDir string, osType OS) string {
	return path.Join(binDir, toolExecutable("terragrunt", osType))
}

func TerraformBinaryPath(binDir string, osType OS) string {
	return path.Join(binDir, toolExecutable("terraform", osType))
}

func toolBinaryVersion(binaryPath string) (string, error) {
	output, err := exec.Command(binaryPath, "--version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("command failed: %v", err)
	}
	versionLine, _, _ := strings.Cut(strings.TrimSpace(string(output)), "\n")
	return versionLine, nil
}

func logToolBinary(label, binaryPath string) error {
	if _, err := os.Stat(binaryPath); err != nil {
		return fmt.Errorf("%s binary not found at %s: %w", label, binaryPath, err)
	}
	version, err := toolBinaryVersion(binaryPath)
	if err != nil {
		return fmt.Errorf("%s binary at %s failed version check: %w", label, binaryPath, err)
	}
	printInfo(fmt.Sprintf("Using %s: %s (%s)", label, binaryPath, version))
	return nil
}

func warnIfDifferentBinaryInPath(commandName, expectedPath string) {
	lookedUp, err := exec.LookPath(commandName)
	if err != nil {
		return
	}
	if lookedUp != expectedPath {
		printAskQuestion(fmt.Sprintf(
			"Warning: %q in PATH resolves to %s, but installer will use %s",
			commandName, lookedUp, expectedPath,
		))
	}
}

func LogConfiguredToolBinaries(config InstallToolConfig) error {
	terragruntPath := TerragruntBinaryPath(config.TerragruntBinDir, config.OS)
	terraformPath := TerraformBinaryPath(config.TerraformBinDir, config.OS)
	nodePath := path.Join(config.NodeBinDir, toolExecutable("node", config.OS))

	if err := logToolBinary("Terragrunt", terragruntPath); err != nil {
		return err
	}
	if err := logToolBinary("Terraform (TG_TF_PATH)", terraformPath); err != nil {
		return err
	}
	if err := logToolBinary("Node", nodePath); err != nil {
		return err
	}

	warnIfDifferentBinaryInPath("terragrunt", terragruntPath)
	warnIfDifferentBinaryInPath("terraform", terraformPath)

	return nil
}

func DownloadTerragrunt(dir string, version string, osType OS, arch Arch) (string, error) {
	url := fmt.Sprintf(terragruntUrl, version, osType, arch)
	printInfo("Downloading Terragrunt")
	printInfo(url)
	fileExtension := ""
	if osType == Windows {
		fileExtension = ".exe"
	}
	fileName := fmt.Sprintf("terragrunt%s", fileExtension)
	err := downloadFile(url, dir, fileName)
	if err != nil {
		return "", err
	}
	if osType != Windows {
		// Give execute permission to the file
		err = os.Chmod(path.Join(dir, fileName), 0755)
		if err != nil {
			return "", err
		}
	}
	terragruntPath := TerragruntBinaryPath(dir, osType)
	err = ExecuteWithOptions(terragruntPath, &ExecuteOptions{
		WorkingDir: dir,
	}, "--version")
	if err != nil {
		return "", err
	}
	return dir, nil
}

func DownloadTerraform(dir string, version string, osType OS, arch Arch) (string, error) {
	url := fmt.Sprintf(terraformUrl, version, version, osType, arch, "zip")
	printInfo("Downloading Terraform")
	printInfo(url)
	zipFileName := "terraform.zip"
	err := downloadFile(url, dir, zipFileName)
	if err != nil {
		return "", err
	}

	// Extraer el ZIP
	zipPath := path.Join(dir, zipFileName)
	err = extractZip(zipPath, dir)
	if err != nil {
		return "", err
	}

	terraformPath := TerraformBinaryPath(dir, osType)
	err = ExecuteWithOptions(terraformPath, &ExecuteOptions{
		WorkingDir: dir,
	}, "--version")
	if err != nil {
		return "", err
	}
	// Eliminar el archivo ZIP después de extraer
	return dir, os.Remove(zipPath)
}

func DownloadNode(dir string, version string, osType OS, arch Arch) (string, error) {
	var url string
	var nodeDir string
	switch osType {
	case Windows:
		url = fmt.Sprintf(nodeUrl, version, version, "win", "x64", "zip")
		nodeDir = fmt.Sprintf("node-v%s-%s-%s", version, "win", "x64")
	default:
		archDownload := "x64"
		if osType == Darwin && arch == ARM64 {
			archDownload = "arm64"
		}
		url = fmt.Sprintf(nodeUrl, version, version, osType, archDownload, "tar.gz")
		nodeDir = fmt.Sprintf("node-v%s-%s-%s", version, osType, archDownload)
	}
	printInfo("Downloading Node")
	printInfo(url)
	tarFileName := "node.tar.gz"
	err := downloadFile(url, dir, tarFileName)
	if err != nil {
		return "", err
	}
	tarPath := path.Join(dir, tarFileName)
	err = extractTarGz(tarPath, dir)
	if err != nil {
		return "", err
	}
	nodeBinDir := path.Join(dir, nodeDir, "bin")
	nodePath := path.Join(nodeBinDir, toolExecutable("node", osType))
	err = ExecuteWithOptions(nodePath, &ExecuteOptions{
		WorkingDir: nodeBinDir,
	}, "--version")
	if err != nil {
		return "", err
	}
	npmCommand := "npm"
	if osType == Windows {
		npmCommand = path.Join(nodeBinDir, "npm.cmd")
	}
	err = ExecuteWithOptions(npmCommand, &ExecuteOptions{
		WorkingDir: nodeBinDir,
	}, "--version")
	if err != nil {
		return "", err
	}
	return nodeDir, os.Remove(tarPath)
}

type InstallToolConfig struct {
	Dir              string
	OS               OS
	Arch             Arch
	TitvoDir         string
	TerraformBinDir  string
	NodeBinDir       string
	TerragruntBinDir string
}

func InstallTools() (config *InstallToolConfig, err error) {
	// Get home directory
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	titvoDir := path.Join(home, ".titvo")
	binDir := path.Join(titvoDir, "bin")
	printInfo(fmt.Sprintf("Installing Tools in %s", binDir))
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return nil, err
	}
	os, err := GetOS()
	if err != nil {
		return nil, err
	}
	arch, err := GetArch()
	if err != nil {
		return nil, err
	}
	terragruntDir, err := DownloadTerragrunt(binDir, "0.69.1", os, arch)
	if err != nil {
		return nil, err
	}
	printInfo(fmt.Sprintf("Terragrunt downloaded to %s", terragruntDir))
	terraformDir, err := DownloadTerraform(binDir, "1.9.8", os, arch)
	if err != nil {
		return nil, err
	}
	printInfo(fmt.Sprintf("Terraform downloaded to %s", terraformDir))
	nodeDir, err := DownloadNode(titvoDir, "20.19.4", os, arch)
	if err != nil {
		return nil, err
	}
	printInfo(fmt.Sprintf("Node downloaded to %s", nodeDir))
	config = &InstallToolConfig{
		Dir:              binDir,
		OS:               os,
		Arch:             arch,
		TitvoDir:         titvoDir,
		TerraformBinDir:  terraformDir,
		NodeBinDir:       path.Join(nodeDir, "bin"),
		TerragruntBinDir: terragruntDir,
	}
	if err := LogConfiguredToolBinaries(*config); err != nil {
		return nil, err
	}
	return config, nil
}
