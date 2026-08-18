package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed all:dist
var content embed.FS

func Register(router *gin.Engine) {
	dist, err := fs.Sub(content, "dist")
	if err != nil {
		panic(err)
	}
	index, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		panic(err)
	}
	files := http.FS(dist)

	router.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}

		requested := strings.TrimPrefix(path.Clean(c.Request.URL.Path), "/")
		if requested != "." && requested != "" {
			if file, err := dist.Open(requested); err == nil {
				file.Close()
				c.FileFromFS(requested, files)
				return
			}
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", index)
	})
}
