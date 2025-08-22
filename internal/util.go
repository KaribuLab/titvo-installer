package internal

import (
	"archive/zip"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
)

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
