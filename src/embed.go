package main

import (
	"embed"
	"io/fs"
)

//go:embed webui/build
var efs embed.FS

var webuifs fs.FS

func init() {
	webuifs, _ = fs.Sub(efs, "webui/build")
}
