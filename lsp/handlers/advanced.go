package handlers

import (
	"strings"

	"github.com/humberto/ruby-lsp-go/indexer"
	"github.com/humberto/ruby-lsp-go/parser"
)

// CodeLens handles textDocument/codeLens.
func CodeLens(ctx *Context, params interface{}) interface{} {
	uri := extractURI(params)
	doc, ok := ctx.Store.GetDocument(uri)
	if !ok {
		return []interface{}{}
	}

	parseResult := doc.ParseResult()
	if parseResult == nil || parseResult.AST == nil {
		return []interface{}{}
	}

	lenses := []interface{}{}
	collectTestLenses(parseResult.AST, uri, &lenses)
	return lenses
}

// CodeLensResolve handles codeLens/resolve.
func CodeLensResolve(ctx *Context, params interface{}) interface{} {
	if paramMap, ok := params.(map[string]interface{}); ok {
		if data, ok := paramMap["data"].(map[string]interface{}); ok {
			testName, _ := data["testName"].(string)
			filePath, _ := data["filePath"].(string)
			kind, _ := data["kind"].(string)

			title := "Run Test"
			if kind == "debug" {
				title = "Debug Test"
			}

			return map[string]interface{}{
				"range": paramMap["range"],
				"command": map[string]interface{}{
					"title":   title,
					"command": "rubyLspGo.runTest",
					"arguments": []interface{}{
						filePath,
						testName,
						kind,
					},
				},
			}
		}
	}
	return params
}

func collectTestLenses(node *parser.Node, uri string, lenses *[]interface{}) {
	if node == nil {
		return
	}

	if node.Type == parser.NodeMethod {
		name := node.Name
		if strings.HasPrefix(name, "test_") {
			*lenses = append(*lenses, testLens(node, uri, name, "run"))
			*lenses = append(*lenses, testLens(node, uri, name, "debug"))
		}
	}

	// RSpec-style: it 'description' blocks detected via call nodes named 'it' or 'describe'
	if node.Type == parser.NodeCall && (node.Name == "it" || node.Name == "describe") {
		label := node.Name
		if len(node.Children) > 0 && node.Children[0].Type == parser.NodeString {
			label = node.Children[0].Value
		}
		*lenses = append(*lenses, testLens(node, uri, label, "run"))
	}

	for _, child := range node.Children {
		collectTestLenses(child, uri, lenses)
	}
}

func testLens(node *parser.Node, uri, testName, kind string) map[string]interface{} {
	title := "▶ Run"
	if kind == "debug" {
		title = "🐛 Debug"
	}
	return map[string]interface{}{
		"range": map[string]interface{}{
			"start": map[string]interface{}{"line": node.Range.Start.Line, "character": 0},
			"end":   map[string]interface{}{"line": node.Range.Start.Line, "character": 80},
		},
		"command": map[string]interface{}{
			"title": title,
		},
		"data": map[string]interface{}{
			"testName": testName,
			"filePath": uriToPath(uri),
			"kind":     kind,
		},
	}
}

// CodeAction handles textDocument/codeAction.
func CodeAction(ctx *Context, params interface{}) interface{} {
	uri := extractURI(params)
	doc, ok := ctx.Store.GetDocument(uri)
	if !ok {
		return []interface{}{}
	}

	actions := []interface{}{}

	// RuboCop quick fix
	actions = append(actions, map[string]interface{}{
		"title": "Fix all RuboCop offenses",
		"kind":  "source.fixAll",
	})

	// Extract variable (if range selected)
	if paramMap, ok := params.(map[string]interface{}); ok {
		if ctx_, ok := paramMap["context"].(map[string]interface{}); ok {
			if rangeParam, ok := ctx_["range"].(map[string]interface{}); ok {
				startMap, _ := rangeParam["start"].(map[string]interface{})
				endMap, _ := rangeParam["end"].(map[string]interface{})
				startLine, _ := startMap["line"].(float64)
				startChar, _ := startMap["character"].(float64)
				endLine, _ := endMap["line"].(float64)
				endChar, _ := endMap["character"].(float64)

				selected := extractRange(doc.Source(), int(startLine), int(startChar), int(endLine), int(endChar))
				if selected != "" && !strings.Contains(selected, "\n") {
					actions = append(actions, map[string]interface{}{
						"title": "Extract variable",
						"kind":  "refactor.extract.variable",
						"edit": map[string]interface{}{
							"changes": map[string]interface{}{
								uri: []interface{}{
									map[string]interface{}{
										"range": map[string]interface{}{
											"start": map[string]interface{}{"line": int(startLine), "character": 0},
											"end":   map[string]interface{}{"line": int(startLine), "character": 0},
										},
										"newText": "variable = " + selected + "\n",
									},
								},
							},
						},
					})
				}
			}

			// Generate attr_accessor from @ivar in initialize
			if diags, ok := ctx_["diagnostics"].([]interface{}); ok {
				_ = diags
				actions = append(actions, map[string]interface{}{
					"title": "Generate attr_accessor",
					"kind":  "refactor.rewrite",
				})
			}
		}
	}

	return actions
}

// CodeActionResolve handles codeAction/resolve.
func CodeActionResolve(ctx *Context, params interface{}) interface{} {
	return params
}

func extractRange(source string, startLine, startChar, endLine, endChar int) string {
	lines := strings.Split(source, "\n")
	if startLine >= len(lines) {
		return ""
	}
	if startLine == endLine {
		line := lines[startLine]
		if endChar > len(line) {
			endChar = len(line)
		}
		if startChar > len(line) {
			startChar = len(line)
		}
		return line[startChar:endChar]
	}
	return ""
}

// DocumentLink handles textDocument/documentLink.
func DocumentLink(ctx *Context, params interface{}) interface{} {
	uri := extractURI(params)
	doc, ok := ctx.Store.GetDocument(uri)
	if !ok {
		return []interface{}{}
	}

	parseResult := doc.ParseResult()
	if parseResult == nil || parseResult.AST == nil {
		return []interface{}{}
	}

	links := []interface{}{}
	collectRequireLinks(parseResult.AST, uri, ctx.State.WorkspacePath, &links)
	return links
}

func collectRequireLinks(node *parser.Node, currentURI, workspaceRoot string, links *[]interface{}) {
	if node == nil {
		return
	}

	if node.Type == parser.NodeRequire {
		reqPath := node.Value
		if reqPath == "" && len(node.Children) > 0 {
			reqPath = strings.Trim(node.Children[0].Value, `"'`)
		}

		targetURI := resolveRequirePath(reqPath, currentURI, workspaceRoot)
		if targetURI != "" {
			*links = append(*links, map[string]interface{}{
				"range": map[string]interface{}{
					"start": map[string]interface{}{"line": node.Range.Start.Line, "character": node.Range.Start.Character},
					"end":   map[string]interface{}{"line": node.Range.End.Line, "character": node.Range.End.Character},
				},
				"target":  targetURI,
				"tooltip": reqPath,
			})
		}
	}

	for _, child := range node.Children {
		collectRequireLinks(child, currentURI, workspaceRoot, links)
	}
}

func resolveRequirePath(reqPath, currentURI, workspaceRoot string) string {
	if strings.HasPrefix(reqPath, ".") {
		// require_relative
		baseDir := uriToPath(currentURI)
		lastSlash := strings.LastIndex(baseDir, "/")
		if lastSlash >= 0 {
			baseDir = baseDir[:lastSlash]
		}
		resolved := baseDir + "/" + reqPath
		if !strings.HasSuffix(resolved, ".rb") {
			resolved += ".rb"
		}
		return pathToURI(resolved)
	}

	// require 'gem/path'
	if workspaceRoot != "" {
		candidates := []string{
			workspaceRoot + "/lib/" + reqPath + ".rb",
			workspaceRoot + "/" + reqPath + ".rb",
		}
		for _, c := range candidates {
			return pathToURI(c)
		}
	}
	return ""
}

// PrepareTypeHierarchy handles textDocument/prepareTypeHierarchy.
func PrepareTypeHierarchy(ctx *Context, params interface{}) interface{} {
	uri, pos := extractPosition(params)
	if ctx.Index == nil {
		return nil
	}

	doc, ok := ctx.Store.GetDocument(uri)
	if !ok {
		return nil
	}

	word := parser.GetWordAtPosition(doc.Source(), pos.Line, pos.Character)
	if word == "" {
		return nil
	}

	entries := ctx.Index.LookupLegacy(word)
	if len(entries) == 0 {
		return nil
	}

	entry := entries[0]
	if entry.Type != indexer.SymbolClass && entry.Type != indexer.SymbolModule {
		return nil
	}

	return []interface{}{
		map[string]interface{}{
			"name": entry.FullyQualifiedName,
			"kind": indexer.SymbolKindToLSP(entry.Type),
			"uri":  pathToURI(entry.FilePath),
			"range": map[string]interface{}{
				"start": map[string]interface{}{"line": entry.Line - 1, "character": entry.Character},
				"end":   map[string]interface{}{"line": entry.Line - 1, "character": entry.Character + len(entry.Name)},
			},
		},
	}
}

// TypeHierarchySupertypes handles typeHierarchy/supertypes.
func TypeHierarchySupertypes(ctx *Context, params interface{}) interface{} {
	if ctx.Index == nil {
		return nil
	}

	itemName := ""
	if paramMap, ok := params.(map[string]interface{}); ok {
		if item, ok := paramMap["item"].(map[string]interface{}); ok {
			itemName, _ = item["name"].(string)
		}
	}

	if itemName == "" {
		return nil
	}

	ancestors := ctx.Index.LinearizedAncestors(itemName)
	items := []interface{}{}
	for _, ancestor := range ancestors {
		entries := ctx.Index.LookupLegacy(ancestor)
		if len(entries) == 0 {
			continue
		}
		entry := entries[0]
		items = append(items, map[string]interface{}{
			"name": entry.FullyQualifiedName,
			"kind": indexer.SymbolKindToLSP(entry.Type),
			"uri":  pathToURI(entry.FilePath),
			"range": map[string]interface{}{
				"start": map[string]interface{}{"line": entry.Line - 1, "character": entry.Character},
				"end":   map[string]interface{}{"line": entry.Line - 1, "character": entry.Character + len(entry.Name)},
			},
		})
	}
	return items
}
