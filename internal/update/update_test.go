package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/aeciopires/updateGit/internal/config"
)

func sha(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// allowHTTP disables the HTTPS/host checks for tests with httptest (plain HTTP on 127.0.0.1).
func allowHTTP(t *testing.T) {
	t.Helper()
	original := requireHTTPS
	requireHTTPS = false
	t.Cleanup(func() { requireHTTPS = original })
}

func TestParseChecksum(t *testing.T) {
	a, b, c := sha("a"), sha("b"), sha("c")
	content := fmt.Sprintf("%s  bin/updateGit-linux-amd64\n%s  bin/updateGit-linux-arm64\n\n%s  bin/updateGit-darwin-arm64\nmalformed-line\nZZZ  bin/invalid\n",
		a, strings.ToUpper(b), c)

	tests := []struct {
		file    string
		want    string
		wantErr bool
	}{
		{"bin/updateGit-linux-amd64", a, false},
		{"bin/updateGit-linux-arm64", b, false}, // uppercase is normalized
		{"bin/updateGit-darwin-arm64", c, false},
		{"bin/updateGit-darwin-amd64", "", true},
		{"updateGit-linux-amd64", "", true}, // must match the full name
		{"bin/invalid", "", true},           // not a SHA256
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			got, err := ParseChecksum(content, tt.file)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseChecksum(%q) = %q, want %q", tt.file, got, tt.want)
			}
		})
	}
}

func TestParseChecksumSha256sumFormat(t *testing.T) {
	// Real output of "sha256sum bin/updateGit-* bin/updateGit.sbom.json" (Makefile)
	content := sha("darwin") + "  bin/updateGit-darwin-amd64\n" +
		sha("linux") + "  bin/updateGit-linux-amd64\n" +
		sha("sbom") + "  bin/updateGit.sbom.json\n"
	got, err := ParseChecksum(content, config.CLICheckSumBinDir+AssetName("linux", "amd64"))
	if err != nil || got != sha("linux") {
		t.Errorf("ParseChecksum = %q, %v", got, err)
	}
}

func TestVerifyChecksum(t *testing.T) {
	checksums := sha("binary") + "  bin/updateGit-linux-amd64\n"
	if err := VerifyChecksum([]byte("binary"), checksums, "bin/updateGit-linux-amd64"); err != nil {
		t.Errorf("valid checksum: %v", err)
	}
	if err := VerifyChecksum([]byte("tampered"), checksums, "bin/updateGit-linux-amd64"); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("tampered binary: %v", err)
	}
	if err := VerifyChecksum([]byte("binary"), checksums, "bin/other"); err == nil {
		t.Error("missing checksum must fail")
	}
}

func TestReleaseAssetNames(t *testing.T) {
	// Contract with the Makefile (GOLANG_SUPPORTED_PLATFORMS) and the release workflow
	want := map[[2]string]string{
		{"linux", "amd64"}:  "updateGit-linux-amd64",
		{"linux", "arm64"}:  "updateGit-linux-arm64",
		{"darwin", "amd64"}: "updateGit-darwin-amd64",
		{"darwin", "arm64"}: "updateGit-darwin-arm64",
	}
	for platform, name := range want {
		if got := AssetName(platform[0], platform[1]); got != name {
			t.Errorf("AssetName(%s/%s) = %q, want %q", platform[0], platform[1], got, name)
		}
	}
	if config.CLICheckSumBinDir != "bin/" {
		t.Errorf("CLICheckSumBinDir = %q, the checksums.txt uses the bin/ prefix", config.CLICheckSumBinDir)
	}

	release := &GitHubRelease{Assets: []GitHubReleaseAsset{
		{Name: "updateGit-darwin-arm64"}, {Name: "checksums.txt"}, {Name: "updateGit-linux-amd64"},
	}}
	binary, checksums := findAssets(release, "updateGit-linux-amd64")
	if binary == nil || binary.Name != "updateGit-linux-amd64" || checksums == nil {
		t.Errorf("findAssets = %+v, %+v", binary, checksums)
	}
	if binary, _ = findAssets(release, "updateGit-linux-riscv64"); binary != nil {
		t.Error("unexpected asset for an unknown platform")
	}
}

func TestDownloadFile(t *testing.T) {
	allowHTTP(t)
	var userAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgent = r.Header.Get("User-Agent")
		switch r.URL.Path {
		case "/ok":
			_, _ = w.Write([]byte("file content"))
		case "/big":
			_, _ = w.Write([]byte(strings.Repeat("x", 2048)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	ctx := context.Background()

	got, err := DownloadFile(ctx, server.URL+"/ok")
	if err != nil || string(got) != "file content" {
		t.Fatalf("DownloadFile = %q, %v", got, err)
	}
	if userAgent != "updateGit/"+config.CLIVersion {
		t.Errorf("User-Agent = %q", userAgent)
	}

	if _, err := DownloadFile(ctx, server.URL+"/missing"); err == nil {
		t.Error("DownloadFile should fail on HTTP 404")
	}
	if _, err := DownloadFile(ctx, "http://127.0.0.1:0/invalid"); err == nil {
		t.Error("DownloadFile should fail on connection error")
	}
	if _, err := DownloadFile(ctx, "://bad url"); err == nil {
		t.Error("DownloadFile should fail on an invalid URL")
	}

	// Maximum size
	original := maxDownloadSize
	maxDownloadSize = 1024
	t.Cleanup(func() { maxDownloadSize = original })
	if _, err := DownloadFile(ctx, server.URL+"/big"); err == nil || !strings.Contains(err.Error(), "larger than the limit") {
		t.Errorf("download larger than the limit: %v", err)
	}

	// Canceled context
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := DownloadFile(canceled, server.URL+"/ok"); err == nil {
		t.Error("DownloadFile should respect the context")
	}
}

func TestValidateURL(t *testing.T) {
	tests := map[string]bool{
		"https://github.com/aeciopires/updateGit/releases/download/0.2.0/checksums.txt": true,
		"https://objects.githubusercontent.com/x":                                       true,
		"https://release-assets.githubusercontent.com/x":                                true,
		"http://github.com/x":                   false, // not HTTPS
		"https://evil.example.com/updateGit":    false, // untrusted host
		"https://github.com.evil.example.com/x": false,
	}
	for raw, valid := range tests {
		u, _ := url.Parse(raw)
		if err := validateURL(u); (err == nil) != valid {
			t.Errorf("validateURL(%s) = %v, want valid=%v", raw, err, valid)
		}
	}
	if _, err := DownloadFile(context.Background(), "http://github.com/x"); err == nil {
		t.Error("DownloadFile must refuse plain HTTP")
	}

	// Redirects are checked too
	client := httpClient(apiTimeout)
	evil, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://evil.example.com/x", nil)
	if err := client.CheckRedirect(evil, nil); err == nil {
		t.Error("redirect to an untrusted host must be refused")
	}
	good, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://objects.githubusercontent.com/x", nil)
	if err := client.CheckRedirect(good, nil); err != nil {
		t.Errorf("redirect to the GitHub CDN: %v", err)
	}
	if err := client.CheckRedirect(good, make([]*http.Request, 10)); err == nil {
		t.Error("too many redirects must be refused")
	}
}

func TestLatestRelease(t *testing.T) {
	var accept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accept = r.Header.Get("Accept")
		switch r.URL.Path {
		case "/repos/aeciopires/updateGit/releases/latest":
			_, _ = w.Write([]byte(`{"tag_name":"0.3.0","assets":[{"name":"checksums.txt","browser_download_url":"https://github.com/x"}]}`))
		case "/repos/bad/json/releases/latest":
			_, _ = w.Write([]byte(`{not json`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	original := githubAPIURL
	githubAPIURL = server.URL
	t.Cleanup(func() { githubAPIURL = original })

	release, err := LatestRelease(context.Background(), "aeciopires/updateGit")
	if err != nil || release.TagName != "0.3.0" || len(release.Assets) != 1 {
		t.Fatalf("LatestRelease = %+v, %v", release, err)
	}
	if accept != "application/vnd.github+json" {
		t.Errorf("Accept = %q", accept)
	}
	if _, err := LatestRelease(context.Background(), "missing/repo"); err == nil {
		t.Error("HTTP 404 must fail")
	}
	if _, err := LatestRelease(context.Background(), "bad/json"); err == nil {
		t.Error("invalid JSON must fail")
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"0.2.0", "0.2.0", 0},
		{"0.2.1", "0.2.0", 1},
		{"0.2.0", "0.10.0", -1},
		{"1.0.0", "0.99.99", 1},
		{"1.0.0", "1.0.0-rc.1", 1},
		{"1.0.0-rc.1", "1.0.0", -1},
		{"1.0.0-rc.2", "1.0.0-rc.10", -1}, // numeric identifiers
		{"1.0.0-alpha", "1.0.0-alpha.1", -1},
		{"1.0.0-alpha.beta", "1.0.0-alpha.1", 1}, // alphanumeric > numeric
		{"1.0.0-beta", "1.0.0-alpha", 1},
		{"1.0.0-1", "1.0.0-alpha", -1},
	}
	for _, tt := range tests {
		got, err := CompareVersions(tt.a, tt.b)
		if err != nil || got != tt.want {
			t.Errorf("CompareVersions(%s, %s) = %d, %v; want %d", tt.a, tt.b, got, err, tt.want)
		}
	}
	if _, err := CompareVersions("v0.2.0", "0.2.0"); err == nil {
		t.Error("v prefix must be invalid")
	}
	if _, err := CompareVersions("0.2.0", "x"); err == nil {
		t.Error("invalid version must fail")
	}
}

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		latest, current string
		want            bool
	}{
		{"0.3.0", "0.2.0", true},
		{"0.2.0", "0.2.0", false},
		{"0.1.0", "0.2.0", false},     // never "update" to an older version
		{"0.2.0", "0.2.0-rc.1", true}, // final release after a pre-release
	}
	for _, tt := range tests {
		if got, err := IsNewerVersion(tt.latest, tt.current); err != nil || got != tt.want {
			t.Errorf("IsNewerVersion(%s, %s) = %v, %v", tt.latest, tt.current, got, err)
		}
	}
	if _, err := IsNewerVersion("latest", "0.2.0"); err == nil {
		t.Error("invalid tag must fail")
	}
}

func TestReplaceBinary(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "updateGit")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	oldPath, err := ReplaceBinary(exe, []byte("new"))
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "new" {
		t.Errorf("new binary = %q", got)
	}
	if got, _ := os.ReadFile(oldPath); oldPath != exe+".old" || string(got) != "old" {
		t.Errorf("old binary %s = %q", oldPath, got)
	}
	if info, _ := os.Stat(exe); info.Mode().Perm() != config.PermissionBinary {
		t.Errorf("permission = %v", info.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Errorf("temporary file left behind: %v", entries)
	}

	// Missing executable: error and no temporary file left
	if _, err := ReplaceBinary(filepath.Join(dir, "missing"), []byte("x")); err == nil {
		t.Error("renaming a missing executable must fail")
	}
	if entries, _ = os.ReadDir(dir); len(entries) != 2 {
		t.Errorf("temporary file left behind after an error: %v", entries)
	}
	// Directory that does not exist
	if _, err := ReplaceBinary(filepath.Join(dir, "no", "such", "exe"), []byte("x")); err == nil {
		t.Error("missing directory must fail")
	}
}

// releaseServer serves a release with the binary of the current platform.
func releaseServer(t *testing.T, binary, checksums string) (*httptest.Server, *GitHubRelease) {
	t.Helper()
	allowHTTP(t)
	asset := AssetName(runtime.GOOS, runtime.GOARCH)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + asset:
			_, _ = w.Write([]byte(binary))
		case "/checksums.txt":
			_, _ = w.Write([]byte(checksums))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server, &GitHubRelease{TagName: "9.9.9", Assets: []GitHubReleaseAsset{
		{Name: asset, DownloadURL: server.URL + "/" + asset},
		{Name: "checksums.txt", DownloadURL: server.URL + "/checksums.txt"},
	}}
}

func TestApplyUpdate(t *testing.T) {
	asset := AssetName(runtime.GOOS, runtime.GOARCH)
	newBinary := "#!/bin/sh\necho new\n"
	_, release := releaseServer(t, newBinary, sha(newBinary)+"  bin/"+asset+"\n")

	exe := filepath.Join(t.TempDir(), "updateGit")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldPath, err := ApplyUpdate(context.Background(), release, exe)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(exe); string(got) != newBinary {
		t.Errorf("binary not replaced: %q", got)
	}
	if got, _ := os.ReadFile(oldPath); string(got) != "old" {
		t.Errorf("old binary = %q", got)
	}
}

func TestApplyUpdateErrors(t *testing.T) {
	asset := AssetName(runtime.GOOS, runtime.GOARCH)
	exe := filepath.Join(t.TempDir(), "updateGit")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	assertUntouched := func(t *testing.T) {
		t.Helper()
		if got, _ := os.ReadFile(exe); string(got) != "old" {
			t.Errorf("the current binary must not change on errors: %q", got)
		}
	}

	t.Run("tampered binary", func(t *testing.T) {
		_, release := releaseServer(t, "tampered", sha("original")+"  bin/"+asset+"\n")
		if _, err := ApplyUpdate(context.Background(), release, exe); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
			t.Errorf("err = %v", err)
		}
		assertUntouched(t)
	})
	t.Run("missing checksum", func(t *testing.T) {
		_, release := releaseServer(t, "x", sha("x")+"  bin/other\n")
		if _, err := ApplyUpdate(context.Background(), release, exe); err == nil {
			t.Error("missing checksum must fail")
		}
		assertUntouched(t)
	})
	t.Run("missing assets", func(t *testing.T) {
		if _, err := ApplyUpdate(context.Background(), &GitHubRelease{}, exe); err == nil || !strings.Contains(err.Error(), "your platform") {
			t.Errorf("err = %v", err)
		}
		onlyBinary := &GitHubRelease{Assets: []GitHubReleaseAsset{{Name: asset}}}
		if _, err := ApplyUpdate(context.Background(), onlyBinary, exe); err == nil || !strings.Contains(err.Error(), "checksums.txt") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("download errors", func(t *testing.T) {
		server, release := releaseServer(t, "x", sha("x")+"  bin/"+asset+"\n")
		release.Assets[1].DownloadURL = server.URL + "/missing-checksums"
		if _, err := ApplyUpdate(context.Background(), release, exe); err == nil || !strings.Contains(err.Error(), "download checksums") {
			t.Errorf("err = %v", err)
		}
		_, release = releaseServer(t, "x", sha("x")+"  bin/"+asset+"\n")
		release.Assets[0].DownloadURL = server.URL + "/missing-binary"
		if _, err := ApplyUpdate(context.Background(), release, exe); err == nil || !strings.Contains(err.Error(), "download new binary") {
			t.Errorf("err = %v", err)
		}
		assertUntouched(t)
	})
}

func TestNewRequestInvalidURL(t *testing.T) {
	if _, err := newRequest(t.Context(), "http://bad host/"); err == nil {
		t.Error("invalid URL must fail")
	}
	allowHTTP(t)
	if _, err := LatestRelease(t.Context(), "x/y\x00"); err == nil {
		t.Error("invalid repository name must fail")
	}
	if sign(0) != 0 || sign(-5) != -1 || sign(5) != 1 {
		t.Error("sign")
	}
}
