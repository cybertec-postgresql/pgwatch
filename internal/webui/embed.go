package webui

import (
	"embed"
	"io/fs"
)

//go:embed build
var efs embed.FS

var WebUIFs fs.FS

func init() {
	WebUIFs, _ = fs.Sub(efs, "build")
}
