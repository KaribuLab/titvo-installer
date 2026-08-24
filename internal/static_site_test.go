package internal

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// fakeStaticSiteDeployer is a test double for staticSiteDeployer so
// deployStaticAssets' orchestration logic (which files get uploaded, with
// what key/content-type, and that invalidation only runs after every upload
// succeeds) can be verified without real AWS credentials or a live S3
// bucket/CloudFront distribution — mirrors fakeAdminUserStore's role for
// seedAdminUser in admin_seeder_test.go.
type fakeStaticSiteDeployer struct {
	uploadErr      error
	invalidateErr  error
	uploads        []uploadCall
	invalidated    []string
	failUploadKey  string
	uploadsAtFail  int
	invalidateCall func()
}

type uploadCall struct {
	bucket      string
	key         string
	localPath   string
	contentType string
}

func (f *fakeStaticSiteDeployer) UploadFile(bucket, key, localPath, contentType string) error {
	if f.failUploadKey != "" && key == f.failUploadKey {
		return f.uploadErr
	}
	if f.uploadErr != nil && f.failUploadKey == "" {
		return f.uploadErr
	}
	f.uploads = append(f.uploads, uploadCall{bucket: bucket, key: key, localPath: localPath, contentType: contentType})
	return nil
}

func (f *fakeStaticSiteDeployer) InvalidateDistribution(distributionID string) error {
	f.invalidated = append(f.invalidated, distributionID)
	if f.invalidateCall != nil {
		f.invalidateCall()
	}
	return f.invalidateErr
}

func writeTestBuildDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"index.html":              "<html></html>",
		"assets/main.js":          "console.log('hi')",
		"assets/styles/style.css": "body{}",
	}
	for rel, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("failed creating dir for %s: %v", rel, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("failed writing %s: %v", rel, err)
		}
	}
	return dir
}

func TestDeployStaticAssetsUploadsEveryBuildFileThenInvalidates(t *testing.T) {
	buildDir := writeTestBuildDir(t)
	deployer := &fakeStaticSiteDeployer{}

	if err := deployStaticAssets(deployer, buildDir, "my-bucket", "DIST123"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(deployer.uploads) != 3 {
		t.Fatalf("expected 3 uploads, got %d", len(deployer.uploads))
	}

	keys := make([]string, len(deployer.uploads))
	for i, u := range deployer.uploads {
		keys[i] = u.key
		if u.bucket != "my-bucket" {
			t.Fatalf("expected bucket 'my-bucket', got %s", u.bucket)
		}
	}
	sort.Strings(keys)
	expectedKeys := []string{"assets/main.js", "assets/styles/style.css", "index.html"}
	for i, k := range expectedKeys {
		if keys[i] != k {
			t.Fatalf("expected key %s at position %d, got %v", k, i, keys)
		}
	}

	if len(deployer.invalidated) != 1 || deployer.invalidated[0] != "DIST123" {
		t.Fatalf("expected exactly one invalidation for DIST123, got %v", deployer.invalidated)
	}
}

func TestDeployStaticAssetsSetsContentTypeFromExtension(t *testing.T) {
	buildDir := writeTestBuildDir(t)
	deployer := &fakeStaticSiteDeployer{}

	if err := deployStaticAssets(deployer, buildDir, "my-bucket", "DIST123"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	byKey := map[string]uploadCall{}
	for _, u := range deployer.uploads {
		byKey[u.key] = u
	}
	if byKey["index.html"].contentType != "text/html; charset=utf-8" {
		t.Fatalf("unexpected content type for index.html: %s", byKey["index.html"].contentType)
	}
	if byKey["assets/main.js"].contentType == "" {
		t.Fatalf("expected a non-empty content type for assets/main.js")
	}
}

func TestDeployStaticAssetsAbortsInvalidationWhenAnUploadFails(t *testing.T) {
	buildDir := writeTestBuildDir(t)
	expectedErr := errors.New("s3 upload failed")
	deployer := &fakeStaticSiteDeployer{uploadErr: expectedErr}

	err := deployStaticAssets(deployer, buildDir, "my-bucket", "DIST123")
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected wrapped error %v, got %v", expectedErr, err)
	}
	if len(deployer.invalidated) != 0 {
		t.Fatalf("expected zero invalidations when an upload fails, got %v", deployer.invalidated)
	}
}

func TestDeployStaticAssetsPropagatesInvalidationError(t *testing.T) {
	buildDir := writeTestBuildDir(t)
	expectedErr := errors.New("cloudfront invalidation failed")
	deployer := &fakeStaticSiteDeployer{invalidateErr: expectedErr}

	err := deployStaticAssets(deployer, buildDir, "my-bucket", "DIST123")
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected wrapped error %v, got %v", expectedErr, err)
	}
	if len(deployer.uploads) != 3 {
		t.Fatalf("expected all 3 uploads to have run before the invalidation failure, got %d", len(deployer.uploads))
	}
}

func TestDeployStaticAssetsFailsOnEmptyBuildDir(t *testing.T) {
	emptyDir := t.TempDir()
	deployer := &fakeStaticSiteDeployer{}

	err := deployStaticAssets(deployer, emptyDir, "my-bucket", "DIST123")
	if err == nil {
		t.Fatal("expected an error for an empty build output directory")
	}
	if len(deployer.invalidated) != 0 {
		t.Fatalf("expected zero invalidations for an empty build dir, got %v", deployer.invalidated)
	}
}

func TestDeployStaticAssetsFailsOnMissingBuildDir(t *testing.T) {
	deployer := &fakeStaticSiteDeployer{}

	err := deployStaticAssets(deployer, filepath.Join(t.TempDir(), "does-not-exist"), "my-bucket", "DIST123")
	if err == nil {
		t.Fatal("expected an error for a missing build output directory")
	}
}
