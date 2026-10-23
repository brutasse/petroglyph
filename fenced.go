package petroglyph

import (
	stdhtml "html"
	"io"
	"strings"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/renderer"
	htmlrenderer "github.com/yuin/goldmark/v2/renderer/html"
	"github.com/yuin/goldmark/v2/util"
)

var (
	chromaFormatter = chromahtml.New(chromahtml.WithClasses(true))
	chromaStyle     = styles.Get("github")
)

// renderCodeBlock replaces goldmark's code-block rendering. goldmark v2 gives
// indented and fenced blocks one kind, so the indented case keeps CommonMark's
// shape here and only fenced fences get mermaid or chroma treatment.
func renderCodeBlock(w io.Writer, source []byte, n ast.Node, entering bool, rc renderer.Context) (ast.WalkStatus, error) {
	bw := w.(util.BufWriter)
	cb := n.(*ast.CodeBlock)
	if cb.CodeBlockKind != ast.CodeBlockKindFenced {
		if entering {
			_, _ = bw.WriteString("<pre><code>")
			_, _ = cb.Value.WriteTo(htmlrenderer.ContextTextWriter(rc), source)
		} else {
			_, _ = bw.WriteString("</code></pre>\n")
		}
		return ast.WalkContinue, nil
	}
	if !entering {
		return ast.WalkContinue, nil
	}
	info := fencedInfo(cb, source)
	src := fencedText(cb, source)

	if info == "mermaid" {
		_, _ = bw.WriteString(`<pre class="mermaid">`)
		_, _ = bw.WriteString(stdhtml.EscapeString(src))
		_, _ = bw.WriteString("</pre>")
		return ast.WalkSkipChildren, nil
	}
	// Language hint is the first word of the info string (```bash matches,
	// ```bash title=x matches too); unknown hints fall back to escaped
	// preformatted text. The wrapper div lets the shared app.js attach copy
	// buttons — sanitized content carries no scripts or buttons of its own.
	lang := info
	if i := strings.IndexAny(lang, " ,"); i >= 0 {
		lang = lang[:i]
	}
	_, _ = bw.WriteString(`<div class="codeblock">`)
	if lexer := lexers.Get(lang); lexer != nil {
		it, err := chroma.Coalesce(lexer).Tokenise(nil, src)
		if err == nil {
			err = chromaFormatter.Format(w, chromaStyle, it)
		}
		if err != nil {
			_, _ = bw.WriteString(plainPre(src))
		}
	} else {
		_, _ = bw.WriteString(plainPre(src))
	}
	_, _ = bw.WriteString("</div>")
	return ast.WalkSkipChildren, nil
}

// fencedInfo is the lowercased, trimmed info string of a fence, empty for an
// indented code block.
func fencedInfo(cb *ast.CodeBlock, source []byte) string {
	if cb.CodeBlockKind != ast.CodeBlockKindFenced {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(cb.Info.Str(source)))
}

func fencedText(cb *ast.CodeBlock, source []byte) string {
	return strings.TrimRight(cb.Value.Str(source), "\n")
}
