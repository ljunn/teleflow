package updater

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/semver"
)

var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

type Options struct {
	Repository     string
	Current        string
	APIBaseURL     string
	ExecutablePath string
}

type Service struct {
	repository     string
	current        string
	apiBaseURL     string
	executablePath string
	client         *http.Client
	updateMu       sync.Mutex
}

type Release struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	Available      bool   `json:"available"`
	ReleaseURL     string `json:"releaseUrl,omitempty"`
	PublishedAt    string `json:"publishedAt,omitempty"`
	Notes          string `json:"notes,omitempty"`
	Configured     bool   `json:"configured"`
}

type UpdateResult struct {
	Release Release `json:"release"`
	Updated bool    `json:"updated"`
}

type githubRelease struct {
	TagName     string         `json:"tag_name"`
	HTMLURL     string         `json:"html_url"`
	Body        string         `json:"body"`
	PublishedAt time.Time      `json:"published_at"`
	Assets      []releaseAsset `json:"assets"`
}

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func New(options Options) *Service {
	return &Service{
		repository:     strings.TrimSpace(options.Repository),
		current:        normalizeVersion(options.Current),
		apiBaseURL:     strings.TrimRight(defaultString(options.APIBaseURL, "https://api.github.com"), "/"),
		executablePath: options.ExecutablePath,
		client:         &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *Service) Check(ctx context.Context) (Release, error) {
	release, err := s.fetchLatest(ctx)
	if err != nil {
		return Release{CurrentVersion: s.current, Configured: s.repository != ""}, err
	}
	return s.releaseInfo(release), nil
}

func (s *Service) Update(ctx context.Context) (UpdateResult, error) {
	if !s.updateMu.TryLock() {
		return UpdateResult{}, errors.New("an update is already in progress")
	}
	defer s.updateMu.Unlock()
	release, err := s.fetchLatest(ctx)
	if err != nil {
		return UpdateResult{}, err
	}
	info := s.releaseInfo(release)
	result := UpdateResult{Release: info}
	if !info.Available {
		return result, nil
	}
	if runtime.GOOS == "windows" {
		return result, errors.New("在线升级暂不支持 Windows，请下载新版本覆盖安装")
	}
	executablePath, err := s.executable()
	if err != nil {
		return result, err
	}
	archiveName := fmt.Sprintf("teleflow_%s_%s_%s.tar.gz", strings.TrimPrefix(info.LatestVersion, "v"), runtime.GOOS, runtime.GOARCH)
	archiveURL, checksumURL := "", ""
	for _, asset := range release.Assets {
		switch asset.Name {
		case archiveName:
			archiveURL = asset.BrowserDownloadURL
		case "checksums.txt":
			checksumURL = asset.BrowserDownloadURL
		}
	}
	if archiveURL == "" || checksumURL == "" {
		return result, fmt.Errorf("release %s does not contain %s and checksums.txt", info.LatestVersion, archiveName)
	}
	tempDir, err := os.MkdirTemp("", "teleflow-update-")
	if err != nil {
		return result, fmt.Errorf("create update directory: %w", err)
	}
	defer os.RemoveAll(tempDir)
	archivePath := filepath.Join(tempDir, archiveName)
	checksumsPath := filepath.Join(tempDir, "checksums.txt")
	if err := s.download(ctx, archiveURL, archivePath); err != nil {
		return result, fmt.Errorf("download release archive: %w", err)
	}
	if err := s.download(ctx, checksumURL, checksumsPath); err != nil {
		return result, fmt.Errorf("download release checksums: %w", err)
	}
	expected, err := checksumFor(checksumsPath, archiveName)
	if err != nil {
		return result, err
	}
	if err := verifySHA256(archivePath, expected); err != nil {
		return result, err
	}
	if err := installArchive(archivePath, executablePath); err != nil {
		return result, err
	}
	result.Updated = true
	return result, nil
}

func (s *Service) fetchLatest(ctx context.Context) (githubRelease, error) {
	result := githubRelease{}
	if s.repository == "" {
		return result, nil
	}
	if !repositoryPattern.MatchString(s.repository) {
		return result, errors.New("invalid GitHub repository; expected owner/name")
	}
	url := fmt.Sprintf("%s/repos/%s/releases/latest", s.apiBaseURL, s.repository)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return result, fmt.Errorf("create release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "teleflow-update-checker")
	response, err := s.client.Do(req)
	if err != nil {
		return result, fmt.Errorf("request latest release: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return result, fmt.Errorf("GitHub release API returned %s", response.Status)
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return result, fmt.Errorf("decode latest release: %w", err)
	}
	return result, nil
}

func (s *Service) releaseInfo(release githubRelease) Release {
	latest := normalizeVersion(release.TagName)
	result := Release{
		CurrentVersion: s.current,
		LatestVersion:  latest,
		ReleaseURL:     release.HTMLURL,
		Notes:          release.Body,
		Configured:     s.repository != "",
		Available:      semver.IsValid(latest) && semver.IsValid(s.current) && semver.Compare(latest, s.current) > 0,
	}
	if !release.PublishedAt.IsZero() {
		result.PublishedAt = release.PublishedAt.UTC().Format(time.RFC3339)
	}
	return result
}

func (s *Service) executable() (string, error) {
	if s.executablePath != "" {
		return s.executablePath, nil
	}
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve current executable: %w", err)
	}
	return filepath.Clean(path), nil
}

func (s *Service) download(ctx context.Context, url, destination string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	response, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %s", response.Status)
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, io.LimitReader(response.Body, 256<<20))
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func checksumFor(path, filename string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open checksums: %w", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && fields[len(fields)-1] == filename {
			return strings.ToLower(fields[0]), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read checksums: %w", err)
	}
	return "", fmt.Errorf("checksum for %s not found", filename)
}

func verifySHA256(path, expected string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open downloaded archive: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("hash downloaded archive: %w", err)
	}
	actual := fmt.Sprintf("%x", hash.Sum(nil))
	if !strings.EqualFold(actual, strings.TrimSpace(expected)) {
		return fmt.Errorf("archive checksum mismatch")
	}
	return nil
}

func installArchive(archivePath, executablePath string) error {
	archive, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open release archive: %w", err)
	}
	defer archive.Close()
	gzipReader, err := gzip.NewReader(archive)
	if err != nil {
		return fmt.Errorf("read release archive: %w", err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	tempFile, err := os.CreateTemp(filepath.Dir(executablePath), ".teleflow-new-")
	if err != nil {
		return fmt.Errorf("create replacement executable: %w", err)
	}
	tempPath := tempFile.Name()
	defer tempFile.Close()
	defer os.Remove(tempPath)
	found := false
	for {
		header, readErr := tarReader.Next()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return fmt.Errorf("read release archive entry: %w", readErr)
		}
		if header.Name != "teleflow" || header.Typeflag != tar.TypeReg {
			continue
		}
		if header.Size <= 0 || header.Size > 256<<20 {
			return errors.New("invalid teleflow executable in release archive")
		}
		if _, err := io.CopyN(tempFile, tarReader, header.Size); err != nil {
			return fmt.Errorf("extract teleflow executable: %w", err)
		}
		found = true
		break
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close replacement executable: %w", err)
	}
	if !found {
		return errors.New("release archive does not contain teleflow executable")
	}
	if err := os.Chmod(tempPath, 0o755); err != nil {
		return fmt.Errorf("set replacement executable permissions: %w", err)
	}
	if err := os.Rename(tempPath, executablePath); err != nil {
		return fmt.Errorf("replace current executable: %w", err)
	}
	return nil
}

func normalizeVersion(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "dev" {
		return value
	}
	if !strings.HasPrefix(value, "v") {
		return "v" + value
	}
	return value
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
