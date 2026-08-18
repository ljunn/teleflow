package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestCheckWithoutRepository(t *testing.T) {
	service := New(Options{Current: "1.2.3"})
	release, err := service.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if release.Configured {
		t.Fatal("empty repository must not be configured")
	}
	if release.CurrentVersion != "v1.2.3" {
		t.Fatalf("unexpected current version: %s", release.CurrentVersion)
	}
}

func TestCheckRejectsInvalidRepository(t *testing.T) {
	service := New(Options{Repository: "https://github.com/example/project", Current: "v1.0.0"})
	if _, err := service.Check(context.Background()); err == nil {
		t.Fatal("expected invalid repository error")
	}
}

func TestUpdateDownloadsVerifiesAndReplacesExecutable(t *testing.T) {
	archive := testArchive(t, []byte("new-binary"))
	archiveHash := sha256.Sum256(archive)
	archiveName := fmt.Sprintf("teleflow_1.1.0_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/example/teleflow/releases/latest":
			_ = json.NewEncoder(w).Encode(githubRelease{
				TagName:     "v1.1.0",
				HTMLURL:     "https://example.test/releases/v1.1.0",
				PublishedAt: time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC),
				Assets: []releaseAsset{
					{Name: archiveName, BrowserDownloadURL: serverURL + "/download/archive"},
					{Name: "checksums.txt", BrowserDownloadURL: serverURL + "/download/checksums"},
				},
			})
		case "/download/archive":
			_, _ = w.Write(archive)
		case "/download/checksums":
			_, _ = fmt.Fprintf(w, "%x  %s\n", archiveHash, archiveName)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	executablePath := filepath.Join(t.TempDir(), "teleflow")
	if err := os.WriteFile(executablePath, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	service := New(Options{
		Repository:     "example/teleflow",
		Current:        "v1.0.0",
		APIBaseURL:     server.URL,
		ExecutablePath: executablePath,
	})
	result, err := service.Update(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated || result.Release.LatestVersion != "v1.1.0" {
		t.Fatalf("unexpected update result: %+v", result)
	}
	contents, err := os.ReadFile(executablePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "new-binary" {
		t.Fatalf("unexpected executable contents: %q", contents)
	}
	info, err := os.Stat(executablePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("unexpected executable mode: %o", info.Mode().Perm())
	}
}

func testArchive(t *testing.T, executable []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: "teleflow", Mode: 0o755, Size: int64(len(executable)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(executable); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
