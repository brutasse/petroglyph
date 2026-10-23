package petroglyph

import (
	"fmt"
	stdhtml "html"
	"io"
	"regexp"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/renderer"
	"github.com/yuin/goldmark/v2/text"
)

// GitHub's explicit image sizing is not CommonMark; parse the syntax into a
// custom node. The emitted tag goes through the same sanitizer as regular
// images. (Raw <img width=height=> markup also survives the sanitizer now,
// but the markdown shorthand is what GitHub's own editor produces.)

var sizedImageMatch = regexp.MustCompile(`^!\[([^\]]*)\]\(([^\s()]+) =([0-9]*)x([0-9]*)\)`)

type sizedImageNode struct {
	ast.BaseInline
	alt, url, width, height string
}

var kindSizedImage = ast.NewNodeKind("SizedImage")

func (n *sizedImageNode) Kind() ast.NodeKind { return kindSizedImage }

func (n *sizedImageNode) Dump(_ []byte) *ast.NodeDump {
	return ast.NewNodeDump(n, map[string]any{
		"URL": n.url, "Width": n.width, "Height": n.height,
	})
}

func newSizedImageNode(alt, url, width, height string) *sizedImageNode {
	n := &sizedImageNode{alt: alt, url: url, width: width, height: height}
	n.Init(n)
	return n
}

type sizedImageParser struct{}

func (sizedImageParser) Trigger() []byte { return []byte{'!'} }

func (sizedImageParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, _ := block.PeekLine()
	m := sizedImageMatch.FindSubmatch(line)
	if m == nil || (len(m[3]) == 0 && len(m[4]) == 0) {
		return nil
	}
	block.Advance(len(m[0]))
	return newSizedImageNode(string(m[1]), string(m[2]), string(m[3]), string(m[4]))
}

func renderSizedImage(w io.Writer, _ []byte, n ast.Node, entering bool, _ renderer.Context) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	ni := n.(*sizedImageNode)
	fmt.Fprintf(w, `<img src="%s" alt="%s"`,
		stdhtml.EscapeString(ni.url), stdhtml.EscapeString(ni.alt))
	if ni.width != "" {
		fmt.Fprintf(w, ` width="%s"`, ni.width)
	}
	if ni.height != "" {
		fmt.Fprintf(w, ` height="%s"`, ni.height)
	}
	fmt.Fprintf(w, `>`)
	return ast.WalkSkipChildren, nil
}
