package updater

import (
	"context"
	"testing"
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
