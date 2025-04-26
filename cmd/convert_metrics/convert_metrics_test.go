package main

import (
	"archive/zip"
	"flag"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func downloadAndExtractMetrics(t *testing.T, dest string) string {
	t.Helper()
	url := "https://github.com/cybertec-postgresql/pgwatch2/archive/refs/heads/master.zip"
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("failed to download zip: %v", err)
	}
	defer resp.Body.Close()

	zipFile, err := os.CreateTemp("", "pgwatch2-*.zip")
	if err != nil {
		t.Fatalf("failed to create temp zip: %v", err)
	}
	defer os.Remove(zipFile.Name())

	_, err = io.Copy(zipFile, resp.Body)
	if err != nil {
		t.Fatalf("failed to save zip: %v", err)
	}
	zipFile.Close()

	r, err := zip.OpenReader(zipFile.Name())
	if err != nil {
		t.Fatalf("failed to open zip: %v", err)
	}
	defer r.Close()

	var metricsRoot string
	for _, f := range r.File {
		if strings.HasSuffix(f.Name, "/metrics/") {
			metricsRoot = f.Name
			break
		}
	}
	if metricsRoot == "" {
		t.Fatal("metrics folder not found in zip")
	}

	var extracted string
	for _, f := range r.File {
		if strings.HasPrefix(f.Name, metricsRoot) {
			rel := strings.TrimPrefix(f.Name, metricsRoot)
			if rel == "" {
				extracted = filepath.Join(dest, "metrics")
				os.MkdirAll(extracted, 0755)
				continue
			}
			path := filepath.Join(dest, "metrics", rel)
			if f.FileInfo().IsDir() {
				os.MkdirAll(path, 0755)
			} else {
				os.MkdirAll(filepath.Dir(path), 0755)
				out, err := os.Create(path)
				if err != nil {
					t.Fatalf("failed to create file: %v", err)
				}
				rc, err := f.Open()
				if err != nil {
					out.Close()
					t.Fatalf("failed to open file in zip: %v", err)
				}
				io.Copy(out, rc)
				out.Close()
				rc.Close()
			}
		}
	}
	return extracted
}

func TestMain_Integration(t *testing.T) {
	tempDir := t.TempDir()
	metricsDir := downloadAndExtractMetrics(t, tempDir)
	dstFile := filepath.Join(tempDir, "out.yaml")

	t.Run("success", func(t *testing.T) {
		flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
		os.Args = []string{"convert_metrics", "-src", metricsDir, "-dst", dstFile}
		main()
		data, err := os.ReadFile(dstFile)
		if err != nil {
			t.Fatalf("output file not created: %v", err)
		}
		if !strings.Contains(string(data), "metrics:") {
			t.Errorf("output missing metrics key")
		}
		if !strings.Contains(string(data), "presets:") {
			t.Errorf("output missing presets key")
		}
	})

	t.Run("missing_cmd_opts", func(t *testing.T) {
		flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
		os.Args = []string{"convert_metrics"}
		assert.NotPanics(t, func() { main() })
	})

}

func TestMain_getArgs(t *testing.T) {
	t.Run("missing_both", func(t *testing.T) {
		flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
		os.Args = []string{"convert_metrics"}
		assert.Error(t, getArgs(flag.String("src", "", ""), flag.String("dst", "", "")))
	})
	t.Run("missing_dst", func(t *testing.T) {
		flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
		os.Args = []string{"convert_metrics", "-src", "foo"}
		assert.Error(t, getArgs(flag.String("src", "", ""), flag.String("dst", "", "")))
	})
	t.Run("missing_src", func(t *testing.T) {
		flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
		os.Args = []string{"convert_metrics", "-dst", "foo"}
		assert.Error(t, getArgs(flag.String("src", "", ""), flag.String("dst", "", "")))
	})
}
