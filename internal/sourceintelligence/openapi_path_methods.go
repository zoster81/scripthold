package sourceintelligence

import (
	"context"
	"strings"
)

// appendOpenAPIPathMethods indexes YAML HTTP method keys without an operationId.
// Path and method indentation are checked together so nested examples,
// responses and parameter descriptions cannot create operations.
func appendOpenAPIPathMethods(ctx context.Context, document *SourceDocument, lines []sourceTextLine, builder *SymbolBuilder) error {
	type method struct {
		path, verb  string
		line        sourceTextLine
		childIndent int
		hasID       bool
	}
	inPaths, pathIndent, methodIndent, path := false, -1, -1, ""
	var current *method
	finish := func() {
		if current != nil && !current.hasID {
			name := strings.ToUpper(current.verb) + " " + current.path
			trimStart := current.line.start + strings.Index(current.line.text, current.verb+":")
			addDocumentDataHardwareSymbol(builder, SymbolKindOperation, "http-method", name, nil,
				OffsetRange{Start: trimStart, End: current.line.end},
				OffsetRange{Start: trimStart, End: trimStart + len(current.verb)})
		}
		current = nil
	}
	for _, line := range lines {
		if err := ctx.Err(); err != nil {
			return err
		}
		trimmed := strings.TrimSpace(stripLineComment(line.text, "#"))
		if trimmed == "" {
			continue
		}
		if line.indent == 0 {
			finish()
			inPaths = trimmed == "paths:"
			pathIndent, methodIndent, path = -1, -1, ""
			continue
		}
		if !inPaths {
			continue
		}
		if pathIndent < 0 {
			if strings.HasSuffix(trimmed, ":") {
				route := unquoteSourceValue(strings.TrimSuffix(trimmed, ":"))
				if strings.HasPrefix(route, "/") {
					pathIndent, path, methodIndent = line.indent, route, -1
				}
			}
			continue
		}
		if line.indent <= pathIndent {
			finish()
			path, methodIndent = "", -1
			if line.indent == pathIndent && strings.HasSuffix(trimmed, ":") {
				route := unquoteSourceValue(strings.TrimSuffix(trimmed, ":"))
				if strings.HasPrefix(route, "/") {
					path = route
				}
			}
			continue
		}
		if path == "" {
			continue
		}
		if methodIndent < 0 {
			methodIndent = line.indent
		}
		if current != nil && line.indent <= methodIndent {
			finish()
		}
		if line.indent == methodIndent {
			word := strings.TrimSuffix(trimmed, ":")
			if strings.HasSuffix(trimmed, ":") && isOpenAPIHTTPMethod(word) {
				current = &method{path: path, verb: word, line: line, childIndent: -1}
			}
			continue
		}
		if current != nil && line.indent > methodIndent {
			if current.childIndent < 0 {
				current.childIndent = line.indent
			}
			if line.indent == current.childIndent && strings.HasPrefix(trimmed, "operationId:") {
				current.hasID = true
			}
		}
	}
	finish()
	return nil
}

func isOpenAPIHTTPMethod(word string) bool {
	switch word {
	case "get", "put", "post", "delete", "options", "head", "patch", "trace":
		return true
	}
	return false
}
