package internal

import (
	"fmt"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
)

// staticSiteDeployer abstracts the AWS operations needed to publish a built
// static site: uploading each build artifact to its bucket key and
// invalidating the CloudFront distribution that serves it. Abstracting this
// (instead of calling the AWS SDK directly from deployStaticAssets) lets the
// walk/key-mapping/ordering logic be tested with a fake, the same way
// seedAdminUser is tested against fakeAdminUserStore in admin_seeder_test.go.
type staticSiteDeployer interface {
	UploadFile(bucket, key, localPath, contentType string) error
	InvalidateDistribution(distributionID string) error
}

// deployStaticAssets uploads every file under buildDir to bucketName (the
// object key is the path relative to buildDir, always using forward
// slashes regardless of OS) and, only once every upload has succeeded,
// invalidates distributionID so CloudFront stops serving the previous
// version. Upload happens strictly before invalidation so a mid-sync
// failure never invalidates a distribution pointing at a half-updated S3
// bucket.
func deployStaticAssets(deployer staticSiteDeployer, buildDir, bucketName, distributionID string) error {
	entries, err := collectBuildFiles(buildDir)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return fmt.Errorf("build output directory %s has no files to publish", buildDir)
	}

	for _, entry := range entries {
		if err := deployer.UploadFile(bucketName, entry.key, entry.localPath, entry.contentType); err != nil {
			return fmt.Errorf("failed to upload %s: %w", entry.key, err)
		}
	}

	if err := deployer.InvalidateDistribution(distributionID); err != nil {
		return fmt.Errorf("failed to invalidate distribution %s: %w", distributionID, err)
	}
	return nil
}

// buildFileEntry is one file discovered under a build output directory,
// ready to be uploaded.
type buildFileEntry struct {
	key         string
	localPath   string
	contentType string
}

var walkBuildDirFn = filepath.WalkDir

// collectBuildFiles walks buildDir and returns every regular file found,
// with an S3 key (forward-slash relative path) and a best-effort content
// type derived from the file extension.
func collectBuildFiles(buildDir string) ([]buildFileEntry, error) {
	var entries []buildFileEntry
	err := walkBuildDirFn(buildDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(buildDir, p)
		if err != nil {
			return err
		}
		contentType := mime.TypeByExtension(filepath.Ext(p))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		entries = append(entries, buildFileEntry{
			key:         filepath.ToSlash(rel),
			localPath:   p,
			contentType: contentType,
		})
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("build output directory %s does not exist", buildDir)
		}
		return nil, err
	}
	return entries, nil
}
