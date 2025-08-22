package internal

import (
	"fmt"
	"os"
	"path"
)

// https://github.com/gruntwork-io/terragrunt/releases/download/v0.69.1/terragrunt_darwin_amd64
// https://github.com/gruntwork-io/terragrunt/releases/download/v0.69.1/terragrunt_windows_amd64.exe
// https://github.com/gruntwork-io/terragrunt/releases/download/v0.69.1/terragrunt_linux_amd64
const terragruntUrl = "https://github.com/gruntwork-io/terragrunt/releases/download/v%s/terragrunt_%s_%s"

// https://releases.hashicorp.com/terraform/1.13.0/terraform_1.13.0_darwin_amd64.zip
// https://releases.hashicorp.com/terraform/1.13.0/terraform_1.13.0_windows_amd64.zip
// https://releases.hashicorp.com/terraform/1.13.0/terraform_1.13.0_linux_amd64.zip
const terraformUrl = "https://releases.hashicorp.com/terraform/%s/terraform_%s_%s_%s.%s"

func DownloadTerragrunt(dir string, version string, osType OS, arch Arch) error {
	url := fmt.Sprintf(terragruntUrl, version, osType, arch)
	fmt.Println("Downloading Terragrunt")
	fmt.Println(url)
	fileExtension := ""
	if osType == Windows {
		fileExtension = ".exe"
	}
	fileName := fmt.Sprintf("terragrunt%s", fileExtension)
	err := downloadFile(url, dir, fileName)
	if err != nil {
		return err
	}
	if osType != Windows {
		// Give execute permission to the file
		err = os.Chmod(path.Join(dir, fileName), 0755)
		if err != nil {
			return err
		}
	}
	output, err := ExecuteWithOptions("terragrunt", &ExecuteOptions{
		WorkingDir: dir,
	}, "--version")
	if err != nil {
		return err
	}
	fmt.Printf("%s", output)
	return nil
}

func DownloadTerraform(dir string, version string, osType OS, arch Arch) error {
	url := fmt.Sprintf(terraformUrl, version, version, osType, arch, "zip")
	fmt.Println("Downloading Terraform")
	fmt.Println(url)
	zipFileName := "terraform.zip"
	err := downloadFile(url, dir, zipFileName)
	if err != nil {
		return err
	}

	// Extraer el ZIP
	zipPath := path.Join(dir, zipFileName)
	err = extractZip(zipPath, dir)
	if err != nil {
		return err
	}

	output, err := ExecuteWithOptions("terraform", &ExecuteOptions{
		WorkingDir: dir,
	}, "--version")
	if err != nil {
		return err
	}
	fmt.Printf("%s", output)

	// Eliminar el archivo ZIP después de extraer
	return os.Remove(zipPath)
}

type InstallToolConfig struct {
	Dir      string
	OS       OS
	Arch     Arch
	TitvoDir string
}

func InstallTools() (error, InstallToolConfig) {
	// Get home directory
	home, err := os.UserHomeDir()
	if err != nil {
		return err, InstallToolConfig{}
	}
	titvoDir := path.Join(home, ".titvo")
	binDir := path.Join(titvoDir, "bin")
	fmt.Printf("Installing Tools in %s\n", binDir)
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return err, InstallToolConfig{}
	}
	os, err := GetOS()
	if err != nil {
		return err, InstallToolConfig{}
	}
	arch, err := GetArch()
	if err != nil {
		return err, InstallToolConfig{}
	}
	err = DownloadTerragrunt(binDir, "0.69.1", os, arch)
	if err != nil {
		return err, InstallToolConfig{}
	}
	err = DownloadTerraform(binDir, "1.9.8", os, arch)
	if err != nil {
		return err, InstallToolConfig{}
	}
	return nil, InstallToolConfig{
		Dir:      binDir,
		OS:       os,
		Arch:     arch,
		TitvoDir: titvoDir,
	}
}
