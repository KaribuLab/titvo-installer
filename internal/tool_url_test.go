package internal

import "testing"

func TestTerragruntDownloadURL(t *testing.T) {
	tests := []struct {
		osType OS
		arch   Arch
		want   string
	}{
		{
			osType: Windows,
			arch:   AMD64,
			want:   "https://github.com/gruntwork-io/terragrunt/releases/download/v0.69.1/terragrunt_windows_amd64.exe",
		},
		{
			osType: Linux,
			arch:   AMD64,
			want:   "https://github.com/gruntwork-io/terragrunt/releases/download/v0.69.1/terragrunt_linux_amd64",
		},
		{
			osType: Darwin,
			arch:   ARM64,
			want:   "https://github.com/gruntwork-io/terragrunt/releases/download/v0.69.1/terragrunt_darwin_arm64",
		},
	}

	for _, tt := range tests {
		got := terragruntDownloadURL("0.69.1", tt.osType, tt.arch)
		if got != tt.want {
			t.Fatalf("terragruntDownloadURL(%s, %s) = %q, want %q", tt.osType, tt.arch, got, tt.want)
		}
	}
}

func TestTerraformDownloadURL(t *testing.T) {
	got := terraformDownloadURL("1.9.8", Windows, AMD64)
	want := "https://releases.hashicorp.com/terraform/1.9.8/terraform_1.9.8_windows_amd64.zip"
	if got != want {
		t.Fatalf("terraformDownloadURL = %q, want %q", got, want)
	}
}

func TestNodeDownloadSpec_WindowsAMD64(t *testing.T) {
	url, folder, ext := nodeDownloadSpec("20.19.4", Windows, AMD64)
	if url != "https://nodejs.org/download/release/v20.19.4/node-v20.19.4-win-x64.zip" {
		t.Fatalf("url = %q", url)
	}
	if folder != "node-v20.19.4-win-x64" {
		t.Fatalf("folder = %q", folder)
	}
	if ext != "zip" {
		t.Fatalf("ext = %q", ext)
	}
}

func TestNodeDownloadSpec_WindowsARM64(t *testing.T) {
	url, folder, ext := nodeDownloadSpec("20.19.4", Windows, ARM64)
	if url != "https://nodejs.org/download/release/v20.19.4/node-v20.19.4-win-arm64.zip" {
		t.Fatalf("url = %q", url)
	}
	if folder != "node-v20.19.4-win-arm64" {
		t.Fatalf("folder = %q", folder)
	}
	if ext != "zip" {
		t.Fatalf("ext = %q", ext)
	}
}

func TestNodeBinDirFromInstallRoot(t *testing.T) {
	root := "/home/user/.titvo/node-v20.19.4-linux-x64"
	if got := NodeBinDirFromInstallRoot(root, Linux); got != root+"/bin" {
		t.Fatalf("Linux bin dir = %q", got)
	}
	winRoot := `C:\Users\me\.titvo\node-v20.19.4-win-x64`
	if got := NodeBinDirFromInstallRoot(winRoot, Windows); got != winRoot {
		t.Fatalf("Windows bin dir = %q, want install root", got)
	}
}
