// Command ghreadme writes the repository's .github/README.md from
// apps/desktop/README.md, so that the repository's page on GitHub, which
// prefers .github/README.md to the root one, introduces the desktop app.
// The root README.md stays mx-core's own.
//
// Relative links and image sources are rewritten from apps/desktop to
// .github. Run it from apps/desktop:
//
//	go run ./internal/ghreadme
//
// TestInSync fails when the two files differ.
package main

import (
	"flag"
	"log"
	"os"
	"regexp"
	"strings"
)

const header = "<!-- Generated from apps/desktop/README.md by `go run ./internal/ghreadme` in apps/desktop. Do not edit: edit that file and run it again. -->\n\n"

// prefix leads from .github to apps/desktop.
const prefix = "../apps/desktop/"

// links are the relative targets of Markdown links and images, and of
// HTML src and href attributes.
var links = regexp.MustCompile(`(\]\(|(?:src|href)=")([^)"\s]+)`)

// render returns the .github README for the desktop README.
func render(src string) string {
	return header + links.ReplaceAllStringFunc(src, func(m string) string {
		parts := links.FindStringSubmatch(m)
		lead, target := parts[1], parts[2]
		if !relative(target) {
			return m
		}
		return lead + prefix + strings.TrimPrefix(target, "./")
	})
}

func relative(target string) bool {
	if strings.HasPrefix(target, "#") || strings.HasPrefix(target, "/") || strings.HasPrefix(target, "mailto:") {
		return false
	}
	return !strings.Contains(target, "://")
}

func main() {
	in := flag.String("in", "README.md", "the desktop README")
	out := flag.String("out", "../../.github/README.md", "the README to write")
	flag.Parse()
	data, err := os.ReadFile(*in)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, []byte(render(string(data))), 0o644); err != nil {
		log.Fatal(err)
	}
}
