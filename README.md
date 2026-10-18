# petroglyph

A Go Markdown-to-HTML renderer built on
[goldmark](https://github.com/yuin/goldmark). One function call turns an
untrusted Markdown document into sanitized HTML with GitHub-style extras.
No configuration required.

## Features

- GitHub-flavored Markdown: tables, strikethrough, task lists, autolinks,
  footnotes, and emoji (`:tada:`).
- Heading ids with permalink anchors, and explicit ids like
  `## Heading {#custom-id}`.
- GitHub alerts: `> [!NOTE]`, `[!TIP]`, `[!IMPORTANT]`, `[!WARNING]`,
  `[!CAUTION]`, with titles.
- Code fences highlighted by [chroma](https://github.com/alecthomas/chroma)
  with CSS classes, using the `github` style.
- Mermaid fences rendered as `<pre class="mermaid">` for a client-side
  renderer such as [Pele](https://pele.run).
- Math with `$...$`, `$$...$$`, `\(...\)`, and `\[...\]`, converted to
  MathJax delimiters.
- YAML front matter rendered as a table above the document.
- Sized images: `![alt](https://example.com/img.png =800x600)`.
- Raw HTML passes through a sanitizer policy modeled on GitHub's subset.

## Install

```
go get github.com/brutasse/petroglyph
```

## Usage

```go
import "github.com/brutasse/petroglyph"

html, flags := petroglyph.Convert(source)
```

`Convert` renders the whole document: optional front matter, body, sanitized
raw HTML, wrapped tables, and alert titles. On a render failure it returns
the source as escaped `<pre>` text instead of failing.

The returned `Flags` tell you which optional scripts the page needs:

```go
if flags.Math {
    // load MathJax
}
if flags.Mermaid {
    // load Pele, then pele.mount(el, el.textContent) on each pre.mermaid
}
```

This keeps pages free of JavaScript they do not use.

### Lower-level API

- `petroglyph.GFM` is a `goldmark.Extender`. Compose it into your own
  goldmark pipeline when you want control over parsing or rendering. You
  then sanitize the output yourself.
- `petroglyph.New()` returns a goldmark instance with the extension
  applied. Use it with `petroglyph.Needs(doc, source)` when you already
  hold the parsed document.
- `petroglyph.DisplayPolicy()` returns the bluemonday policy `Convert`
  applies. Reuse it to sanitize other markup the same way.

## Styling the output

`Convert` emits these classes. Your stylesheet decides how they look; the
library ships no CSS.

| Markup | Meaning |
| --- | --- |
| `.table-wrap` | wrapper div around every table, for `overflow-x: auto` |
| `.frontmatter` | table generated from YAML front matter |
| `.codeblock` | wrapper div around a highlighted code fence |
| `.chroma` | chroma-generated code markup (class-based) |
| `.mermaid` | `<pre>` holding a mermaid diagram source: the container a client-side renderer mounts into |
| `.markdown-alert` `.markdown-alert-note` and friends | alert blockquotes |
| `.markdown-alert-title` | title paragraph inside an alert |
| `.heading-anchor` | the `#` permalink link inside a heading |
| `.fallback` | `<pre>` used when rendering failed |

Generate the chroma stylesheet once with the same style the formatter
uses:

```go
f := chromahtml.New(chromahtml.WithClasses(true))
css, _ := os.Create("chroma.css")
defer css.Close()
f.WriteCSS(css, styles.Get("github"))
```

`showcase.md` exercises every feature. Render it with `Convert` to inspect
the full markup, or use it as a fixture in your own stylesheet work.

## Math and mermaid

Math output uses the `\(...\)` and `\[...\]` delimiters, which MathJax
activates by default. A page shows math by loading MathJax when
`Flags.Math` is set. A page shows diagrams by loading
[Pele](https://pele.run) and mounting each `pre.mermaid` when
`Flags.Mermaid` is set:

```js
for (const el of document.querySelectorAll('pre.mermaid'))
    pele.mount(el, el.textContent, { links: false, images: false });
```

`mount()` needs no initialization call, sizes each diagram to its
container, and shows a syntax error in place instead of throwing. Diagram
colors come from Pele's `--pele-*` CSS variables.

## Safety

`Convert` renders raw HTML (goldmark unsafe mode) and then filters the
result through `DisplayPolicy`. The policy extends bluemonday's UGC policy
with the classes, ids, and checkbox inputs this renderer generates. Treat
the output as display-safe, not script-safe: it can carry class and
id attributes, so do not serve your own trust-critical DOM ids that a
document could collide with.

## License

MIT. See [LICENSE](LICENSE).
