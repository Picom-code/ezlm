package integrate

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Text-level YAML edits for DeepSeek Harness's patch files. The user's own
// rows and providers must stay byte for byte as they were (their comments,
// quoting, `!!js` tags and indentation), so EZLM doesn't re-encode the file:
// it parses it with yaml.v3 for positions, then replaces or inserts only the
// lines of the one key it owns. Every edit is checked by parsing the result
// and comparing it with the same edit made on the node tree; when the text
// edit doesn't give exactly that tree (an unusual layout), the node tree is
// encoded instead, which keeps the meaning and most comments.

// dshDoc is one YAML file being edited.
type dshDoc struct {
	src   []byte
	start []int // byte offset where each line starts (1-based line n at start[n-1]); one extra entry = len(src)
	crlf  bool
	doc   *yaml.Node // the document node, nil for an empty file
}

func dshParse(src []byte) (*dshDoc, error) {
	d := &dshDoc{src: src, crlf: bytes.Contains(src, []byte("\r\n"))}
	d.start = append(d.start, 0)
	for i, c := range src {
		if c == '\n' {
			d.start = append(d.start, i+1)
		}
	}
	if d.start[len(d.start)-1] != len(src) {
		d.start = append(d.start, len(src))
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return nil, err
	}
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		d.doc = &doc
	}
	return d, nil
}

// root is the document's top node, or nil.
func (d *dshDoc) root() *yaml.Node {
	if d.doc == nil {
		return nil
	}
	return d.doc.Content[0]
}

func (d *dshDoc) lines() int { return len(d.start) - 1 }

// line is line n (1-based) without its line break.
func (d *dshDoc) line(n int) string {
	if n < 1 || n > d.lines() {
		return ""
	}
	return strings.TrimRight(string(d.src[d.start[n-1]:d.start[n]]), "\r\n")
}

// offset is the byte offset of line n, column col (both 1-based); n past the
// end gives the end of the file.
func (d *dshDoc) offset(n, col int) int {
	if n > d.lines() {
		return len(d.src)
	}
	return d.start[n-1] + col - 1
}

// blockEnd is the first line after the block that starts on line from with
// its key (or sequence dash) at column col: the following lines indented
// deeper belong to it, and with dashes also lines that start a sequence item
// at col (YAML lets a mapping's sequence value sit at the key's own indent).
// Blank and comment lines after the last such line are left to whatever
// follows.
func (d *dshDoc) blockEnd(from, col int, dashes bool) int {
	last := from
	for n := from + 1; n <= d.lines(); n++ {
		l := d.line(n)
		t := strings.TrimLeft(l, " ")
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		ind := len(l) - len(t) + 1
		if ind > col || (dashes && ind == col && (t == "-" || strings.HasPrefix(t, "- "))) {
			last = n
			continue
		}
		break
	}
	return last + 1
}

// pairEnd is the first line after m's key/value pair at index i (the key's
// index in m.Content).
func (d *dshDoc) pairEnd(m *yaml.Node, i int) int {
	k, v := m.Content[i], m.Content[i+1]
	return d.blockEnd(k.Line, k.Column, v.Kind == yaml.SequenceNode && v.Style&yaml.FlowStyle == 0 && v.Column == k.Column)
}

// dshEdit replaces src[from:to] with text.
type dshEdit struct {
	from, to int
	text     string
}

func (d *dshDoc) apply(e dshEdit) []byte {
	out := make([]byte, 0, len(d.src)+len(e.text))
	out = append(out, d.src[:e.from]...)
	out = append(out, e.text...)
	return append(out, d.src[e.to:]...)
}

// render encodes n with two-space indents, every line after the first
// indented by col-1 spaces, and the first one too when indentFirst. The text
// ends with a line break in the file's own style.
func (d *dshDoc) render(n *yaml.Node, col int, indentFirst bool) (string, error) {
	b, err := yamlEncode(n)
	if err != nil {
		return "", err
	}
	pad := strings.Repeat(" ", col-1)
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	for i := range lines {
		if (i > 0 || indentFirst) && lines[i] != "" {
			lines[i] = pad + lines[i]
		}
	}
	nl := "\n"
	if d.crlf {
		nl = "\r\n"
	}
	return strings.Join(lines, nl) + nl, nil
}

// insertAt is an edit that inserts text at the start of line n, adding a
// line break first when the file doesn't end with one.
func (d *dshDoc) insertAt(n int, text string) dshEdit {
	at := d.offset(n, 1)
	if at == len(d.src) && at > 0 && d.src[at-1] != '\n' {
		if d.crlf {
			text = "\r\n" + text
		} else {
			text = "\n" + text
		}
	}
	return dshEdit{from: at, to: at, text: text}
}

// replacePair is an edit that rewrites m's pair at index i as key: v.
func (d *dshDoc) replacePair(m *yaml.Node, i int, v *yaml.Node) (dshEdit, error) {
	k := m.Content[i]
	text, err := d.render(yamlMap(k.Value, v), k.Column, false)
	if err != nil {
		return dshEdit{}, err
	}
	return dshEdit{from: d.offset(k.Line, k.Column), to: d.offset(d.pairEnd(m, i), 1), text: text}, nil
}

// appendPair is an edit that adds key: v after m's last pair. m must be a
// non-empty block mapping.
func (d *dshDoc) appendPair(m *yaml.Node, key string, v *yaml.Node) (dshEdit, error) {
	if !dshBlock(m) {
		return dshEdit{}, fmt.Errorf("not a block mapping")
	}
	text, err := d.render(yamlMap(key, v), m.Content[0].Column, true)
	if err != nil {
		return dshEdit{}, err
	}
	return d.insertAt(d.pairEnd(m, len(m.Content)-2), text), nil
}

// appendRow is an edit that adds row at the end of the top-level list (or
// makes the list, in a file with none).
func (d *dshDoc) appendRow(row *yaml.Node) (dshEdit, error) {
	seq := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{row}}
	root := d.root()
	switch {
	case root == nil:
		text, err := d.render(seq, 1, true)
		return d.insertAt(d.lines()+1, text), err
	case root.Kind != yaml.SequenceNode:
		return dshEdit{}, fmt.Errorf("not a list")
	case root.Style&yaml.FlowStyle != 0 && len(root.Content) == 0:
		// "[]": the shipped template's empty list.
		from := d.offset(root.Line, root.Column)
		close := bytes.IndexByte(d.src[from:], ']')
		if close < 0 {
			return dshEdit{}, fmt.Errorf("unclosed list")
		}
		to := from + close + 1
		if bytes.HasPrefix(d.src[to:], []byte("\r\n")) {
			to += 2
		} else if to < len(d.src) && d.src[to] == '\n' {
			to++
		}
		text, err := d.render(seq, root.Column, false)
		return dshEdit{from: from, to: to, text: text}, err
	case root.Style&yaml.FlowStyle != 0:
		return dshEdit{}, fmt.Errorf("flow list")
	}
	last := root.Content[len(root.Content)-1]
	end := d.blockEnd(last.Line, root.Column, false)
	text, err := d.render(seq, root.Column, true)
	return d.insertAt(end, text), err
}

// dshBlock reports whether m is a non-empty block-style mapping, the shape
// the text edits handle.
func dshBlock(m *yaml.Node) bool {
	return m != nil && m.Kind == yaml.MappingNode && m.Style&yaml.FlowStyle == 0 && len(m.Content) >= 2
}

// dshPairIndex is the index of key in m.Content (-1 if absent).
func dshPairIndex(m *yaml.Node, key string) int {
	if m == nil || m.Kind != yaml.MappingNode {
		return -1
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return i
		}
	}
	return -1
}

// dshSame reports whether two node trees mean the same YAML: kinds, tags,
// scalar values and children in order. Positions, styles and comments are
// ignored.
func dshSame(a, b *yaml.Node) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Kind == yaml.AliasNode {
		a = a.Alias
	}
	if b.Kind == yaml.AliasNode {
		b = b.Alias
	}
	if a.Kind != b.Kind {
		return false
	}
	if a.Kind == yaml.ScalarNode {
		if a.ShortTag() != b.ShortTag() {
			return false
		}
		return a.ShortTag() == "!!null" || a.Value == b.Value
	}
	if len(a.Content) != len(b.Content) {
		return false
	}
	for i := range a.Content {
		if !dshSame(a.Content[i], b.Content[i]) {
			return false
		}
	}
	return true
}

// dshEdited applies e and keeps the result only when it parses to want (the
// same edit made on the node tree); otherwise it encodes want.
func dshEdited(d *dshDoc, e dshEdit, eerr error, want *yaml.Node) ([]byte, error) {
	if eerr == nil {
		out := d.apply(e)
		var got yaml.Node
		if yaml.Unmarshal(out, &got) == nil && dshSame(&got, want) {
			return out, nil
		}
	}
	return yamlEncode(want)
}
