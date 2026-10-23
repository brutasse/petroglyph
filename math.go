package petroglyph

import (
	"fmt"
	stdhtml "html"
	"io"
	"regexp"
	"strings"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/renderer"
	"github.com/yuin/goldmark/v2/text"
)

// Math parses as regular AST nodes: a span inside a line becomes an inline
// node, and a $$...$$ block with the delimiters on their own lines becomes a
// block node. Content renders as escaped text in the TeX delimiters MathJax
// is configured for ($..$ becomes \(..\) and a $$ block becomes \[..\]),
// which is why a dollar-form pair the parser deliberately left alone (money)
// can never be re-detected by MathJax's own, laxer scanner. Code spans and
// fences never reach the inline parser: they consume their trigger bytes
// (backticks) earlier in the line, so math-looking text inside them is
// untouched. GitHub reads only these two dollar forms, so no backslash
// delimiters are parsed: "\(x\)" is an escaped parenthesis on both sides.

var (
	// Both are anchored at position 0: the parser triggers at the opening
	// delimiter and must match from there.
	mathDisplay = regexp.MustCompile(`^\$\$(.+?)\$\$`)
	// Inline $...$: same line, non-empty, no space right after the opening
	// or before the closing delimiter ("$5 and $6" is money, not math).
	mathDollar = regexp.MustCompile(`^\$(\S(?:[^$\n]*\S)?)\$`)
)

var mathSpanRules = []struct {
	re      *regexp.Regexp
	display bool
}{
	{mathDisplay, true},
	{mathDollar, false},
}

var (
	kindMath      = ast.NewNodeKind("Math")
	kindMathBlock = ast.NewNodeKind("MathBlock")
)

// mathNode is a single-line TeX span; content is the source segment
// between the delimiters.
type mathNode struct {
	ast.BaseInline
	content text.Segment
	display bool
}

func (n *mathNode) Kind() ast.NodeKind { return kindMath }

func (n *mathNode) Dump(_ []byte) *ast.NodeDump {
	return ast.NewNodeDump(n, map[string]any{
		"Display": n.display,
	})
}

func newMathNode(content text.Segment, display bool) *mathNode {
	n := &mathNode{content: content, display: display}
	n.Init(n)
	return n
}

type mathInlineParser struct{}

func (mathInlineParser) Trigger() []byte { return []byte{'$'} }

func (mathInlineParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, seg := block.PeekLine()
	for _, rule := range mathSpanRules {
		m := rule.re.FindSubmatchIndex(line)
		if m == nil {
			continue
		}
		block.Advance(m[1])
		return newMathNode(text.NewSegment(seg.Start+m[2], seg.Start+m[3]), rule.display)
	}
	return nil
}

// mathBlockNode is a display block delimited by standalone "$$" lines. When
// the document runs out before the closing delimiter, the source renders as
// literal text instead of math.
type mathBlockNode struct {
	ast.BaseBlock
	opener string
	closer string
	closed bool
}

func (n *mathBlockNode) Kind() ast.NodeKind { return kindMathBlock }

func (n *mathBlockNode) Dump(_ []byte) *ast.NodeDump {
	return ast.NewNodeDump(n, map[string]any{
		"Opener": n.opener, "Closed": n.closed,
	})
}

func newMathBlockNode(opener, closer string) *mathBlockNode {
	n := &mathBlockNode{opener: opener, closer: closer}
	n.Init(n)
	return n
}

var mathBlockDelims = map[string]string{"$$": "$$"}

type mathBlockParser struct{}

func (mathBlockParser) Trigger() []byte { return []byte{'$'} }

func (mathBlockParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, _ := reader.PeekLine()
	opener := strings.TrimSpace(string(line))
	closer, ok := mathBlockDelims[opener]
	if !ok {
		return nil, parser.NoChildren
	}
	// No reader advance here: the block loop advances the opener line.
	return newMathBlockNode(opener, closer), parser.NoChildren
}

func (mathBlockParser) Continue(n ast.Node, reader text.Reader, pc parser.Context) parser.State {
	mb := n.(*mathBlockNode)
	line, seg := reader.PeekLine()
	if strings.TrimSpace(string(line)) == mb.closer {
		mb.closed = true
		reader.AdvanceToEOL()
		return parser.Close
	}
	mb.AppendSource(seg)
	reader.AdvanceToEOL()
	return parser.Continue | parser.NoChildren
}

// Math never interrupts a paragraph: mid-paragraph delimiters are inline
// parser business, and a "$$" that merely sits among text lines is
// ordinary content.
func (mathBlockParser) CanInterruptParagraph() bool { return false }

func (mathBlockParser) CanAcceptIndentedLine() bool { return false }

func (mathBlockParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {}

func renderMathSpan(w io.Writer, source []byte, n ast.Node, entering bool, _ renderer.Context) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	mn := n.(*mathNode)
	open, closing := `\(`, `\)`
	if mn.display {
		open, closing = `\[`, `\]`
	}
	fmt.Fprintf(w, "%s%s%s", open, stdhtml.EscapeString(mn.content.Str(source)), closing)
	return ast.WalkSkipChildren, nil
}

func renderMathBlock(w io.Writer, source []byte, n ast.Node, entering bool, _ renderer.Context) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	mb := n.(*mathBlockNode)
	var body strings.Builder
	for _, seg := range mb.Source() {
		body.Write(seg.Bytes(source))
	}
	content := strings.TrimRight(body.String(), "\n")
	if !mb.closed {
		fmt.Fprintf(w, "%s\n", stdhtml.EscapeString(mb.opener+"\n"+content))
		return ast.WalkSkipChildren, nil
	}
	fmt.Fprintf(w, "%s\n", stdhtml.EscapeString(`\[`+content+`\]`))
	return ast.WalkSkipChildren, nil
}
