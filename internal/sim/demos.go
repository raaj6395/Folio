package sim

import (
	"embed"
	"strings"
)

//go:embed demos/*.json
var demoFS embed.FS

// Demo is one of the reference systems: the engine's fixtures, and what `folio demo` installs.
type Demo struct {
	Name  string // file name without .json
	Title string
	JSON  []byte
}

var demoTitles = map[string]string{
	"read_path":      "Demo system: product read path",
	"url_shortener":  "Demo system: URL shortener",
	"ticket_booking": "Demo system: ticket booking",
}

func Demos() []Demo {
	ents, _ := demoFS.ReadDir("demos")
	var out []Demo
	for _, e := range ents {
		name := strings.TrimSuffix(e.Name(), ".json")
		data, _ := demoFS.ReadFile("demos/" + e.Name())
		out = append(out, Demo{name, demoTitles[name], data})
	}
	return out
}
