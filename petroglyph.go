// Package petroglyph is a Markdown-to-HTML stack: a zero-config goldmark
// extension assembling GitHub-flavored Markdown (tables, strikethrough,
// task lists, autolinks, footnotes, emoji) plus heading ids built by
// GitHub's anchor rule with permalink anchors, GitHub alerts,
// `![alt](url =WxH)` image sizing, chroma-highlighted and mermaid fences,
// YAML front matter, and MathJax math in $...$ and $$...$$ delimiters. Raw
// HTML passes through the renderer for DisplayPolicy to gate, GitHub-style.
//
// Convert is the intended entry point. GFMParser and GFMHTMLRenderer
// compose into a custom goldmark pipeline, but then raw-HTML rendering and
// sanitizing are the caller's business.
package petroglyph

import (
	"bytes"
	stdhtml "html"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	emoji "github.com/yuin/goldmark-emoji/v2"
	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	"github.com/yuin/goldmark/v2/parser"
	htmlrenderer "github.com/yuin/goldmark/v2/renderer/html"
	"github.com/yuin/goldmark/v2/util"
)

// Flags tells the page which optional payloads the rendered HTML needs:
// the template loads only the vendored JS actually required.
type Flags struct {
	Math    bool // TeX math present: load MathJax
	Mermaid bool // mermaid fence present: load a client-side renderer
}

// GFMParser and GFMHTMLRenderer hold the goldmark v2 extensions that make up
// this package's feature set with no configuration, in the order
// parser.WithExtensions and html.WithExtensions want them. goldmark applies
// renderer extensions in list order and a later one wins the node-renderer
// map, so GFMHTMLRenderer has to stay last in a longer list: its fence,
// heading, image and math renderers replace the CommonMark ones that way.
var GFMParser = []parser.Extension{
	// Assembled by hand rather than extension.GFMParser so the table
	// renderer can be configured.
	extension.LinkifyParser,
	extension.StrikethroughParser,
	extension.TaskListItemParser,
	extension.TableParser,
	extension.FootnoteParser,
	emoji.Parser,
	gfmExt{},
}

var GFMHTMLRenderer = []htmlrenderer.Extension{
	extension.StrikethroughHTMLRenderer,
	extension.TaskListItemHTMLRenderer,
	extension.NewTableHTMLRenderer(
		// goldmark aligns cells with style attributes, which
		// DisplayPolicy strips; the align attribute survives.
		extension.WithTableCellAlignMethod(extension.TableCellAlignAttribute),
	),
	extension.FootnoteHTMLRenderer,
	emoji.HTMLRenderer,
	gfmHTMLExt{},
}

type gfmExt struct{}

func (gfmExt) ParserOptions(*parser.Config) []parser.Option {
	return []parser.Option{
		// parser.WithAttribute stays off: GitHub reads a "# Heading {#id}"
		// suffix as text, so it stays text here. Heading ids come from the
		// headingIDTransformer below, and DisplayPolicy bounds the id and
		// class attributes those and raw HTML carry.
		// 50: ahead of every other inline parser on math's trigger byte
		// ($), so emphasis cannot start inside a TeX span. codespans still
		// win over math because they trigger earlier in the line, at the
		// backtick. sized images before footnote (101) and link (1000).
		parser.WithInlineParsers(
			util.Prioritized[parser.InlineParser](&mathInlineParser{}, 50),
			util.Prioritized[parser.InlineParser](&sizedImageParser{}, 100),
		),
		parser.WithBlockParsers(
			util.Prioritized[parser.BlockParser](&mathBlockParser{}, 250),
		),
		parser.WithASTTransformers(
			util.Prioritized[parser.ASTTransformer](&alertTransformer{}, 900), // avoids footnote's 999
			util.Prioritized[parser.ASTTransformer](&headingIDTransformer{}, 901),
		),
	}
}

type gfmHTMLExt struct{}

func (gfmHTMLExt) RendererOptions(*htmlrenderer.Config) []htmlrenderer.Option {
	return []htmlrenderer.Option{
		htmlrenderer.WithNodeRenderers(map[ast.NodeKind]htmlrenderer.NodeRenderer{
			ast.KindCodeBlock: htmlrenderer.NodeRendererFunc(renderCodeBlock),
			ast.KindHeading:   htmlrenderer.NodeRendererFunc(renderHeading),
			kindSizedImage:    htmlrenderer.NodeRendererFunc(renderSizedImage),
			kindMath:          htmlrenderer.NodeRendererFunc(renderMathSpan),
			kindMathBlock:     htmlrenderer.NodeRendererFunc(renderMathBlock),
		}),
	}
}

// Markdown is the Markdown-to-HTML pipeline: parse with Parser, then render
// the document with Renderer.
type Markdown struct {
	Parser   parser.Parser
	Renderer htmlrenderer.Renderer
}

// New returns a Markdown with the GFM extensions applied.
func New() *Markdown {
	return &Markdown{
		Parser: parser.New(parser.WithExtensions(GFMParser...)),
		Renderer: htmlrenderer.New(
			// Raw HTML reaches the sanitizer instead of being omitted;
			// DisplayPolicy is the gate, GitHub-style.
			htmlrenderer.WithUnsafe(),
			htmlrenderer.WithExtensions(GFMHTMLRenderer...),
		),
	}
}

// Convert renders a whole Markdown document: the optional YAML front
// matter table, then the body with raw HTML gated by DisplayPolicy, every
// table wrapped for horizontal scrolling, and alert titles injected (trusted
// markup, post-sanitize). On render failure it returns the source as escaped
// preformatted text.
func Convert(src string) (string, Flags) {
	fmTable, body := SplitFrontMatter(src)
	md := New()
	source := []byte(body)
	doc := md.Parser.Parse(source)
	var out bytes.Buffer
	if err := md.Renderer.Render(&out, source, doc); err != nil {
		return plainPre(src), Flags{}
	}
	safe := string(policy.SanitizeBytes(out.Bytes()))
	return fmTable + wrapTables(AddAlertTitles(safe)), Needs(doc, source)
}

// Needs reports whether the parsed document contains math or mermaid.
func Needs(doc ast.Node, source []byte) Flags {
	var f Flags
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n.Kind() {
		case kindMath, kindMathBlock:
			f.Math = true
		case ast.KindCodeBlock:
			if fencedInfo(n.(*ast.CodeBlock), source) == "mermaid" {
				f.Mermaid = true
			}
		}
		return ast.WalkContinue, nil
	})
	return f
}

var policy = DisplayPolicy()

// DisplayPolicy renders untrusted content safely. Chroma emits class-named
// spans/tables and mermaid fences need a class; UGC strips class by
// default, so allow it back — no user-controlled CSS is ever served,
// classes only map to the fixed stylesheets the app ships.
func DisplayPolicy() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowElements("table", "thead", "tbody", "tr", "td", "th", "div", "span")
	p.AllowAttrs("class").Globally()
	// Task-list checkboxes only: goldmark's TaskList emits disabled inputs,
	// which the UGC forms ban would otherwise strip.
	p.AllowAttrs("type").Matching(checkboxType).OnElements("input")
	p.AllowAttrs("checked", "disabled").OnElements("input")
	// ids are generated (heading slugs, footnote fn:/fnref:
	// links), never taken verbatim from user text; the charset regex keeps
	// raw-HTML-injected ids out of DOM-clobbering shapes. Note: UGC's
	// AllowStandardAttributes already admits id/title globally with their
	// own charset-limited regexes and bluemonday offers no removal API,
	// so raw HTML can still set those two — accepted: the charset bounds
	// the effect to id collisions, not script or markup injection.
	p.AllowAttrs("id").Matching(safeID).OnElements("h1", "h2", "h3", "h4", "h5", "h6", "sup", "li", "div")
	// The standard id rule is unanchored, so a slug containing any ASCII
	// from its set survives — accented ids like "comité-exécutif" pass.
	// A pure non-Latin slug ("東京") contains none and would be stripped,
	// dangling its anchor; this additional rule rescues it, scoped to
	// headings.
	p.AllowAttrs("id").Matching(unicodeID).OnElements("h1", "h2", "h3", "h4", "h5", "h6")
	// The one gap in UGC vs GitHub's raw-HTML subset.
	p.AllowElements("kbd")
	// GitHub documents <a name="x"></a> as its custom-anchor idiom, and an
	// anchor with no allowed attribute is dropped with its name.
	p.AllowAttrs("name").Matching(safeID).OnElements("a")
	return p
}

var (
	checkboxType = regexp.MustCompile(`^checkbox$`)
	safeID       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9:._-]{0,63}$`)
	unicodeID    = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N}:._-]{0,63}$`)
)

// wrapTables puts every table in a horizontally scrollable box, so wide
// tables (many columns, unbreakable paths) scroll instead of pushing the
// whole page wide. Runs after sanitize: the only literal "<table" left at
// this point is generated or sanitizer-approved markup — code-fence text
// is already escaped.
func wrapTables(s string) string {
	s = strings.ReplaceAll(s, "</table>", "</table></div>")
	return strings.ReplaceAll(s, "<table", `<div class="table-wrap"><table`)
}

func plainPre(s string) string {
	return `<pre class="fallback"><code>` + stdhtml.EscapeString(s) + `</code></pre>`
}
