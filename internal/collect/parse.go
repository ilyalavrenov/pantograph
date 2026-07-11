package collect

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var errNotDirective = errors.New("not a flow directive")

const (
	keyKind        = "kind"
	keyCond        = "cond"
	keyNote        = "note"
	keyHandoffFrom = "handoff-from"
	keyHandoffTo   = "handoff-to"
)

var flowIDRe = regexp.MustCompile(`^//\s*pantograph:(\S+)\s*(.*)$`)

func parseNodeDirective(line string) (*Node, error) {
	flowID, rest, ok := matchFlowPrefix(line)
	if !ok {
		return nil, errNotDirective
	}

	n := &Node{Flow: flowID}

	var hp handoffPos

	for _, tok := range tokens(rest) {
		if err := applyNodeToken(n, tok, rest, &hp); err != nil {
			return nil, err
		}
	}

	return n, nil
}

type handoffPos struct {
	name   string
	isFrom bool
}

func applyNodeToken(n *Node, tok dtok, rest string, hp *handoffPos) error {
	if !tok.hasEq {
		if strings.HasPrefix(tok.raw, `"`) || strings.Contains(rest, "->") {
			return fmt.Errorf("pantograph:%s: edges are derived; remove -> arrows", n.Flow)
		}

		return fmt.Errorf("pantograph:%s: unknown token %q (keys are key=value)", n.Flow, tok.raw)
	}

	if tok.key == keyCond || tok.key == keyNote {
		if !tok.quoted {
			return fmt.Errorf("pantograph:%s: %s= needs a quoted value (e.g. %s=%q)", n.Flow, tok.key, tok.key, tok.val)
		}

		if hp.name == "" {
			return fmt.Errorf(
				"pantograph:%s: %s= labels no edge here (put it after a handoff-from=, or label a call edge with a call-site directive)",
				n.Flow, tok.key)
		}

		if !hp.isFrom {
			return fmt.Errorf(
				"pantograph:%s: %s= after handoff-to=%s labels an incoming edge (a handoff label belongs on the -from endpoint)",
				n.Flow, tok.key, hp.name)
		}

		applyHandoffSubLabel(n, tok.key, tok.val, hp.name)

		return nil
	}

	switch tok.key {
	case keyHandoffFrom:
		*hp = handoffPos{name: tok.val, isFrom: true}
	case keyHandoffTo:
		*hp = handoffPos{name: tok.val, isFrom: false}
	}

	return applyNodeKey(n, tok)
}

func applyHandoffSubLabel(n *Node, key, val, handoff string) {
	if n.HandoffLabels == nil {
		n.HandoffLabels = map[string]EndpointLabel{}
	}

	lbl := n.HandoffLabels[handoff]
	if key == keyCond {
		lbl.Cond = val
	} else {
		lbl.Note = val
	}

	n.HandoffLabels[handoff] = lbl
}

func applyNodeKey(n *Node, tok dtok) error {
	if tok.quoted {
		return fmt.Errorf("pantograph:%s: %s= takes a bareword value, not a quoted one", n.Flow, tok.key)
	}

	switch tok.key {
	case keyKind:
		n.Kind = tok.val
	case keyHandoffFrom:
		n.HandoffFrom = append(n.HandoffFrom, tok.val)
	case keyHandoffTo:
		n.HandoffTo = append(n.HandoffTo, tok.val)
	default:
		return fmt.Errorf("pantograph:%s: unknown key %q (want kind/handoff-from/handoff-to/cond/note)", n.Flow, tok.key)
	}

	return nil
}

func parseCallSiteDirective(line string) (string, string, string, error) {
	id, rest, ok := matchFlowPrefix(line)
	if !ok {
		return "", "", "", errNotDirective
	}

	var cond, note string

	for _, tok := range tokens(rest) {
		if !tok.hasEq || (tok.key != keyCond && tok.key != keyNote) {
			return "", "", "", fmt.Errorf("pantograph:%s: call-site directive allows only cond=/note=, got %q", id, tok.raw)
		}

		if !tok.quoted {
			return "", "", "", fmt.Errorf("pantograph:%s: %s= needs a quoted value (e.g. %s=%q)", id, tok.key, tok.key, tok.val)
		}

		if tok.key == keyCond {
			cond = tok.val
		} else {
			note = tok.val
		}
	}

	return id, cond, note, nil
}

func matchFlowPrefix(line string) (string, string, bool) {
	m := flowIDRe.FindStringSubmatch(strings.TrimSpace(line))
	if m == nil {
		return "", "", false
	}

	return m[1], m[2], true
}

type dtok struct {
	key    string
	val    string
	raw    string
	hasEq  bool
	quoted bool
}

func tokens(rest string) []dtok {
	var toks []dtok

	i := 0
	for i < len(rest) {
		for i < len(rest) && isSpace(rest[i]) {
			i++
		}

		if i >= len(rest) {
			break
		}

		tok, next := scanToken(rest, i)
		toks = append(toks, tok)
		i = next
	}

	return toks
}

func scanToken(rest string, i int) (dtok, int) {
	start := i

	for i < len(rest) && !isSpace(rest[i]) && rest[i] != '=' {
		i++
	}

	key := rest[start:i]

	if i >= len(rest) || rest[i] != '=' {
		for i < len(rest) && !isSpace(rest[i]) {
			i++
		}

		return dtok{key: key, raw: rest[start:i]}, i
	}

	i++

	tok, next := scanValue(rest, i)
	tok.key = key
	tok.raw = rest[start:next]
	tok.hasEq = true

	return tok, next
}

func scanValue(rest string, i int) (dtok, int) {
	switch {
	case i < len(rest) && rest[i] == '"':
		i++
		valStart := i

		for i < len(rest) && rest[i] != '"' {
			i++
		}

		val := rest[valStart:i]
		if i < len(rest) {
			i++
		}

		return dtok{val: val, quoted: true}, i

	default:
		valStart := i
		for i < len(rest) && !isSpace(rest[i]) {
			i++
		}

		return dtok{val: rest[valStart:i]}, i
	}
}

func isSpace(b byte) bool { return b == ' ' || b == '\t' }
