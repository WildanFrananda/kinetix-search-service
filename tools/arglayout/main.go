package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var generated = regexp.MustCompile(`(?m)^// Code generated .* DO NOT EDIT\.$`)

type item struct {
	start token.Pos
	end   token.Pos
	text  string
}

type group struct {
	open   token.Pos
	close  token.Pos
	items  []item
	closer string
}

func main() {
	fix := flag.Bool("fix", false, "rewrite files instead of reporting")

	flag.Parse()

	failed := false

	for _, root := range flag.Args() {
		tree, err := os.OpenRoot(root)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}

		err = fs.WalkDir(tree.FS(), ".", func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if entry.IsDir() && skipped(path) {
				return filepath.SkipDir
			}

			if entry.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}

			bad, err := process(tree, root, path, *fix)
			if err != nil {
				return err
			}

			failed = failed || bad

			return nil
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	}

	if failed {
		fmt.Println("a call, literal or signature spread over several lines must put each element on its own line")
		os.Exit(1)
	}
}

func skipped(path string) bool {
	for _, generated := range []string{"contractgen", "protogen", "storegen"} {
		if filepath.Base(path) == generated {
			return true
		}
	}

	return false
}

func process(tree *os.Root, root, path string, fix bool) (bool, error) {
	name := filepath.Join(root, path)

	source, err := tree.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("reading %s: %w", name, err)
	}

	if generated.Match(source) {
		return false, nil
	}

	if !fix {
		groups, fset, err := violations(name, source)
		if err != nil {
			return false, err
		}

		for _, g := range groups {
			fmt.Printf("%s: elements share a line\n", fset.Position(g.open))
		}

		return len(groups) > 0, nil
	}

	for range 20 {
		groups, fset, err := violations(name, source)
		if err != nil {
			return false, err
		}

		if len(groups) == 0 {
			break
		}

		source, err = rewrite(source, fset, outermost(groups))
		if err != nil {
			return false, fmt.Errorf("%s: %w", path, err)
		}
	}

	original, err := tree.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("reading %s: %w", name, err)
	}

	if !bytes.Equal(original, source) {
		if err := tree.WriteFile(path, source, 0o600); err != nil {
			return false, fmt.Errorf("writing %s: %w", name, err)
		}
	}

	return false, nil
}

func violations(path string, source []byte) ([]group, *token.FileSet, error) {
	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, path, source, parser.ParseComments)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	line := func(pos token.Pos) int { return fset.Position(pos).Line }
	text := func(start, end token.Pos) string {
		return string(source[fset.Position(start).Offset:fset.Position(end).Offset])
	}

	var found []group

	check := func(g group, trailingBlock bool) {
		if len(g.items) == 0 || line(g.open) == line(g.close) {
			return
		}

		if trailingBlock && sameLineHead(g, line) {
			return
		}

		previous := line(g.open)
		for _, it := range g.items {
			if line(it.start) <= previous {
				found = append(found, g)
				return
			}

			previous = line(it.end)
		}

		if line(g.close) <= previous {
			found = append(found, g)
		}
	}

	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.CallExpr:
			g := group{open: n.Lparen, close: n.Rparen, closer: ")"}
			for i, arg := range n.Args {
				body := text(arg.Pos(), arg.End())
				if i == len(n.Args)-1 && n.Ellipsis.IsValid() {
					body += "..."
				}

				g.items = append(g.items, item{start: arg.Pos(), end: arg.End(), text: body})
			}

			check(g, len(n.Args) > 0 && block(n.Args[len(n.Args)-1]))
		case *ast.CompositeLit:
			g := group{open: n.Lbrace, close: n.Rbrace, closer: "}"}
			for _, elt := range n.Elts {
				g.items = append(g.items, item{start: elt.Pos(), end: elt.End(), text: text(elt.Pos(), elt.End())})
			}

			check(g, len(n.Elts) > 0 && block(n.Elts[len(n.Elts)-1]))
		case *ast.FuncType:
			for _, list := range []*ast.FieldList{n.Params, n.Results} {
				if list == nil || !list.Opening.IsValid() || !list.Closing.IsValid() {
					continue
				}

				g := group{open: list.Opening, close: list.Closing, closer: ")"}
				for _, field := range list.List {
					g.items = append(
						g.items,
						item{start: field.Pos(), end: field.End(), text: text(field.Pos(), field.End())},
					)
				}

				check(g, false)
			}
		}

		return true
	})

	return found, fset, nil
}

func block(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.FuncLit, *ast.CompositeLit:
		return true
	case *ast.KeyValueExpr:
		return block(e.Value)
	case *ast.UnaryExpr:
		return block(e.X)
	default:
		return false
	}
}

func sameLineHead(g group, line func(token.Pos) int) bool {
	for _, it := range g.items {
		if line(it.start) != line(g.open) {
			return false
		}
	}

	return line(g.close) == line(g.items[len(g.items)-1].end)
}

func outermost(groups []group) []group {
	sort.Slice(groups, func(a, b int) bool { return groups[a].open < groups[b].open })

	var kept []group
	for _, g := range groups {
		if len(kept) > 0 && g.open < kept[len(kept)-1].close {
			continue
		}

		kept = append(kept, g)
	}

	return kept
}

func rewrite(source []byte, fset *token.FileSet, groups []group) ([]byte, error) {
	var out bytes.Buffer

	cursor := 0

	for _, g := range groups {
		open := fset.Position(g.open).Offset
		closing := fset.Position(g.close).Offset

		out.Write(source[cursor : open+1])
		out.WriteString("\n")

		for _, it := range g.items {
			out.WriteString(strings.TrimSpace(it.text))
			out.WriteString(",\n")
		}

		cursor = closing
	}

	out.Write(source[cursor:])

	formatted, err := format.Source(out.Bytes())
	if err != nil {
		return nil, fmt.Errorf("formatting: %w", err)
	}

	return formatted, nil
}
