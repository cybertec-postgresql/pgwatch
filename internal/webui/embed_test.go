package webui

import (
	"io/fs"
	"testing"
)

func TestWebUIInit(t *testing.T) {
	if WebUIFs == nil {
		t.Error("WebUIFs is nil")
	}
	if _, err := fs.Stat(WebUIFs, "index.html"); err != nil {
		t.Errorf("WebUIFs does not contain index.html: %v", err)
	}
}
