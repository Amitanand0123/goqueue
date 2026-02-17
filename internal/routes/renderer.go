package routes

import (
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin/render"
)

// MultiRenderer is a custom Gin HTML renderer that creates a separate template
// tree for each page, so that {{define "content"}} blocks don't collide.
type MultiRenderer struct {
	templates map[string]*template.Template
	funcMap   template.FuncMap
	logger    *slog.Logger
}

// NewMultiRenderer loads layouts, partials, and pages into isolated template trees.
func NewMultiRenderer(funcMap template.FuncMap, logger *slog.Logger) *MultiRenderer {
	r := &MultiRenderer{
		templates: make(map[string]*template.Template),
		funcMap:   funcMap,
		logger:    logger,
	}
	r.load()
	return r
}

func (r *MultiRenderer) load() {
	// Collect shared templates (layouts + partials)
	sharedFiles := []string{}
	for _, dir := range []string{"web/templates/layouts", "web/templates/partials"} {
		matches, err := filepath.Glob(filepath.Join(dir, "*.html"))
		if err != nil {
			r.logger.Error("failed to glob shared templates", "dir", dir, "error", err)
			continue
		}
		sharedFiles = append(sharedFiles, matches...)
	}

	// For each page template, create an isolated template tree:
	// clone shared (layout+partials) + parse the page on top
	pageFiles, err := filepath.Glob(filepath.Join("web/templates/pages", "*.html"))
	if err != nil {
		r.logger.Error("failed to glob page templates", "error", err)
		return
	}

	for _, pageFile := range pageFiles {
		name := "pages/" + filepath.Base(pageFile)

		// Start fresh: parse shared files first
		files := make([]string, 0, len(sharedFiles)+1)
		files = append(files, sharedFiles...)
		files = append(files, pageFile)

		tmpl, err := template.New("").Funcs(r.funcMap).ParseFiles(files...)
		if err != nil {
			r.logger.Error("failed to parse template", "name", name, "error", err)
			continue
		}

		r.templates[name] = tmpl
		r.logger.Debug("loaded template", "name", name)
	}

	// Also load partials as standalone templates (for HTMX partial responses)
	partialFiles, _ := filepath.Glob(filepath.Join("web/templates/partials", "*.html"))
	for _, pf := range partialFiles {
		name := "partials/" + filepath.Base(pf)

		data, err := os.ReadFile(pf)
		if err != nil {
			r.logger.Error("failed to read partial", "name", name, "error", err)
			continue
		}

		// Partials need the funcMap but are standalone
		tmpl, err := template.New(name).Funcs(r.funcMap).Parse(string(data))
		if err != nil {
			r.logger.Error("failed to parse partial", "name", name, "error", err)
			continue
		}

		r.templates[name] = tmpl
		r.logger.Debug("loaded partial template", "name", name)
	}
}

// Instance returns a render.Render for the given template name and data.
func (r *MultiRenderer) Instance(name string, data interface{}) render.Render {
	tmpl, ok := r.templates[name]
	if !ok {
		r.logger.Error("template not found", "name", name, "available", r.templateNames())
		return &render.HTML{
			Template: template.Must(template.New("error").Parse("Template not found: " + name)),
			Data:     data,
		}
	}

	// For page templates, render the base layout which includes {{template "content" .}}
	// The layout file is named "base.html" by ParseFiles (uses filename, not path)
	renderName := ""
	if strings.HasPrefix(name, "pages/") {
		renderName = "base.html"
	} else if strings.HasPrefix(name, "partials/") {
		// Partials define themselves as just the filename (e.g. "stats_cards.html")
		renderName = filepath.Base(name)
	}

	return &render.HTML{
		Template: tmpl,
		Name:     renderName,
		Data:     data,
	}
}

// WriteContentType implements render.HTMLRender.
func (r *MultiRenderer) WriteContentType(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
}

func (r *MultiRenderer) templateNames() []string {
	names := make([]string, 0, len(r.templates))
	for n := range r.templates {
		names = append(names, n)
	}
	return names
}
