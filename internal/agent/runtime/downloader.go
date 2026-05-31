package runtime

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 5 * time.Minute}

// BinaryInfo describes an installed binary.
type BinaryInfo struct {
	Version string
	Path    string
}

// InstallXray downloads and installs xray-core from GitHub releases.
// If version is empty, installs the latest release.
func InstallXray(version, destPath string) (*BinaryInfo, error) {
	if version == "" {
		var err error
		version, err = latestGitHubRelease("XTLS/Xray-core")
		if err != nil {
			return nil, fmt.Errorf("fetch latest xray version: %w", err)
		}
	}

	arch := goArchToXray(runtime.GOARCH)
	os_ := strings.Title(runtime.GOOS) //nolint:staticcheck
	assetName := fmt.Sprintf("Xray-%s-%s.zip", os_, arch)
	url := fmt.Sprintf("https://github.com/XTLS/Xray-core/releases/download/%s/%s", version, assetName)

	tmp, err := downloadToTemp(url)
	if err != nil {
		return nil, fmt.Errorf("download xray: %w", err)
	}
	defer os.Remove(tmp)

	if err := extractFromZip(tmp, "xray", destPath); err != nil {
		return nil, fmt.Errorf("extract xray: %w", err)
	}
	if err := os.Chmod(destPath, 0755); err != nil {
		return nil, err
	}
	return &BinaryInfo{Version: version, Path: destPath}, nil
}

// InstallHysteria2 downloads and installs hysteria2 from GitHub releases.
func InstallHysteria2(version, destPath string) (*BinaryInfo, error) {
	if version == "" {
		var err error
		version, err = latestGitHubRelease("apernet/hysteria")
		if err != nil {
			return nil, fmt.Errorf("fetch latest hysteria2 version: %w", err)
		}
	}

	arch := goArchToHysteria(runtime.GOARCH)
	assetName := fmt.Sprintf("hysteria-%s-%s", runtime.GOOS, arch)
	url := fmt.Sprintf("https://github.com/apernet/hysteria/releases/download/%s/%s", version, assetName)

	tmp, err := downloadToTemp(url)
	if err != nil {
		return nil, fmt.Errorf("download hysteria2: %w", err)
	}
	defer os.Remove(tmp)

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, destPath); err != nil {
		// Rename fails across filesystems; fall back to copy
		if err2 := copyFile(tmp, destPath); err2 != nil {
			return nil, fmt.Errorf("install hysteria2: %w", err2)
		}
	}
	if err := os.Chmod(destPath, 0755); err != nil {
		return nil, err
	}
	return &BinaryInfo{Version: version, Path: destPath}, nil
}

// latestGitHubRelease returns the tag_name of the latest release.
func latestGitHubRelease(repo string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", err
	}
	if rel.TagName == "" {
		return "", fmt.Errorf("empty tag_name from GitHub API")
	}
	return rel.TagName, nil
}

func downloadToTemp(url string) (string, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	tmp, err := os.CreateTemp("", "subforge-download-*")
	if err != nil {
		return "", err
	}
	defer tmp.Close()

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

// extractFromZip extracts a named file from a zip archive to destPath.
func extractFromZip(zipPath, fileName, destPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if filepath.Base(f.Name) != fileName {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}
		out, err := os.Create(destPath)
		if err != nil {
			return err
		}
		defer out.Close()

		_, err = io.Copy(out, rc)
		return err
	}
	return fmt.Errorf("file %q not found in archive", fileName)
}

// extractFromTarGz extracts a named file from a .tar.gz to destPath.
func extractFromTarGz(tgzPath, fileName, destPath string) error {
	f, err := os.Open(tgzPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if filepath.Base(hdr.Name) != fileName {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}
		out, err := os.Create(destPath)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, tr)
		return err
	}
	return fmt.Errorf("file %q not found in archive", fileName)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func goArchToXray(arch string) string {
	switch arch {
	case "amd64":
		return "64"
	case "arm64":
		return "arm64-v8a"
	case "arm":
		return "arm32-v7a"
	default:
		return arch
	}
}

func goArchToHysteria(arch string) string {
	switch arch {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	default:
		return arch
	}
}
