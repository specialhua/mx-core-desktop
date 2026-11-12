package main

import (
	"os"
	"strings"
	"testing"
)

// TestInSync fails when .github/README.md is not what the desktop README
// renders to: run `go run ./internal/ghreadme` in apps/desktop.
func TestInSync(t *testing.T) {
	src, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../../../.github/README.md")
	if err != nil {
		t.Fatalf("%v; run `go run ./internal/ghreadme` in apps/desktop", err)
	}
	if string(got) != render(string(src)) {
		t.Error(".github/README.md is out of date; run `go run ./internal/ghreadme` in apps/desktop")
	}
}

func TestRender(t *testing.T) {
	in := `<img src="docs/a.png" /> [x](./main.go) [y](https://mygo.run) [z](#top) ![i](docs/b.png)`
	want := `<img src="../apps/desktop/docs/a.png" /> [x](../apps/desktop/main.go) [y](https://mygo.run) [z](#top) ![i](../apps/desktop/docs/b.png)`
	if got := strings.TrimPrefix(render(in), header); got != want {
		t.Errorf("render:\n got %s\nwant %s", got, want)
	}
}
