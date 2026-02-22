package transform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

const testdataDir = "../testdata"

func testIBGEMunicipalitiesServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(testdataDir, "tabmun.csv"))
	}))
}

func TestLoadCompanyCSVs(t *testing.T) {
	srcs := sources()
	pth := filepath.Join(testdataDir, "2026-01.zip")
	ext := t.TempDir()
	if err := unzipMainArchive(pth, ext, nil); err != nil {
		t.Fatalf("expected no error extracting archive, got %s", err)
	}

	for _, tc := range []struct {
		key string
		exp []string
	}{ // expected value is the first column of each row
		{"cna", []string{"6204000", "6201501", "6202300", "6203100", "6209100", "6311900"}},
		{"emp", []string{"33683111", "19131243"}},
		{"mot", []string{"00", "01"}},
		{"mun", []string{"9701"}},
		{"nat", []string{"2011"}},
		{"pai", []string{"105"}},
		{"qua", []string{"05", "10", "16"}},
		{"sim", []string{"33683111"}},
		{"soc", []string{"33683111", "33683111", "33683111", "33683111", "33683111", "33683111", "19131243"}},
	} {
		src := srcs[tc.key]
		t.Run(src.prefix, func(t *testing.T) {
			ctx := context.Background()
			kv, err := newBadger(t.TempDir(), false)
			defer func() {
				if err := kv.db.Close(); err != nil {
					t.Errorf("expected no error closing badger, got %s", err)
				}
			}()
			if err != nil {
				t.Errorf("expected no error creating badger, got %s", err)
			}
			if err := loadCSVs(ctx, ext, src, nil, kv, false); err != nil {
				t.Errorf("expected no error loading csvs, got %s", err)
			}
			for _, id := range tc.exp {
				key := src.keyPrefixFor(id)
				got, err := kv.getPrefix(key)
				if err != nil {
					t.Errorf("expect no error getting %s, got %s", string(key), err)
				}
				if got == nil {
					t.Errorf("expected to find key %s, got nil", string(key))
				}
			}
		})
	}
}

func TestLoadCSVs(t *testing.T) {
	srcs := sources()

	for _, tc := range []struct {
		key string
		exp []string
	}{ // expected value is the first column of each row
		{"imu", []string{"2023"}},
		{"arb", []string{"2023"}},
		{"pre", []string{"2018"}},
		{"rea", []string{"2023"}},
	} {
		src := srcs[tc.key]
		t.Run(src.prefix, func(t *testing.T) {
			ctx := context.Background()
			kv, err := newBadger(t.TempDir(), false)
			defer func() {
				if err := kv.db.Close(); err != nil {
					t.Errorf("expected no error closing badger, got %s", err)
				}
			}()
			if err != nil {
				t.Errorf("expected no error creating badger, got %s", err)
			}
			if err := loadCSVs(ctx, testdataDir, src, nil, kv, false); err != nil {
				t.Errorf("expected no error loading csvs, got %s", err)
			}
			for _, id := range tc.exp {
				key := src.keyPrefixFor(id)
				got, err := kv.getPrefix(key)
				if err != nil {
					t.Errorf("expect no error getting %s, got %s", string(key), err)
				}
				if got == nil {
					t.Errorf("expected to find key %s, got nil", string(key))
				}
			}
		})
	}
}

func TestLoadIBGEMunicipalitiesFromURL(t *testing.T) {
	srcs := sources()
	src := srcs["tab"]
	ts := testIBGEMunicipalitiesServer(t)
	defer ts.Close()

	ctx := context.Background()
	kv, err := newBadger(t.TempDir(), false)
	if err != nil {
		t.Fatalf("expected no error creating badger, got %s", err)
	}
	defer func() {
		if err := kv.db.Close(); err != nil {
			t.Errorf("expected no error closing badger, got %s", err)
		}
	}()
	if err := loadIBGEMunicipalitiesFromURL(ctx, ts.URL, src, nil, kv); err != nil {
		t.Fatalf("expected no error loading municipalities, got %s", err)
	}
	key := src.keyPrefixFor("9701")
	got, err := kv.getPrefix(key)
	if err != nil {
		t.Fatalf("expect no error getting %s, got %s", string(key), err)
	}
	if got == nil {
		t.Fatalf("expected to find key %s, got nil", string(key))
	}
}
