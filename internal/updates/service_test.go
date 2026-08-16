package updates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestEmbeddedManifestAndVersionComparison(t *testing.T) {
	service, err := New("")
	if err != nil {
		t.Fatal(err)
	}
	status, err := service.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.CurrentVersion != "1.4.0" || status.UpdateAvailable || len(status.Changelog) == 0 {
		t.Fatalf("unexpected status: %+v", status)
	}
	if compareVersions("0.9.0", "0.8.9") <= 0 {
		t.Fatal("version comparison failed")
	}
}

func TestGitHubReleaseAPIAndInstallerSelection(t *testing.T) {
	apiURL, err := githubReleaseAPI(
		"https://raw.githubusercontent.com/brunooboy/bruno-browser/main/version.json",
		"1.4.0",
	)
	if err != nil {
		t.Fatal(err)
	}
	if apiURL != "https://api.github.com/repos/brunooboy/bruno-browser/releases/tags/v1.4.0" {
		t.Fatalf("unexpected release API URL %q", apiURL)
	}
	// Use a hexadecimal digest rather than relying on a live GitHub release.
	digest := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	asset, err := selectInstallerAsset([]githubReleaseAsset{
		{Name: "Bruno-Browser-Setup-amd64.exe", BrowserDownloadURL: "https://github.com/brunooboy/bruno-browser/releases/download/v1.4.0/amd64.exe", Digest: digest, Size: 20, State: "uploaded"},
		{Name: "Bruno.Browser.Setup.exe", BrowserDownloadURL: "https://github.com/brunooboy/bruno-browser/releases/download/v1.4.0/Bruno.Browser.Setup.exe", Digest: digest, Size: 30, State: "uploaded"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if asset.Name != "Bruno.Browser.Setup.exe" || asset.Size != 30 {
		t.Fatalf("unexpected installer asset: %#v", asset)
	}
}

func TestVerifiedDownloadChecksSizeAndSHA256(t *testing.T) {
	payload := []byte("signed installer fixture")
	digest := sha256.Sum256(payload)
	path := filepath.Join(t.TempDir(), "Bruno.Browser.Setup.exe")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := verifiedDownload(path, "1.4.0", Asset{
		Name: filepath.Base(path), Size: int64(len(payload)), SHA256: hex.EncodeToString(digest[:]),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Ready || result.Version != "1.4.0" || result.Bytes != int64(len(payload)) {
		t.Fatalf("unexpected verified download: %#v", result)
	}
	if _, err := verifiedDownload(path, "1.4.0", Asset{
		Name: filepath.Base(path), Size: int64(len(payload)), SHA256: "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
	}); err == nil {
		t.Fatal("expected a hash mismatch")
	}
}
