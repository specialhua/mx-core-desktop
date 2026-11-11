package main

import (
	"embed"
	"strings"

	"github.com/egoist/mygo/ui"
)

// The sidebar is generated from the web admin's routes; after an upstream
// update, run `go generate` here and review the diff of nav_gen.go.
//
//go:generate go run ./internal/gennav -admin ../admin -out nav_gen.go -icons icons

// navSection is a titled group of the sidebar, as the web admin's
// (content), (community)… route groups.
type navSection struct {
	Title, TitleEN string
	Items          []navItem
}

// navItem is a page of the web admin, by its hash route.
type navItem struct {
	Path           string
	Title, TitleEN string
	Icon           string
	Children       []navItem
}

//go:embed icons/*.svg
var iconFS embed.FS

var iconCache = map[string]*ui.SVG{}

func navIcon(name string) *ui.SVG {
	if name == "" {
		return nil
	}
	if svg, ok := iconCache[name]; ok {
		return svg
	}
	data, err := iconFS.ReadFile("icons/" + name + ".svg")
	var svg *ui.SVG
	if err == nil {
		svg, _ = ui.ParseSVG(data)
	}
	iconCache[name] = svg
	return svg
}

// activeNavPath returns the sidebar item a web route belongs to: the item
// with the longest path that is the route or a parent of it, as the web
// sidebar highlights /posts for /posts?page=2 and /comments for /comments/1.
func activeNavPath(route string) string {
	if i := strings.IndexAny(route, "?#"); i >= 0 {
		route = route[:i]
	}
	best := ""
	var walk func(items []navItem)
	walk = func(items []navItem) {
		for _, it := range items {
			if (route == it.Path || strings.HasPrefix(route, it.Path+"/")) && len(it.Path) > len(best) {
				best = it.Path
			}
			walk(it.Children)
		}
	}
	for _, s := range navSections {
		walk(s.Items)
	}
	return best
}
