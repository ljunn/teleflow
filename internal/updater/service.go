package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

type Options struct {
	Repository string
	Current    string
}

type Service struct {
	repository string
	current    string
	client     *http.Client
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

type githubRelease struct {
	TagName     string    `json:"tag_name"`
	HTMLURL     string    `json:"html_url"`
	Body        string    `json:"body"`
	PublishedAt time.Time `json:"published_at"`
}

func New(options Options) *Service {
	return &Service{
		repository: strings.TrimSpace(options.Repository),
		current:    normalizeVersion(options.Current),
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *Service) Check(ctx context.Context) (Release, error) {
	result := Release{CurrentVersion: s.current, Configured: s.repository != ""}
	if s.repository == "" {
		return result, nil
	}
	if !repositoryPattern.MatchString(s.repository) {
		return result, errors.New("invalid GitHub repository; expected owner/name")
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", s.repository)
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

	var release githubRelease
	if err := json.NewDecoder(response.Body).Decode(&release); err != nil {
		return result, fmt.Errorf("decode latest release: %w", err)
	}
	latest := normalizeVersion(release.TagName)
	result.LatestVersion = latest
	result.ReleaseURL = release.HTMLURL
	result.PublishedAt = release.PublishedAt.UTC().Format(time.RFC3339)
	result.Notes = release.Body
	result.Available = semver.IsValid(latest) && semver.IsValid(s.current) && semver.Compare(latest, s.current) > 0
	return result, nil
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
