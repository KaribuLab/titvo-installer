package internal

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
)

// https://github.com/gruntwork-io/terragrunt/releases/download/v0.69.1/terragrunt_darwin_amd64
// https://github.com/gruntwork-io/terragrunt/releases/download/v0.69.1/terragrunt_windows_amd64.exe
// https://github.com/gruntwork-io/terragrunt/releases/download/v0.69.1/terragrunt_linux_amd64
const terragruntUrl = "https://github.com/gruntwork-io/terragrunt/releases/download/v%s/terragrunt_%s_%s"

// https://releases.hashicorp.com/terraform/1.13.0/terraform_1.13.0_darwin_amd64.zip
// https://releases.hashicorp.com/terraform/1.13.0/terraform_1.13.0_windows_amd64.zip
// https://releases.hashicorp.com/terraform/1.13.0/terraform_1.13.0_linux_amd64.zip
const terraformUrl = "https://releases.hashicorp.com/terraform/%s/terraform_%s_%s_%s.%s"

func downloadFile(url string, dir string, fileName string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	filePath := path.Join(dir, fileName)

	out, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

func extractZip(src, dest string) error {
	reader, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		path := filepath.Join(dest, file.Name)

		// Crear directorio si es necesario
		if file.FileInfo().IsDir() {
			os.MkdirAll(path, file.FileInfo().Mode())
			continue
		}

		// Crear el archivo
		fileReader, err := file.Open()
		if err != nil {
			return err
		}
		defer fileReader.Close()

		targetFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.FileInfo().Mode())
		if err != nil {
			return err
		}
		defer targetFile.Close()

		_, err = io.Copy(targetFile, fileReader)
		if err != nil {
			return err
		}
	}
	return nil
}

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
	Dir  string
	OS   OS
	Arch Arch
}

func InstallTools() error {
	// Get home directory
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	binDir := path.Join(home, ".titvo", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return err
	}
	os, err := GetOS()
	if err != nil {
		return err
	}
	arch, err := GetArch()
	if err != nil {
		return err
	}
	err = DownloadTerragrunt(binDir, "0.69.1", os, arch)
	if err != nil {
		return err
	}
	err = DownloadTerraform(binDir, "1.9.8", os, arch)
	if err != nil {
		return err
	}
	return nil
}
