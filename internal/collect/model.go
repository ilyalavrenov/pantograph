package collect

import (
	"strings"
)

type Node struct {
	Flow        string
	Qual        string
	Lane        string
	Pos         string
	Kind        string
	HandoffFrom []string
	HandoffTo   []string

	HandoffLabels map[string]EndpointLabel
}

type EndpointLabel struct {
	Cond string
	Note string
}

type Edge struct {
	From, To  string
	Cond      string
	Note      string
	Handoff   bool
	Goroutine bool
}

type Flow struct {
	ID      string
	Nodes   []*Node
	Edges   []Edge
	Members []string
	Note    string
}

func FuncLabel(qual string) string {
	if i := strings.LastIndex(qual, "."); i >= 0 {
		return qual[i+1:]
	}

	return qual
}
