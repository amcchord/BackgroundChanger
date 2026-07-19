package loginscreen

import (
	"context"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/amcchord/WallpaperIdentity/v4/internal/paths"
	"golang.org/x/sys/windows/registry"
)

const personalizationCSPRegistry = `SOFTWARE\Microsoft\Windows\CurrentVersion\PersonalizationCSP`

// CurrentBackgroundImage describes the readable image Windows is currently
// configured to use for the machine lock/sign-in surface.
type CurrentBackgroundImage struct {
	Path   string
	Source string
}

type currentBackgroundCandidate struct {
	reference string
	source    string
}

// FindCurrentBackgroundImage resolves documented machine policy/CSP settings,
// the current user's Windows lock-screen URI, and finally Windows' stock lock
// screen. W:ID-owned output is deliberately ignored to prevent recursive
// overlays during a retry or repair.
func FindCurrentBackgroundImage() (CurrentBackgroundImage, bool) {
	candidates := registryBackgroundCandidates()
	if current, found := chooseCurrentBackground(candidates); found {
		return current, true
	}
	candidates = candidates[:0]
	if reference := currentUserLockScreenReference(); reference != "" {
		candidates = append(candidates, currentBackgroundCandidate{reference: reference, source: "Windows lock screen"})
	}
	systemRoot := os.Getenv("SystemRoot")
	if systemRoot == "" {
		systemRoot = `C:\Windows`
	}
	candidates = append(candidates, currentBackgroundCandidate{
		reference: filepath.Join(systemRoot, "Web", "Screen", "img100.jpg"),
		source:    "Windows default",
	})
	return chooseCurrentBackground(candidates)
}

func registryBackgroundCandidates() []currentBackgroundCandidate {
	result := make([]currentBackgroundCandidate, 0, 3)
	if key, err := registry.OpenKey(registry.LOCAL_MACHINE, personalizationPolicy, registry.QUERY_VALUE); err == nil {
		if value, _, valueErr := key.GetStringValue("LockScreenImage"); valueErr == nil {
			result = append(result, currentBackgroundCandidate{reference: value, source: "Group Policy"})
		}
		key.Close()
	}
	if key, err := registry.OpenKey(registry.LOCAL_MACHINE, personalizationCSPRegistry, registry.QUERY_VALUE); err == nil {
		status, _, statusErr := key.GetIntegerValue("LockScreenImageStatus")
		if statusErr == nil && status == 1 {
			imageURL, _, _ := key.GetStringValue("LockScreenImageUrl")
			urlPath := localBackgroundPath(imageURL)
			if urlPath == "" || !isWIDOwnedBackground(urlPath) {
				if value, _, valueErr := key.GetStringValue("LockScreenImagePath"); valueErr == nil {
					result = append(result, currentBackgroundCandidate{reference: value, source: "Personalization CSP"})
				}
				if imageURL != "" {
					result = append(result, currentBackgroundCandidate{reference: imageURL, source: "Personalization CSP"})
				}
			}
		}
		key.Close()
	}
	return result
}

func currentUserLockScreenReference() string {
	script := `Add-Type -AssemblyName System.Runtime.WindowsRuntime
$uri=[Windows.System.UserProfile.LockScreen,Windows.System.UserProfile,ContentType=WindowsRuntime]::OriginalImageFile
if ($null -ne $uri) { [Console]::Out.Write($uri.AbsoluteUri) }`
	powershell := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	if _, err := os.Stat(powershell); err != nil {
		powershell = "powershell.exe"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, powershell, "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", encodePowerShell(script))
	cmd.SysProcAttr = hiddenProcessAttributes()
	output, err := cmd.Output()
	if err != nil || ctx.Err() != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func chooseCurrentBackground(candidates []currentBackgroundCandidate) (CurrentBackgroundImage, bool) {
	for _, candidate := range candidates {
		path := localBackgroundPath(candidate.reference)
		if path == "" || isWIDOwnedBackground(path) || !isReadableBackgroundImage(path) {
			continue
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		return CurrentBackgroundImage{Path: absolute, Source: candidate.source}, true
	}
	return CurrentBackgroundImage{}, false
}

func localBackgroundPath(reference string) string {
	reference = strings.Trim(strings.TrimSpace(reference), `"`)
	if reference == "" {
		return ""
	}
	reference = expandPercentEnvironment(reference)
	if filepath.IsAbs(reference) {
		return filepath.Clean(reference)
	}
	parsed, err := url.Parse(reference)
	if err == nil && parsed.Scheme != "" {
		if !strings.EqualFold(parsed.Scheme, "file") {
			return ""
		}
		decoded, decodeErr := url.PathUnescape(parsed.EscapedPath())
		if decodeErr != nil {
			return ""
		}
		if parsed.Host != "" {
			return `\\` + parsed.Host + filepath.FromSlash(decoded)
		}
		if len(decoded) >= 3 && decoded[0] == '/' && decoded[2] == ':' {
			decoded = decoded[1:]
		}
		return filepath.FromSlash(decoded)
	}
	return ""
}

func expandPercentEnvironment(value string) string {
	for cursor := 0; cursor < len(value); {
		startOffset := strings.IndexByte(value[cursor:], '%')
		if startOffset < 0 {
			break
		}
		start := cursor + startOffset
		endOffset := strings.IndexByte(value[start+1:], '%')
		if endOffset < 0 {
			break
		}
		end := start + 1 + endOffset
		name := value[start+1 : end]
		if replacement, ok := os.LookupEnv(name); ok {
			value = value[:start] + replacement + value[end+1:]
			cursor = start + len(replacement)
		} else {
			cursor = end + 1
		}
	}
	return value
}

func isReadableBackgroundImage(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	_, format, err := image.DecodeConfig(file)
	return err == nil && (format == "jpeg" || format == "png")
}

func isWIDOwnedBackground(path string) bool {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		programData = `C:\ProgramData`
	}
	for _, root := range []string{
		paths.DataDir(), paths.LegacyDataDir(), paths.CSPImageDir(), filepath.Join(programData, "BgStatusService"),
	} {
		if isOwnedPath(path, root) {
			return true
		}
	}
	return false
}
