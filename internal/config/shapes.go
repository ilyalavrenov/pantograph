package config

import "sort"

//nolint:gochecknoglobals // static allow-list of renderable shapes
var shapes = map[string]bool{
	"":              true,
	"rectangle":     true,
	"square":        true,
	"page":          true,
	"document":      true,
	"diamond":       true,
	"hexagon":       true,
	"cylinder":      true,
	"queue":         true,
	"parallelogram": true,
	"circle":        true,
	"oval":          true,
	"cloud":         true,
	"step":          true,
	"callout":       true,
	"stored_data":   true,
	"package":       true,
}

func validShape(s string) bool { return shapes[s] }

func shapeNames() []string {
	out := make([]string, 0, len(shapes))
	for s := range shapes {
		if s == "" {
			continue
		}

		out = append(out, s)
	}

	sort.Strings(out)

	return out
}
