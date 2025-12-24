package installer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Default timeouts for network operations
const (
	// HTTPConnectTimeout is the timeout for establishing a connection
	HTTPConnectTimeout = 30 * time.Second
	// HTTPRequestTimeout is the overall timeout for a request (including download)
	HTTPRequestTimeout = 5 * time.Minute
	// HTTPAPITimeout is the timeout for API calls (short responses)
	HTTPAPITimeout = 30 * time.Second
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
	return GetLatestReleaseWithContext(context.Background())
}

// GetLatestReleaseWithContext fetches release info with context for cancellation/timeout
func GetLatestReleaseWithContext(ctx context.Context) (*GitHubRelease, error) {
	// Use a timeout context if none provided
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, HTTPAPITimeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, "GET", GitHubAPIURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "BgStatusService-Installer")

	client := &http.Client{
		Timeout: HTTPAPITimeout,
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("connection timed out after %v - check your internet connection", HTTPAPITimeout)
		}
		if ctx.Err() == context.Canceled {
			return nil, fmt.Errorf("operation cancelled")
		}
		return nil, fmt.Errorf("failed to connect to GitHub: %w (check your internet connection)", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("no releases found on GitHub repository")
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("GitHub rate limit exceeded - please try again later")
	}
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
	return DownloadFileWithContext(context.Background(), url, destPath, progress)
}

// DownloadFileWithContext downloads a file with context for cancellation/timeout
func DownloadFileWithContext(ctx context.Context, url, destPath string, progress DownloadProgress) error {
	// Use a timeout context if none provided
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, HTTPRequestTimeout)
		defer cancel()
	}

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

	// Download the file with context
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "BgStatusService-Installer")

	// Use client with connection timeout (overall timeout handled by context)
	client := &http.Client{
		Timeout: HTTPRequestTimeout,
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("download timed out - check your internet connection")
		}
		if ctx.Err() == context.Canceled {
			return fmt.Errorf("download cancelled")
		}
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
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("download timed out - check your internet connection")
		}
		if ctx.Err() == context.Canceled {
			return fmt.Errorf("download cancelled")
		}
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

// DownloadStatusCallback is called with status updates during download
type DownloadStatusCallback func(status string, progressPercent int)

// DownloadLatestServiceWithProgress downloads the latest version with progress updates
func DownloadLatestServiceWithProgress(statusCallback DownloadStatusCallback) (filePath string, version string, err error) {
	// Get latest release info
	statusCallback("Connecting to GitHub...\nFetching release information", 30)
	
	release, err := GetLatestRelease()
	if err != nil {
		// Provide more helpful error messages
		errStr := err.Error()
		if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "timed out") {
			return "", "", fmt.Errorf("connection timed out\n\nPlease check your internet connection and try again")
		}
		if strings.Contains(errStr, "no such host") || strings.Contains(errStr, "DNS") {
			return "", "", fmt.Errorf("cannot resolve github.com\n\nPlease check your DNS settings and internet connection")
		}
		if strings.Contains(errStr, "connection refused") {
			return "", "", fmt.Errorf("connection refused\n\nA firewall may be blocking access to GitHub")
		}
		if strings.Contains(errStr, "rate limit") {
			return "", "", fmt.Errorf("GitHub rate limit exceeded\n\nPlease wait a few minutes and try again")
		}
		return "", "", fmt.Errorf("failed to get release info:\n%w", err)
	}

	// Find the service executable asset
	asset, err := FindServiceAsset(release)
	if err != nil {
		return "", "", err
	}

	// Download to temp directory
	tempDir := os.TempDir()
	destPath := filepath.Join(tempDir, ServiceExeName)

	// Format the download URL for display (shorten it)
	shortURL := asset.BrowserDownloadURL
	if len(shortURL) > 60 {
		shortURL = shortURL[:57] + "..."
	}

	// Track download progress with speed calculation
	startTime := time.Now()
	lastUpdate := startTime
	lastBytes := int64(0)

	progressCallback := func(downloaded, total int64) {
		now := time.Now()
		elapsed := now.Sub(lastUpdate)
		
		// Update at most every 100ms to avoid UI flicker
		if elapsed < 100*time.Millisecond {
			return
		}

		// Calculate speed
		bytesSinceLastUpdate := downloaded - lastBytes
		speed := float64(bytesSinceLastUpdate) / elapsed.Seconds()
		lastUpdate = now
		lastBytes = downloaded

		// Format sizes and speed
		downloadedMB := float64(downloaded) / (1024 * 1024)
		totalMB := float64(total) / (1024 * 1024)
		speedKBs := speed / 1024

		var speedStr string
		if speedKBs >= 1024 {
			speedStr = fmt.Sprintf("%.1f MB/s", speedKBs/1024)
		} else {
			speedStr = fmt.Sprintf("%.0f KB/s", speedKBs)
		}

		// Calculate progress percentage (40-65 range for download phase)
		percent := 40
		if total > 0 {
			percent = 40 + int(float64(downloaded)/float64(total)*25)
		}

		status := fmt.Sprintf("Downloading %s\n%.1f / %.1f MB (%s)", 
			shortURL, downloadedMB, totalMB, speedStr)
		statusCallback(status, percent)
	}

	statusCallback(fmt.Sprintf("Downloading from:\n%s", shortURL), 40)
	
	err = DownloadFile(asset.BrowserDownloadURL, destPath, progressCallback)
	if err != nil {
		// Provide more helpful error messages
		errStr := err.Error()
		if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "timed out") {
			return "", "", fmt.Errorf("download timed out\n\nThe connection was too slow or interrupted.\nPlease try again")
		}
		if strings.Contains(errStr, "cancelled") {
			return "", "", fmt.Errorf("download was cancelled")
		}
		return "", "", fmt.Errorf("download failed:\n%w", err)
	}

	statusCallback("Download complete, verifying...", 65)
	return destPath, release.TagName, nil
}


