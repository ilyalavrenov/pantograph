package render

import "github.com/ilyalavrenov/pantograph/internal/collect"

type (
	Flow = collect.Flow
	Node = collect.Node
	Edge = collect.Edge
)

const (
	KindEvent    = "event"
	KindDecision = "decision"
	KindGateway  = "gateway"
	KindStore    = "store"
	KindBackstop = "backstop"
)

//nolint:gochecknoglobals // the render tests' shared kind→shape vocabulary
var testShapes = map[string]string{
	KindEvent:    "page",
	KindDecision: "diamond",
	KindGateway:  "hexagon",
	KindStore:    "cylinder",
	KindBackstop: "",
}
