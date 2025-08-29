package internal

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
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

func extractTarGz(src, dest string) error {
	// Abrir el archivo
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	// Crear lector gzip
	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()

	// Crear lector tar
	tr := tar.NewReader(gzr)

	// Iterar sobre los archivos en el tar
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break // fin del archivo
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dest, header.Name)

		// Validar que el path esté dentro del directorio de destino (prevenir path traversal)
		cleanDest := filepath.Clean(dest)
		cleanTarget := filepath.Clean(target)
		rel, err := filepath.Rel(cleanDest, cleanTarget)
		if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("invalid file path: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			// Crear directorio
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			// Crear archivo regular
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			outFile, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
			// Establecer permisos del archivo
			if err := os.Chmod(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeSymlink:
			// Crear link simbólico
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			// Remover archivo existente si existe
			if _, err := os.Lstat(target); err == nil {
				if err := os.Remove(target); err != nil {
					return err
				}
			}
			// Crear el link simbólico
			if err := os.Symlink(header.Linkname, target); err != nil {
				return err
			}
		case tar.TypeLink:
			// Crear hard link
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			linkTarget := filepath.Join(dest, header.Linkname)
			// Remover archivo existente si existe
			if _, err := os.Lstat(target); err == nil {
				if err := os.Remove(target); err != nil {
					return err
				}
			}
			// Crear el hard link
			if err := os.Link(linkTarget, target); err != nil {
				return err
			}
		default:
			// Ignorar tipos no soportados en lugar de fallar
			fmt.Printf("Advertencia: tipo de archivo no soportado ignorado: %c en %s\n", header.Typeflag, header.Name)
		}
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
