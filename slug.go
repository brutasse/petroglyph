package petroglyph

import (
	"strings"

	"github.com/yuin/goldmark/ast"
)

// headingText returns the heading's visible text as rendered: the text
// nodes of its inline subtree concatenated (links, autolinks, code spans
// and image alts reduce to their child text), line breaks as single
// spaces. Nodes without text children contribute nothing: math spans and
// raw HTML hold source segments, emoji and footnote references hold none.
func headingText(h *ast.Heading, source []byte) string {
	var b strings.Builder
	_ = ast.Walk(h, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if t, ok := n.(*ast.Text); ok {
				b.Write(t.Segment.Value(source))
				if t.SoftLineBreak() || t.HardLineBreak() {
					b.WriteByte(' ')
				}
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
