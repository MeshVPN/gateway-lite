package main

import (
	"embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed webui/index.html
var webuiFS embed.FS

// serveWebUI serves the embedded single-page management console at "/".
func serveWebUI(c *gin.Context) {
	data, err := webuiFS.ReadFile("webui/index.html")
	if err != nil {
		c.String(http.StatusInternalServerError, "webui not found")
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", data)
}
