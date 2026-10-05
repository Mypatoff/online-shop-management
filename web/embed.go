// Package web embeds the HTML templates and static assets into the
// compiled binary so the server needs no external files at runtime.
package web

import "embed"

//go:embed templates
var Templates embed.FS

//go:embed static
var Static embed.FS
