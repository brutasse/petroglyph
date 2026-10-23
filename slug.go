package petroglyph

import (
	"strings"

	"github.com/yuin/goldmark/v2/ast"
)

// headingText returns the heading's visible text as rendered: the text nodes
// of its inline subtree concatenated (links, autolinks and image alts reduce
// to their child text), line breaks as single spaces. A code span carries its
// text in the node rather than in child text nodes, and that text is literal
// source, so it is read undecoded — "`a &amp; b`" slugs as "a-amp-b". Nodes
// without text contribute nothing: math spans and raw HTML hold source
// segments, emoji and footnote references hold none.
func headingText(h *ast.Heading, source []byte) string {
	var b strings.Builder
	_ = ast.Walk(h, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			switch t := n.(type) {
			case *ast.Text:
				b.WriteString(t.Value.Value(source))
				if t.SoftLineBreak() || t.HardLineBreak() {
					b.WriteByte(' ')
				}
			case *ast.CodeSpan:
				b.WriteString(t.Value.Str(source))
			}
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}

// githubPunct is the ASCII punctuation GitHub deletes when it turns a
// heading into an anchor. Hyphen and underscore are absent from it on
// purpose: those two survive, exactly as an author typed them.
const githubPunct = `\'!"#$%&()*+,./:;<=>?@[]^` + "`" + `{|}~`

// githubDropped reports the characters GitHub removes from a heading anchor:
// its ASCII set plus the two general-punctuation blocks that cover the dashes,
// quotes and bullets a heading can hold.
func githubDropped(r rune) bool {
	if (r >= 0x2000 && r <= 0x206F) || (r >= 0x2E00 && r <= 0x2E7F) {
		return true
	}
	return r < 0x80 && strings.ContainsRune(githubPunct, r)
}

// slug follows GitHub's anchor rule: trim, lowercase, delete what GitHub
// deletes, then turn every remaining space into one hyphen. Nothing collapses
// a hyphen run, so "# C -- D" gives "c----d" here and on GitHub: a link
// written against this renderer's anchors works there too. Letters of any
// script survive verbatim ("comité-exécutif", "東京").
func slug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if githubDropped(r) {
			continue
		}
		if r == ' ' {
			b.WriteByte('-')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
