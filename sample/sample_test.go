package sample

import (
	"os"
	"path/filepath"
	"testing"
)

var testdata = filepath.Join("..", "testdata")

func TestSample(t *testing.T) {
	out := t.TempDir()
	if err := Sample(testdata, out, 2); err != nil {
		t.Fatalf("expected no error running sample, got %s", err)
	}
	ls, err := os.ReadDir(out)
	if err != nil {
		t.Errorf("expected no error reading dir %s, got %s", out, err)
	}
	var got int
	for _, f := range ls {
		if !f.IsDir() {
			got++
		}
	}
	zips, err := filepath.Glob(filepath.Join(testdata, "*.zip"))
	if err != nil {
		t.Fatalf("expected no error globbing testdata zips, got %s", err)
	}
	if got != len(zips) {
		t.Errorf("expected %d files in the sample directory, got %d", len(zips), got)
	}
}
