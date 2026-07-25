package web

import (
	"embed"
	"io/fs"
)

// staticFS はフロントエンドのビルド成果物 (web/frontend から `make frontend-build` で生成)。
//
//go:embed all:static
var staticFS embed.FS

// StaticFS は埋め込んだ静的ファイルのルートを返す。
func StaticFS() fs.FS {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	return sub
}
