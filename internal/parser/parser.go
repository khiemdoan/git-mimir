package parser

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/thuongh2/git-mimir/internal/graph"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// treeParser wraps a pre-allocated tree-sitter parser for one language.
type treeParser struct {
	lang   LangID
	parser *sitter.Parser
}

func newTreeParser(lang LangID) (*treeParser, error) {
	grammar := grammarForLang(lang)
	if grammar == nil {
		return nil, fmt.Errorf("no grammar for language %s", lang)
	}
	p := sitter.NewParser()
	p.SetLanguage(grammar)
	return &treeParser{lang: lang, parser: p}, nil
}

// ParseFile reads a file from disk, determines its language, and extracts symbols.
func ParseFile(path string, src []byte) (*graph.FileSymbols, error) {
	ext := filepath.Ext(path)
	lang := LangForExt(ext)
	if lang == "" {
		return nil, fmt.Errorf("unsupported extension: %s", ext)
	}
	if src == nil {
		var err error
		src, err = os.ReadFile(path)
		if err != nil {
			return nil, err
		}
	}

	tp, err := newTreeParser(lang)
	if err != nil {
		return nil, err
	}
	return tp.parse(path, src)
}

// parse extracts symbols from a source file using the pre-allocated parser.
func (tp *treeParser) parse(path string, src []byte) (*graph.FileSymbols, error) {
	tree := tp.parser.ParseCtx(context.Background(), src, nil)
	if tree == nil {
		return nil, fmt.Errorf("parse %s failed", path)
	}
	defer tree.Close()

	fs := &graph.FileSymbols{FilePath: path}

	switch tp.lang {
	case LangTypeScript, LangTSX, LangJavaScript:
		extractTS(tree.RootNode(), src, path, fs)
	case LangGo:
		extractGo(tree.RootNode(), src, path, fs)
	case LangPython:
		extractPython(tree.RootNode(), src, path, fs)
	case LangJava:
		extractJava(tree.RootNode(), src, path, fs)
	case LangRust:
		extractRust(tree.RootNode(), src, path, fs)
	case LangCPP:
		extractCPP(tree.RootNode(), src, path, fs)
	}

	return fs, nil
}

// ---- TypeScript / JavaScript extractor ----

func extractTS(root *sitter.Node, src []byte, path string, fs *graph.FileSymbols) {
	var currentClass string
	var currentClassUID string

	var walk func(n *sitter.Node, exported bool)
	walk = func(n *sitter.Node, exported bool) {
		switch n.Kind() {
		case "export_statement":
			// Mark children as exported
			for i := uint(0); i < n.ChildCount(); i++ {
				child := n.Child(i)
				if child.Kind() != "export" && child.Kind() != "default" {
					walk(child, true)
				}
			}
			// Handle re-exports: export { x } from './y'
			if source := n.ChildByFieldName("source"); source != nil {
				importPath := strings.Trim(source.Utf8Text(src), `"'`)
				fs.Imports = append(fs.Imports, graph.ImportRef{
					ImportPath: importPath,
				})
			}
			return

		case "import_statement":
			if source := n.ChildByFieldName("source"); source != nil {
				importPath := strings.Trim(source.Utf8Text(src), `"'`)
				isType := false
				if importKind := n.ChildByFieldName("import"); importKind != nil {
					isType = strings.Contains(importKind.Utf8Text(src), "type")
				}
				fs.Imports = append(fs.Imports, graph.ImportRef{
					ImportPath: importPath,
					IsType:     isType,
				})
			}

		case "function_declaration":
			nameNode := n.ChildByFieldName("name")
			if nameNode != nil {
				name := nameNode.Utf8Text(src)
				uid := graph.NodeUID(path, name, "Function")
				node := graph.Node{
					UID:       uid,
					Name:      name,
					Kind:      "Function",
					FilePath:  path,
					StartLine: n.StartPosition().Row + 1,
					EndLine:   n.EndPosition().Row + 1,
					Exported:  exported,
				}
				fs.Nodes = append(fs.Nodes, node)
				// Extract calls within function body
				if body := n.ChildByFieldName("body"); body != nil {
					extractCalls(body, src, uid, fs)
				}
			}

		case "method_definition":
			nameNode := n.ChildByFieldName("name")
			if nameNode != nil {
				name := nameNode.Utf8Text(src)
				kind := "Method"
				uid := graph.NodeUID(path, currentClass+"."+name, kind)
				node := graph.Node{
					UID:       uid,
					Name:      name,
					Kind:      kind,
					FilePath:  path,
					StartLine: n.StartPosition().Row + 1,
					EndLine:   n.EndPosition().Row + 1,
					Exported:  exported || currentClass != "",
				}
				fs.Nodes = append(fs.Nodes, node)
				if currentClassUID != "" {
					fs.Imports = append(fs.Imports, graph.ImportRef{
						FromUID:    uid,
						ImportPath: "MEMBER_OF:" + currentClassUID,
					})
				}
				if body := n.ChildByFieldName("body"); body != nil {
					extractCalls(body, src, uid, fs)
				}
				return // don't recurse into method children for declarations
			}

		case "class_declaration":
			nameNode := n.ChildByFieldName("name")
			if nameNode != nil {
				name := nameNode.Utf8Text(src)
				uid := graph.NodeUID(path, name, "Class")
				node := graph.Node{
					UID:       uid,
					Name:      name,
					Kind:      "Class",
					FilePath:  path,
					StartLine: n.StartPosition().Row + 1,
					EndLine:   n.EndPosition().Row + 1,
					Exported:  exported,
				}
				fs.Nodes = append(fs.Nodes, node)
				// Handle heritage (extends/implements)
				if heritage := n.ChildByFieldName("heritage"); heritage != nil {
					extractHeritage(heritage, src, uid, fs)
				}
				prevClass, prevUID := currentClass, currentClassUID
				currentClass, currentClassUID = name, uid
				if body := n.ChildByFieldName("body"); body != nil {
					walk(body, false)
				}
				currentClass, currentClassUID = prevClass, prevUID
				return
			}

		case "interface_declaration":
			nameNode := n.ChildByFieldName("name")
			if nameNode != nil {
				name := nameNode.Utf8Text(src)
				uid := graph.NodeUID(path, name, "Interface")
				fs.Nodes = append(fs.Nodes, graph.Node{
					UID:       uid,
					Name:      name,
					Kind:      "Interface",
					FilePath:  path,
					StartLine: n.StartPosition().Row + 1,
					EndLine:   n.EndPosition().Row + 1,
					Exported:  exported,
				})
			}

		case "lexical_declaration", "variable_declaration":
			// Handle: const foo = () => {} or const foo = function() {}
			for i := 0; i < int(n.ChildCount()); i++ {
				child := n.Child(uint(i))
				if child.Kind() == "variable_declarator" {
					nameNode := child.ChildByFieldName("name")
					valNode := child.ChildByFieldName("value")
					if nameNode != nil && valNode != nil {
						kind := kindOfVarDecl(valNode)
						if kind != "" {
							name := nameNode.Utf8Text(src)
							uid := graph.NodeUID(path, name, kind)
							fs.Nodes = append(fs.Nodes, graph.Node{
								UID:       uid,
								Name:      name,
								Kind:      kind,
								FilePath:  path,
								StartLine: n.StartPosition().Row + 1,
								EndLine:   n.EndPosition().Row + 1,
								Exported:  exported,
							})
							extractCalls(valNode, src, uid, fs)
						}
					}
				}
			}
		}

		// Recurse
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(uint(i)), false)
		}
	}

	walk(root, false)
}

func kindOfVarDecl(n *sitter.Node) string {
	switch n.Kind() {
	case "arrow_function", "function", "function_expression":
		return "Function"
	case "class":
		return "Class"
	default:
		return ""
	}
}

func extractHeritage(n *sitter.Node, src []byte, ownerUID string, fs *graph.FileSymbols) {
	for i := 0; i < int(n.ChildCount()); i++ {
		child := n.Child(uint(i))
		switch child.Kind() {
		case "extends_clause":
			for j := 0; j < int(child.ChildCount()); j++ {
				c := child.Child(uint(j))
				if c.Kind() == "identifier" || c.Kind() == "type_identifier" {
					fs.Calls = append(fs.Calls, graph.CallRef{
						FromUID:    ownerUID,
						CalleeName: "EXTENDS:" + c.Utf8Text(src),
					})
				}
			}
		case "implements_clause":
			for j := 0; j < int(child.ChildCount()); j++ {
				c := child.Child(uint(j))
				if c.Kind() == "type_identifier" || c.Kind() == "identifier" {
					fs.Calls = append(fs.Calls, graph.CallRef{
						FromUID:    ownerUID,
						CalleeName: "IMPLEMENTS:" + c.Utf8Text(src),
					})
				}
			}
		}
	}
}

func extractCalls(n *sitter.Node, src []byte, ownerUID string, fs *graph.FileSymbols) {
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if n.Kind() == "call_expression" {
			fn := n.ChildByFieldName("function")
			if fn != nil {
				callee := calleeText(fn, src)
				if callee != "" {
					fs.Calls = append(fs.Calls, graph.CallRef{
						FromUID:    ownerUID,
						CalleeName: callee,
						Line:       n.StartPosition().Row + 1,
					})
				}
			}
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(uint(i)))
		}
	}
	walk(n)
}

func calleeText(n *sitter.Node, src []byte) string {
	switch n.Kind() {
	case "identifier":
		return n.Utf8Text(src)
	case "member_expression":
		obj := n.ChildByFieldName("object")
		prop := n.ChildByFieldName("property")
		if obj != nil && prop != nil {
			return obj.Utf8Text(src) + "." + prop.Utf8Text(src)
		}
	}
	return ""
}

// ---- Go extractor ----

func extractGo(root *sitter.Node, src []byte, path string, fs *graph.FileSymbols) {
	// Extract package path from directory
	pkgPath := filepath.Dir(path)

	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		switch n.Kind() {
		case "import_declaration", "import_spec":
			if n.Kind() == "import_spec" {
				pathNode := n.ChildByFieldName("path")
				if pathNode != nil {
					importPath := strings.Trim(pathNode.Utf8Text(src), `"`)
					alias := ""
					if nameNode := n.ChildByFieldName("name"); nameNode != nil {
						alias = nameNode.Utf8Text(src)
					}
					fs.Imports = append(fs.Imports, graph.ImportRef{
						ImportPath: importPath,
						Alias:      alias,
					})
				}
			}

		case "function_declaration":
			nameNode := n.ChildByFieldName("name")
			if nameNode != nil {
				name := nameNode.Utf8Text(src)
				exported := len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z'
				uid := graph.NodeUID(path, name, "Function")
				fs.Nodes = append(fs.Nodes, graph.Node{
					UID:         uid,
					Name:        name,
					Kind:        "Function",
					FilePath:    path,
					StartLine:   n.StartPosition().Row + 1,
					EndLine:     n.EndPosition().Row + 1,
					Exported:    exported,
					PackagePath: pkgPath,
				})
				if body := n.ChildByFieldName("body"); body != nil {
					extractCalls(body, src, uid, fs)
				}
				return

			}

		case "method_declaration":
			nameNode := n.ChildByFieldName("name")
			receiverNode := n.ChildByFieldName("receiver")
			if nameNode != nil {
				name := nameNode.Utf8Text(src)
				receiverType := ""
				if receiverNode != nil {
					receiverType = receiverNode.Utf8Text(src)
					// Strip parens and type parts: "(r *Foo)" -> "Foo"
					receiverType = strings.Trim(receiverType, "()")
					parts := strings.Fields(receiverType)
					if len(parts) > 0 {
						receiverType = strings.TrimLeft(parts[len(parts)-1], "*")
					}
				}
				exported := len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z'
				qualName := receiverType + "." + name
				uid := graph.NodeUID(path, qualName, "Method")
				fs.Nodes = append(fs.Nodes, graph.Node{
					UID:         uid,
					Name:        name,
					Kind:        "Method",
					FilePath:    path,
					StartLine:   n.StartPosition().Row + 1,
					EndLine:     n.EndPosition().Row + 1,
					Exported:    exported,
					PackagePath: pkgPath,
				})
				if body := n.ChildByFieldName("body"); body != nil {
					extractCalls(body, src, uid, fs)
				}
				return

			}

		case "type_declaration":
			for i := 0; i < int(n.ChildCount()); i++ {
				spec := n.Child(uint(i))
				if spec.Kind() == "type_spec" {
					nameNode := spec.ChildByFieldName("name")
					typeNode := spec.ChildByFieldName("type")
					if nameNode != nil {
						name := nameNode.Utf8Text(src)
						exported := len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z'
						kind := "Type"
						if typeNode != nil && typeNode.Kind() == "struct_type" {
							kind = "Class"
						} else if typeNode != nil && typeNode.Kind() == "interface_type" {
							kind = "Interface"
						}
						uid := graph.NodeUID(path, name, kind)
						fs.Nodes = append(fs.Nodes, graph.Node{
							UID:         uid,
							Name:        name,
							Kind:        kind,
							FilePath:    path,
							StartLine:   n.StartPosition().Row + 1,
							EndLine:     n.EndPosition().Row + 1,
							Exported:    exported,
							PackagePath: pkgPath,
						})
					}
				}
			}
		}

		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(uint(i)))
		}
	}
	walk(root)
}

// ---- Python extractor ----

func extractPython(root *sitter.Node, src []byte, path string, fs *graph.FileSymbols) {
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		switch n.Kind() {
		case "import_statement", "import_from_statement":
			if n.Kind() == "import_from_statement" {
				modNode := n.ChildByFieldName("module_name")
				if modNode != nil {
					fs.Imports = append(fs.Imports, graph.ImportRef{
						ImportPath: modNode.Utf8Text(src),
					})
				}
			}

		case "function_definition":
			nameNode := n.ChildByFieldName("name")
			if nameNode != nil {
				name := nameNode.Utf8Text(src)
				uid := graph.NodeUID(path, name, "Function")
				fs.Nodes = append(fs.Nodes, graph.Node{
					UID:       uid,
					Name:      name,
					Kind:      "Function",
					FilePath:  path,
					StartLine: n.StartPosition().Row + 1,
					EndLine:   n.EndPosition().Row + 1,
					Exported:  !strings.HasPrefix(name, "_"),
				})
				if body := n.ChildByFieldName("body"); body != nil {
					extractCallsPython(body, src, uid, fs)
				}
				return
			}

		case "class_definition":
			nameNode := n.ChildByFieldName("name")
			if nameNode != nil {
				name := nameNode.Utf8Text(src)
				uid := graph.NodeUID(path, name, "Class")
				fs.Nodes = append(fs.Nodes, graph.Node{
					UID:       uid,
					Name:      name,
					Kind:      "Class",
					FilePath:  path,
					StartLine: n.StartPosition().Row + 1,
					EndLine:   n.EndPosition().Row + 1,
					Exported:  !strings.HasPrefix(name, "_"),
				})
			}
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(uint(i)))
		}
	}
	walk(root)
}

func extractCallsPython(n *sitter.Node, src []byte, ownerUID string, fs *graph.FileSymbols) {
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if n.Kind() == "call" {
			fn := n.ChildByFieldName("function")
			if fn != nil {
				callee := fn.Utf8Text(src)
				if callee != "" {
					fs.Calls = append(fs.Calls, graph.CallRef{
						FromUID:    ownerUID,
						CalleeName: callee,
						Line:       n.StartPosition().Row + 1,
					})
				}
			}
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(uint(i)))
		}
	}
	walk(n)
}

// ---- Java extractor ----

func extractJava(root *sitter.Node, src []byte, path string, fs *graph.FileSymbols) {
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		switch n.Kind() {
		case "import_declaration":
			for i := 0; i < int(n.ChildCount()); i++ {
				c := n.Child(uint(i))
				if c.Kind() == "scoped_identifier" {
					fs.Imports = append(fs.Imports, graph.ImportRef{ImportPath: c.Utf8Text(src)})
				}
			}
		case "class_declaration", "interface_declaration", "enum_declaration":
			nameNode := n.ChildByFieldName("name")
			if nameNode != nil {
				kind := "Class"
				if n.Kind() == "interface_declaration" {
					kind = "Interface"
				}
				name := nameNode.Utf8Text(src)
				uid := graph.NodeUID(path, name, kind)
				fs.Nodes = append(fs.Nodes, graph.Node{
					UID:       uid,
					Name:      name,
					Kind:      kind,
					FilePath:  path,
					StartLine: n.StartPosition().Row + 1,
					EndLine:   n.EndPosition().Row + 1,
					Exported:  true,
				})
			}
		case "method_declaration":
			nameNode := n.ChildByFieldName("name")
			if nameNode != nil {
				name := nameNode.Utf8Text(src)
				uid := graph.NodeUID(path, name, "Method")
				fs.Nodes = append(fs.Nodes, graph.Node{
					UID:       uid,
					Name:      name,
					Kind:      "Method",
					FilePath:  path,
					StartLine: n.StartPosition().Row + 1,
					EndLine:   n.EndPosition().Row + 1,
					Exported:  true,
				})
				if body := n.ChildByFieldName("body"); body != nil {
					extractCalls(body, src, uid, fs)
				}
				return
			}
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(uint(i)))
		}
	}
	walk(root)
}

// ---- Rust extractor ----

func extractRust(root *sitter.Node, src []byte, path string, fs *graph.FileSymbols) {
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		switch n.Kind() {
		case "use_declaration":
			for i := 0; i < int(n.ChildCount()); i++ {
				c := n.Child(uint(i))
				if c.Kind() == "scoped_identifier" || c.Kind() == "identifier" {
					fs.Imports = append(fs.Imports, graph.ImportRef{ImportPath: c.Utf8Text(src)})
				}
			}
		case "function_item":
			nameNode := n.ChildByFieldName("name")
			if nameNode != nil {
				name := nameNode.Utf8Text(src)
				uid := graph.NodeUID(path, name, "Function")
				pub := false
				if n.Child(0) != nil && n.Child(0).Utf8Text(src) == "pub" {
					pub = true
				}
				fs.Nodes = append(fs.Nodes, graph.Node{
					UID:       uid,
					Name:      name,
					Kind:      "Function",
					FilePath:  path,
					StartLine: n.StartPosition().Row + 1,
					EndLine:   n.EndPosition().Row + 1,
					Exported:  pub,
				})
				if body := n.ChildByFieldName("body"); body != nil {
					extractCalls(body, src, uid, fs)
				}
				return
			}
		case "struct_item", "enum_item", "trait_item":
			nameNode := n.ChildByFieldName("name")
			if nameNode != nil {
				kind := "Class"
				if n.Kind() == "trait_item" {
					kind = "Interface"
				}
				name := nameNode.Utf8Text(src)
				uid := graph.NodeUID(path, name, kind)
				fs.Nodes = append(fs.Nodes, graph.Node{
					UID:       uid,
					Name:      name,
					Kind:      kind,
					FilePath:  path,
					StartLine: n.StartPosition().Row + 1,
					EndLine:   n.EndPosition().Row + 1,
				})
			}
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(uint(i)))
		}
	}
	walk(root)
}

// ---- C++ extractor ----

func extractCPP(root *sitter.Node, src []byte, path string, fs *graph.FileSymbols) {
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		switch n.Kind() {
		case "preproc_include":
			pathNode := n.ChildByFieldName("path")
			if pathNode != nil {
				fs.Imports = append(fs.Imports, graph.ImportRef{ImportPath: pathNode.Utf8Text(src)})
			}
		case "function_definition":
			declarator := n.ChildByFieldName("declarator")
			if declarator != nil {
				nameNode := findFirstIdentifier(declarator)
				if nameNode != nil {
					name := nameNode.Utf8Text(src)
					uid := graph.NodeUID(path, name, "Function")
					fs.Nodes = append(fs.Nodes, graph.Node{
						UID:       uid,
						Name:      name,
						Kind:      "Function",
						FilePath:  path,
						StartLine: n.StartPosition().Row + 1,
						EndLine:   n.EndPosition().Row + 1,
					})
					if body := n.ChildByFieldName("body"); body != nil {
						extractCalls(body, src, uid, fs)
					}
					return
				}
			}
		case "class_specifier", "struct_specifier":
			nameNode := n.ChildByFieldName("name")
			if nameNode != nil {
				name := nameNode.Utf8Text(src)
				uid := graph.NodeUID(path, name, "Class")
				fs.Nodes = append(fs.Nodes, graph.Node{
					UID:       uid,
					Name:      name,
					Kind:      "Class",
					FilePath:  path,
					StartLine: n.StartPosition().Row + 1,
					EndLine:   n.EndPosition().Row + 1,
				})
			}
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(uint(i)))
		}
	}
	walk(root)
}

func findFirstIdentifier(n *sitter.Node) *sitter.Node {
	if n.Kind() == "identifier" || n.Kind() == "qualified_identifier" {
		return n
	}
	for i := 0; i < int(n.ChildCount()); i++ {
		if found := findFirstIdentifier(n.Child(uint(i))); found != nil {
			return found
		}
	}
	return nil
}
