// Package update have public and private functions to update the application.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/aeciopires/updateGit/internal/common"
	"github.com/aeciopires/updateGit/internal/config"
)

const (
	// apiTimeout limits the request to the GitHub API
	apiTimeout = 30 * time.Second
	// downloadTimeout limits each download (binary or checksums)
	downloadTimeout = 5 * time.Minute
	// checksumsAssetName is the name of the checksums file in the release
	checksumsAssetName = "checksums.txt"
)

var (
	// githubAPIURL is the base URL of the GitHub API (replaced in the tests)
	githubAPIURL = "https://api.github.com"
	// allowedDownloadHosts are the only hosts accepted for downloads and redirects.
	// GitHub serves the release assets from github.com and redirects to its CDN.
	allowedDownloadHosts = map[string]bool{
		"github.com":                            true,
		"objects.githubusercontent.com":         true,
		"release-assets.githubusercontent.com":  true,
		"github-releases.githubusercontent.com": true,
	}
	// requireHTTPS rejects plain HTTP URLs (disabled only in the tests)
	requireHTTPS = true
	// maxDownloadSize protects the memory against huge or malicious responses
	maxDownloadSize int64 = 256 << 20 // 256 MiB

	sha256Regex = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// GitHubReleaseAsset represents an asset in a GitHub release.
type GitHubReleaseAsset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
}

// GitHubRelease represents a GitHub release.
type GitHubRelease struct {
	TagName string               `json:"tag_name"`
	Assets  []GitHubReleaseAsset `json:"assets"`
}

// httpClient returns an HTTP client with timeout that only follows redirects to
// allowed HTTPS hosts.
func httpClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("too many redirects")
			}
			return validateURL(req.URL)
		},
	}
}

// validateURL accepts only HTTPS URLs of the allowed hosts.
func validateURL(u *url.URL) error {
	if requireHTTPS && u.Scheme != "https" {
		return fmt.Errorf("refusing non HTTPS URL %s", u.Redacted())
	}
	if requireHTTPS && !allowedDownloadHosts[strings.ToLower(u.Hostname())] {
		return fmt.Errorf("refusing download from untrusted host %s", u.Hostname())
	}
	return nil
}

// newRequest builds a GET request with the headers recommended by the GitHub API.
func newRequest(ctx context.Context, rawURL string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", config.CLIName+"/"+config.CLIVersion)
	return req, nil
}

// LatestRelease returns the latest (non pre-release) release of the repository.
func LatestRelease(ctx context.Context, repo string) (*GitHubRelease, error) {
	apiURL := fmt.Sprintf("%s/repos/%s/releases/latest", githubAPIURL, repo)
	common.Logger("debug", "Checking for updates at: %s", apiURL)

	req, err := newRequest(ctx, apiURL)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := httpClient(apiTimeout).Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch latest release from %s: %w", apiURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get latest release from %s: GitHub API returned status %s", apiURL, resp.Status)
	}

	var release GitHubRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxDownloadSize)).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to parse GitHub release JSON: %w", err)
	}
	return &release, nil
}

// ApplyUpdate downloads the binary of the current platform from the release,
// verifies its SHA256 checksum and replaces executablePath. It returns the path
// of the previous binary (executablePath + ".old").
func ApplyUpdate(ctx context.Context, release *GitHubRelease, executablePath string) (string, error) {
	// Determine the asset name based on OS and architecture
	assetName := AssetName(runtime.GOOS, runtime.GOARCH)
	common.Logger("debug", "Looking for asset: %s", assetName)

	binaryAsset, checksumsAsset := findAssets(release, assetName)
	if binaryAsset == nil {
		return "", fmt.Errorf("could not find a release asset for your platform (%s/%s)", runtime.GOOS, runtime.GOARCH)
	}
	if checksumsAsset == nil {
		return "", fmt.Errorf("could not find %s in the release assets", checksumsAssetName)
	}

	common.Logger("info", "Downloading checksums from %s...", checksumsAsset.DownloadURL)
	checksums, err := DownloadFile(ctx, checksumsAsset.DownloadURL)
	if err != nil {
		return "", fmt.Errorf("failed to download checksums: %w", err)
	}

	common.Logger("info", "Downloading new version from %s...", binaryAsset.DownloadURL)
	newBinaryBytes, err := DownloadFile(ctx, binaryAsset.DownloadURL)
	if err != nil {
		return "", fmt.Errorf("failed to download new binary: %w", err)
	}

	// Verify the checksum before touching the current binary
	expectedChecksumAssetName := config.CLICheckSumBinDir + assetName
	common.Logger("info", "Verifying checksum of %s file...", expectedChecksumAssetName)
	if err := VerifyChecksum(newBinaryBytes, string(checksums), expectedChecksumAssetName); err != nil {
		return "", err
	}
	common.Logger("info", "Checksum verified successfully.")

	return ReplaceBinary(executablePath, newBinaryBytes)
}

// findAssets returns the binary of the platform and the checksums file of the release.
func findAssets(release *GitHubRelease, assetName string) (binary, checksums *GitHubReleaseAsset) {
	for i, asset := range release.Assets {
		switch asset.Name {
		case assetName:
			binary = &release.Assets[i]
		case checksumsAssetName:
			checksums = &release.Assets[i]
		}
	}
	return binary, checksums
}

// VerifyChecksum compares the SHA256 of content with the value of fileName in checksums.
func VerifyChecksum(content []byte, checksums, fileName string) error {
	expected, err := ParseChecksum(checksums, fileName)
	if err != nil {
		return fmt.Errorf("failed to find checksum for asset %s: %w", fileName, err)
	}
	sum := sha256.Sum256(content)
	if actual := hex.EncodeToString(sum[:]); actual != expected {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expected, actual)
	}
	return nil
}

// ReplaceBinary writes content to a temporary file next to executablePath, makes it
// executable and replaces executablePath. The previous binary is kept as
// executablePath + ".old" and restored if the replacement fails.
func ReplaceBinary(executablePath string, content []byte) (oldPath string, err error) {
	tmpFile, err := os.CreateTemp(filepath.Dir(executablePath), "update-*.tmp")
	if err != nil {
		return "", fmt.Errorf("could not create temporary file for update: %w", err)
	}
	tmpName := tmpFile.Name()
	defer func() {
		_ = tmpFile.Close()
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err = tmpFile.Write(content); err != nil {
		return "", fmt.Errorf("failed to write new binary to temporary file: %w", err)
	}
	// Close the file so it can be renamed
	if err = tmpFile.Close(); err != nil {
		return "", fmt.Errorf("failed to close temporary file: %w", err)
	}
	if err = os.Chmod(tmpName, config.PermissionBinary); err != nil {
		return "", fmt.Errorf("failed to set executable permission on new binary: %w", err)
	}

	oldPath = executablePath + ".old"
	if err = os.Rename(executablePath, oldPath); err != nil {
		return "", fmt.Errorf("failed to rename old binary: %w", err)
	}
	if err = os.Rename(tmpName, executablePath); err != nil {
		// Attempt to restore the old binary if the final rename fails
		if restoreErr := os.Rename(oldPath, executablePath); restoreErr != nil {
			common.Logger("error", "Failed to restore the old binary from %s: %v", oldPath, restoreErr)
		}
		return "", fmt.Errorf("failed to move new binary into place: %w", err)
	}
	return oldPath, nil
}

// AssetName returns the name of the release asset of a platform (e.g. updateGit-linux-amd64).
// It is a contract with the release workflow and the Makefile: do not change it.
func AssetName(goos, goarch string) string {
	return fmt.Sprintf("%s-%s-%s", config.CLIName, goos, goarch)
}

// DownloadFile downloads a file from an allowed HTTPS URL, with timeout and a
// maximum size (maxDownloadSize).
func DownloadFile(ctx context.Context, rawURL string) ([]byte, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if err := validateURL(parsed); err != nil {
		return nil, err
	}

	req, err := newRequest(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient(downloadTimeout).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status: %s", resp.Status)
	}

	content, err := io.ReadAll(io.LimitReader(resp.Body, maxDownloadSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > maxDownloadSize {
		return nil, fmt.Errorf("download larger than the limit of %d bytes", maxDownloadSize)
	}
	return content, nil
}

// ParseChecksum finds the SHA256 checksum of a specific file in the checksums.txt
// content ("<sha256>  <file>" per line). Invalid checksums are rejected.
func ParseChecksum(checksumsContent, fileName string) (string, error) {
	for _, line := range strings.Split(checksumsContent, "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == fileName {
			checksum := strings.ToLower(parts[0])
			if !sha256Regex.MatchString(checksum) {
				return "", fmt.Errorf("invalid SHA256 checksum for %s: %q", fileName, parts[0])
			}
			return checksum, nil
		}
	}
	return "", fmt.Errorf("checksum for %s not found", fileName)
}

// semver is a parsed semantic version (without build metadata).
type semver struct {
	core       [3]int
	prerelease []string
}

func parseSemver(version string) (semver, error) {
	var v semver
	if !config.SemverRegex.MatchString(version) {
		return v, fmt.Errorf("invalid semantic version %q", version)
	}
	core, pre, _ := strings.Cut(version, "-")
	for i, part := range strings.Split(core, ".") {
		n, err := strconv.Atoi(part)
		if err != nil {
			return v, fmt.Errorf("invalid semantic version %q: %w", version, err)
		}
		v.core[i] = n
	}
	if pre != "" {
		v.prerelease = strings.Split(pre, ".")
	}
	return v, nil
}

// CompareVersions compares two semantic versions (https://semver.org/#spec-item-11):
// -1 if a < b, 0 if a == b, 1 if a > b.
func CompareVersions(a, b string) (int, error) {
	va, err := parseSemver(a)
	if err != nil {
		return 0, err
	}
	vb, err := parseSemver(b)
	if err != nil {
		return 0, err
	}
	for i := range va.core {
		if va.core[i] != vb.core[i] {
			return sign(va.core[i] - vb.core[i]), nil
		}
	}
	// A version without pre-release is greater than one with pre-release
	switch {
	case len(va.prerelease) == 0 && len(vb.prerelease) == 0:
		return 0, nil
	case len(va.prerelease) == 0:
		return 1, nil
	case len(vb.prerelease) == 0:
		return -1, nil
	}
	for i := 0; i < len(va.prerelease) && i < len(vb.prerelease); i++ {
		if c := compareIdentifier(va.prerelease[i], vb.prerelease[i]); c != 0 {
			return c, nil
		}
	}
	return sign(len(va.prerelease) - len(vb.prerelease)), nil
}

// compareIdentifier compares pre-release identifiers: numeric ones numerically and
// lower than alphanumeric ones, which are compared in ASCII order.
func compareIdentifier(a, b string) int {
	na, errA := strconv.Atoi(a)
	nb, errB := strconv.Atoi(b)
	switch {
	case errA == nil && errB == nil:
		return sign(na - nb)
	case errA == nil:
		return -1
	case errB == nil:
		return 1
	default:
		return strings.Compare(a, b)
	}
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	default:
		return 0
	}
}

// IsNewerVersion tells if latest is newer than current (a development build newer
// than the latest release is never "updated" to an older version).
func IsNewerVersion(latest, current string) (bool, error) {
	c, err := CompareVersions(latest, current)
	if err != nil {
		return false, err
	}
	return c > 0, nil
}
