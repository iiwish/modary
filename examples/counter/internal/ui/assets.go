// Package ui owns the Counter consumer's static web assets.
package ui

import (
	"embed"
	"io/fs"
)

//go:embed static/*
var embedded embed.FS

// Assets returns a filesystem rooted at the consumer-owned static bundle.
func Assets() fs.FS {
	content, err := fs.Sub(embedded, "static")
	if err != nil {
		panic(err)
	}
	return content
}
