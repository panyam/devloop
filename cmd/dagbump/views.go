package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/panyam/devloop/dagbump"
	"golang.org/x/term"
)

// ---- list --pretty ----

func termWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	if w, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && w > 0 {
		return w
	}
	return 160
}

// wrapText breaks s into lines of at most width runes, on spaces where it can.
func wrapText(s string, width int) []string {
	if width < 1 {
		width = 1
	}
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		for utf8.RuneCountInString(word) > width {
			if line != "" {
				lines = append(lines, line)
				line = ""
			}
			r := []rune(word)
			lines = append(lines, string(r[:width]))
			word = string(r[width:])
		}
		switch {
		case line == "":
			line = word
		case utf8.RuneCountInString(line)+1+utf8.RuneCountInString(word) <= width:
			line += " " + word
		default:
			lines = append(lines, line)
			line = word
		}
	}
	if line != "" || len(lines) == 0 {
		lines = append(lines, line)
	}
	return lines
}

func pad(s string, w int) string {
	if n := utf8.RuneCountInString(s); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}

// printPrettyList draws a boxed table sized to the terminal. The last column
// (notes) takes the remaining width and wraps inside its cell, one warning
// per bullet.
func printPrettyList(out io.Writer, views []unitView) {
	headers := []string{"NAME", "KIND", "VERSION", "MODS", "NPM", "UNREL", "LAG", "CFG", "NOTES"}
	rows := make([][]string, len(views))
	notes := make([][]string, len(views))
	for i, v := range views {
		cfg := "-"
		if v.Config {
			cfg = "yes"
		}
		rows[i] = []string{v.Name, v.Kind, dash(v.Version), strconv.Itoa(len(v.Modules)), strconv.Itoa(len(v.Npm)),
			strconv.Itoa(len(v.Unreleased)), strconv.Itoa(len(v.Lagging)), cfg}
		if v.Error != "" {
			notes[i] = []string{"ERROR: " + v.Error}
		} else {
			notes[i] = v.Warnings
		}
	}
	widths := make([]int, len(headers))
	for c, h := range headers {
		widths[c] = len(h)
	}
	for _, r := range rows {
		for c, cell := range r {
			widths[c] = max(widths[c], utf8.RuneCountInString(cell))
		}
	}
	// Each column takes "│ " + content + " ", plus the closing "│".
	fixed := 1
	for c := 0; c < len(headers)-1; c++ {
		fixed += widths[c] + 3
	}
	last := len(headers) - 1
	widths[last] = max(20, termWidth()-fixed-3)

	rule := func(l, m, r string) {
		var b strings.Builder
		b.WriteString(l)
		for c, w := range widths {
			b.WriteString(strings.Repeat("─", w+2))
			if c < last {
				b.WriteString(m)
			}
		}
		b.WriteString(r)
		fmt.Fprintln(out, b.String())
	}
	line := func(cells []string) {
		var b strings.Builder
		for c, w := range widths {
			b.WriteString("│ ")
			b.WriteString(pad(cells[c], w))
			b.WriteString(" ")
		}
		b.WriteString("│")
		fmt.Fprintln(out, b.String())
	}

	rule("┌", "┬", "┐")
	line(headers)
	rule("├", "┼", "┤")
	for i, r := range rows {
		var noteLines []string
		for _, n := range notes[i] {
			for j, l := range wrapText(n, widths[last]-2) {
				if j == 0 {
					noteLines = append(noteLines, "• "+l)
				} else {
					noteLines = append(noteLines, "  "+l)
				}
			}
		}
		if len(noteLines) == 0 {
			noteLines = []string{""}
		}
		for j, nl := range noteLines {
			cells := make([]string, len(headers))
			if j == 0 {
				copy(cells, r)
			}
			cells[last] = nl
			line(cells)
		}
		if i < len(rows)-1 {
			rule("├", "┼", "┤")
		}
	}
	rule("└", "┴", "┘")
}

// ---- list --svg / --html ----

const (
	nodeW   = 190
	nodeH   = 46
	colGap  = 80
	rowGap  = 16
	marginX = 24
	marginY = 40
)

type layoutNode struct {
	Name       string `json:"name"`
	Wave       int    `json:"wave"`
	X          int    `json:"x"`
	Y          int    `json:"y"`
	Kind       string `json:"kind"`
	Version    string `json:"version"`
	Unreleased int    `json:"unreleased"`
	Lagging    int    `json:"lagging"`
	Warnings   int    `json:"warnings"`
	Error      bool   `json:"error"`
}

type layoutEdge struct {
	From       string `json:"from"` // the dependent
	To         string `json:"to"`   // the dependency
	Lagging    bool   `json:"lagging"`
	Propagates bool   `json:"propagates"`
	Detail     string `json:"detail"`
	path       string
}

type graphLayout struct {
	Nodes  []*layoutNode `json:"nodes"`
	Edges  []*layoutEdge `json:"edges"`
	Waves  int           `json:"waves"`
	Width  int           `json:"width"`
	Height int           `json:"height"`
}

// layoutGraph places the units that have edges in columns, one per release
// wave, and orders each column by the mean row of its neighbours to cut
// down on crossings.
func layoutGraph(g *dagbump.Graph, views map[string]unitView) (*graphLayout, error) {
	set := map[*dagbump.Unit]bool{}
	for _, e := range g.Edges {
		if e.From != e.To {
			set[e.From], set[e.To] = true, true
		}
	}
	waves, err := g.Waves(set)
	if err != nil {
		return nil, err
	}

	type key struct{ from, to string }
	merged := map[key]*layoutEdge{}
	var edges []*layoutEdge
	for _, e := range g.Edges {
		if e.From == e.To {
			continue
		}
		k := key{e.From.Name, e.To.Name}
		le := merged[k]
		if le == nil {
			le = &layoutEdge{From: e.From.Name, To: e.To.Name}
			merged[k] = le
			edges = append(edges, le)
		}
		le.Lagging = le.Lagging || e.Lagging
		le.Propagates = le.Propagates || e.Propagates
		d := fmt.Sprintf("%s %s requires %s", e.Kind, e.Module, dash(e.Required))
		if e.Lagging {
			d += ", latest " + dash(e.Latest)
		}
		if le.Detail == "" {
			le.Detail = d
		} else if !strings.Contains(le.Detail, d) {
			le.Detail += "\n" + d
		}
	}

	cols := make([][]string, len(waves))
	waveOf := map[string]int{}
	for i, w := range waves {
		for _, u := range w {
			cols[i] = append(cols[i], u.Name)
			waveOf[u.Name] = i
		}
	}
	nbrs := map[string][]string{}
	for _, e := range edges {
		nbrs[e.From] = append(nbrs[e.From], e.To)
		nbrs[e.To] = append(nbrs[e.To], e.From)
	}
	rowOf := func() map[string]float64 {
		m := map[string]float64{}
		for _, col := range cols {
			for r, n := range col {
				m[n] = float64(r)
			}
		}
		return m
	}
	sweep := func(i, ref int) {
		pos := rowOf()
		bary := map[string]float64{}
		for _, n := range cols[i] {
			sum, cnt := 0.0, 0
			for _, m := range nbrs[n] {
				if waveOf[m] == ref || (ref < i && waveOf[m] < i) || (ref > i && waveOf[m] > i) {
					sum += pos[m]
					cnt++
				}
			}
			if cnt > 0 {
				bary[n] = sum / float64(cnt)
			} else {
				bary[n] = pos[n]
			}
		}
		sort.SliceStable(cols[i], func(a, b int) bool { return bary[cols[i][a]] < bary[cols[i][b]] })
	}
	for pass := 0; pass < 4; pass++ {
		for i := 1; i < len(cols); i++ {
			sweep(i, i-1)
		}
		for i := len(cols) - 2; i >= 0; i-- {
			sweep(i, i+1)
		}
	}

	l := &graphLayout{Waves: len(waves)}
	maxRows := 0
	for _, col := range cols {
		maxRows = max(maxRows, len(col))
	}
	colH := func(n int) int { return n*nodeH + (n-1)*rowGap }
	l.Height = marginY*2 + colH(maxRows)
	l.Width = marginX*2 + len(cols)*nodeW + (len(cols)-1)*colGap
	nodes := map[string]*layoutNode{}
	for i, col := range cols {
		top := marginY + (colH(maxRows)-colH(len(col)))/2
		for r, name := range col {
			v := views[name]
			n := &layoutNode{Name: name, Wave: i + 1, X: marginX + i*(nodeW+colGap), Y: top + r*(nodeH+rowGap),
				Kind: v.Kind, Version: v.Version, Unreleased: len(v.Unreleased), Lagging: len(v.Lagging),
				Warnings: len(v.Warnings), Error: v.Error != ""}
			nodes[name] = n
			l.Nodes = append(l.Nodes, n)
		}
	}
	for _, e := range edges {
		a, b := nodes[e.To], nodes[e.From]
		x1, y1 := a.X+nodeW, a.Y+nodeH/2
		x2, y2 := b.X, b.Y+nodeH/2
		dx := max(40, (x2-x1)/2)
		e.path = fmt.Sprintf("M%d,%d C%d,%d %d,%d %d,%d", x1, y1, x1+dx, y1, x2-dx, y2, x2-6, y2)
	}
	// Draw lagging and propagating edges last so they sit on top.
	sort.SliceStable(edges, func(i, j int) bool {
		rank := func(e *layoutEdge) int {
			if e.Lagging {
				return 2
			}
			if e.Propagates {
				return 1
			}
			return 0
		}
		return rank(edges[i]) < rank(edges[j])
	})
	l.Edges = edges
	return l, nil
}

const svgStyle = `
  .dg { font: 12px ui-sans-serif, system-ui, -apple-system, "Segoe UI", sans-serif; }
  .dg .bg { fill: #ffffff; }
  .dg .wave { fill: #6b7280; font-size: 11px; letter-spacing: .06em; }
  .dg .edge { fill: none; stroke: #9ca3af; stroke-width: 1.2; opacity: .45; }
  .dg .edge.weak { stroke-dasharray: 4 3; opacity: .3; }
  .dg .edge.lag { stroke: #dc2626; opacity: .4; }
  .dg .node rect.box { fill: #f9fafb; stroke: #9ca3af; stroke-width: 1.2; }
  .dg .node.app rect.box { stroke-dasharray: 5 3; }
  .dg .node.lagging rect.box { stroke: #d97706; stroke-width: 2; fill: #fffbeb; }
  .dg .node.error rect.box { stroke: #dc2626; stroke-width: 2; fill: #fef2f2; }
  .dg .node .name { font-weight: 600; fill: #111827; }
  .dg .node .meta { fill: #4b5563; font-size: 11px; }
  .dg .node .meta .lag { fill: #b45309; }
  .dg .node .meta .unrel { fill: #1d4ed8; }
  .dg .node .meta .warn { fill: #6b7280; }
  .dg marker path { fill: #9ca3af; }
  .dg marker#arrow-lag path { fill: #dc2626; }
  @media (prefers-color-scheme: dark) {
    .dg .bg { fill: #111318; }
    .dg .edge { stroke: #5b6170; }
    .dg .edge.lag { stroke: #f87171; }
    .dg .node rect.box { fill: #1b1e25; stroke: #5b6170; }
    .dg .node.lagging rect.box { stroke: #f59e0b; fill: #2a2112; }
    .dg .node.error rect.box { stroke: #f87171; fill: #2c1515; }
    .dg .node .name { fill: #e5e7eb; }
    .dg .node .meta { fill: #9ca3af; }
    .dg .node .meta .lag { fill: #fbbf24; }
    .dg .node .meta .unrel { fill: #93c5fd; }
    .dg marker path { fill: #5b6170; }
    .dg marker#arrow-lag path { fill: #f87171; }
  }
`

func esc(s string) string { return template.HTMLEscapeString(s) }

// renderSVG draws the layout. standalone adds the XML namespace and a
// background so the file reads on its own.
func renderSVG(l *graphLayout, standalone bool) string {
	var b strings.Builder
	ns := ""
	if standalone {
		ns = ` xmlns="http://www.w3.org/2000/svg"`
	}
	fmt.Fprintf(&b, `<svg%s class="dg" viewBox="0 0 %d %d" width="%d" height="%d">`, ns, l.Width, l.Height, l.Width, l.Height)
	fmt.Fprintf(&b, "<style>%s</style>", svgStyle)
	b.WriteString(`<defs>` +
		`<marker id="arrow" viewBox="0 0 8 8" refX="1" refY="4" markerWidth="7" markerHeight="7" orient="auto"><path d="M0,0 L8,4 L0,8 z"/></marker>` +
		`<marker id="arrow-lag" viewBox="0 0 8 8" refX="1" refY="4" markerWidth="7" markerHeight="7" orient="auto"><path d="M0,0 L8,4 L0,8 z"/></marker>` +
		`</defs>`)
	if standalone {
		fmt.Fprintf(&b, `<rect class="bg" width="%d" height="%d"/>`, l.Width, l.Height)
	}
	for i := 0; i < l.Waves; i++ {
		fmt.Fprintf(&b, `<text class="wave" x="%d" y="%d">WAVE %d</text>`, marginX+i*(nodeW+colGap), marginY-16, i+1)
	}
	for _, e := range l.Edges {
		cls, marker := "edge", "arrow"
		if e.Lagging {
			cls, marker = "edge lag", "arrow-lag"
		} else if !e.Propagates {
			cls += " weak"
		}
		fmt.Fprintf(&b, `<path class="%s" data-from="%s" data-to="%s" d="%s" marker-end="url(#%s)"><title>%s → %s&#10;%s</title></path>`,
			cls, esc(e.From), esc(e.To), e.path, marker, esc(e.From), esc(e.To), esc(e.Detail))
	}
	for _, n := range l.Nodes {
		cls := "node " + n.Kind
		if n.Error {
			cls += " error"
		} else if n.Lagging > 0 {
			cls += " lagging"
		}
		fmt.Fprintf(&b, `<g class="%s" data-name="%s" transform="translate(%d,%d)">`, cls, esc(n.Name), n.X, n.Y)
		fmt.Fprintf(&b, `<rect class="box" width="%d" height="%d" rx="7"/>`, nodeW, nodeH)
		fmt.Fprintf(&b, `<text class="name" x="10" y="19">%s</text>`, esc(n.Name))
		b.WriteString(`<text class="meta" x="10" y="36">`)
		ver := n.Version
		if ver == "" {
			ver = n.Kind
		}
		b.WriteString(esc(ver))
		if n.Unreleased > 0 {
			fmt.Fprintf(&b, ` · <tspan class="unrel">%d unrel</tspan>`, n.Unreleased)
		}
		if n.Lagging > 0 {
			fmt.Fprintf(&b, ` · <tspan class="lag">%d lag</tspan>`, n.Lagging)
		}
		if n.Warnings > 0 {
			fmt.Fprintf(&b, ` · <tspan class="warn">⚠%d</tspan>`, n.Warnings)
		}
		b.WriteString(`</text>`)
		fmt.Fprintf(&b, `<title>%s</title></g>`, esc(n.Name))
	}
	b.WriteString(`</svg>`)
	return b.String()
}

//go:embed list.html.tmpl
var listHTML string

func renderHTML(w io.Writer, l *graphLayout, views []unitView, manifest string) error {
	data, err := json.Marshal(map[string]any{"units": views, "layout": l})
	if err != nil {
		return err
	}
	t, err := template.New("list").Parse(listHTML)
	if err != nil {
		return err
	}
	return t.Execute(w, map[string]any{
		"SVG":      template.HTML(renderSVG(l, false)),
		"Data":     template.JS(data),
		"Manifest": manifest,
	})
}

// writeOutput writes to path, or stdout for "-", and opens the file when asked.
func writeOutput(path string, open bool, render func(io.Writer) error) error {
	if path == "-" {
		return render(os.Stdout)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := render(f); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "wrote", path)
	if open {
		opener := "xdg-open"
		if runtime.GOOS == "darwin" {
			opener = "open"
		}
		return exec.Command(opener, path).Start()
	}
	return nil
}
