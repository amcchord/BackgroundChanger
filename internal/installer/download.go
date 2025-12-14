package installer

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	// GitHubAPIURL is the GitHub API endpoint for latest release
	GitHubAPIURL = "https://api.github.com/repos/amcchord/BackgroundChanger/releases/latest"

	// ServiceExeName is the name of the service executable to download
	ServiceExeName = "bgStatusService.exe"
)

// GitHubRelease represents a GitHub release response
type GitHubRelease struct {
	TagName string        `json:"tag_name"`
	Name    string        `json:"name"`
	Assets  []GitHubAsset `json:"assets"`
}

// GitHubAsset represents a release asset
type GitHubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// GetLatestRelease fetches information about the latest release from GitHub
func GetLatestRelease() (*GitHubRelease, error) {
	req, err := http.NewRequest("GET", GitHubAPIURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "BgStatusService-Installer")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch release info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to parse release info: %w", err)
	}

	return &release, nil
}

// FindServiceAsset finds the bgStatusService.exe asset in a release
func FindServiceAsset(release *GitHubRelease) (*GitHubAsset, error) {
	for _, asset := range release.Assets {
		if strings.EqualFold(asset.Name, ServiceExeName) {
			return &asset, nil
		}
	}
	return nil, fmt.Errorf("could not find %s in release %s", ServiceExeName, release.TagName)
}

// DownloadProgress is a callback function for download progress updates
type DownloadProgress func(downloaded, total int64)

// DownloadFile downloads a file from a URL to a local path
func DownloadFile(url, destPath string, progress DownloadProgress) error {
	// Create the destination directory if needed
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Create a temporary file for downloading
	tempPath := destPath + ".tmp"
	out, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() {
		out.Close()
		// Clean up temp file on error
		if _, err := os.Stat(tempPath); err == nil {
			os.Remove(tempPath)
		}
	}()

	// Download the file
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "BgStatusService-Installer")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	// Create a progress reader if callback provided
	var reader io.Reader = resp.Body
	if progress != nil {
		reader = &progressReader{
			reader:   resp.Body,
			total:    resp.ContentLength,
			callback: progress,
		}
	}

	// Copy the data
	_, err = io.Copy(out, reader)
	if err != nil {
		return fmt.Errorf("failed to save file: %w", err)
	}

	// Close the file before renaming
	out.Close()

	// Move temp file to final destination
	if err := os.Rename(tempPath, destPath); err != nil {
		return fmt.Errorf("failed to finalize download: %w", err)
	}

	return nil
}

// progressReader wraps an io.Reader to report progress
type progressReader struct {
	reader     io.Reader
	total      int64
	downloaded int64
	callback   DownloadProgress
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.downloaded += int64(n)
	if pr.callback != nil {
		pr.callback(pr.downloaded, pr.total)
	}
	return n, err
}

// DownloadLatestService downloads the latest bgStatusService.exe to a temporary location
// and returns the path to the downloaded file along with version info
func DownloadLatestService() (filePath string, version string, err error) {
	// Get latest release info
	release, err := GetLatestRelease()
	if err != nil {
		return "", "", fmt.Errorf("failed to get release info: %w", err)
	}

	// Find the service executable asset
	asset, err := FindServiceAsset(release)
	if err != nil {
		return "", "", err
	}

	// Download to temp directory
	tempDir := os.TempDir()
	destPath := filepath.Join(tempDir, ServiceExeName)

	// Show a simple progress message (we can't do a real progress bar with MessageBox)
	err = DownloadFile(asset.BrowserDownloadURL, destPath, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to download: %w", err)
	}

	return destPath, release.TagName, nil
}

