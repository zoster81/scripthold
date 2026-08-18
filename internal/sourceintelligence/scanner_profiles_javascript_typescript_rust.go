package sourceintelligence

// JavaScriptScannerProfile covers declaration-oriented ECMAScript and JSX lexical structure.
// Template literals are intentionally opaque for declaration analysis; expressions inside
// them are runtime expressions, not declaration scopes.
func JavaScriptScannerProfile() ScannerProfile {
	return ScannerProfile{
		Name: "javascript",
		Keywords: []string{
			"async", "await", "break", "case", "catch", "class", "const", "constructor", "continue", "debugger",
			"default", "delete", "do", "else", "export", "extends", "false", "finally", "for", "from", "function",
			"get", "if", "import", "in", "instanceof", "let", "new", "null", "of", "return", "set", "static",
			"super", "switch", "this", "throw", "true", "try", "typeof", "undefined", "var", "void", "while", "with", "yield",
		},
		Identifier: IdentifierPolicy{
			UnicodeLetters: true, UnicodeDigits: true, UnicodeMarks: true, Underscore: true,
			ExtraStart: "$#", ExtraContinue: "$#",
		},
		LineComments:  []string{"//"},
		BlockComments: []BlockCommentRule{{Start: "/*", End: "*/"}},
		Strings: []StringRule{
			{Prefixes: []string{""}, Delimiter: "`", Multiline: true, BackslashEscapes: true},
			{Prefixes: []string{""}, Delimiter: "\"", BackslashEscapes: true},
			{Prefixes: []string{""}, Delimiter: "'", BackslashEscapes: true},
		},
	}
}

// TypeScriptScannerProfile extends ECMAScript with declaration-oriented TypeScript keywords.
// JSX/TSX markup is consumed as part of containing expressions; it is not promoted to
// standalone declarations by the structural parser.
func TypeScriptScannerProfile() ScannerProfile {
	profile := JavaScriptScannerProfile()
	profile.Name = "typescript"
	profile.Keywords = append(profile.Keywords,
		"abstract", "any", "as", "asserts", "bigint", "boolean", "declare", "enum", "implements", "infer",
		"interface", "keyof", "module", "namespace", "never", "number", "object", "override", "private",
		"protected", "public", "readonly", "satisfies", "string", "symbol", "type", "unique", "unknown",
	)
	return profile
}

// RustScannerProfile covers Rust declaration syntax. Dynamic raw-string delimiters are
// masked by the Rust analyzer before this shared scanner runs so lifetimes remain distinct
// from character literals and macro bodies remain parser-owned.
func RustScannerProfile() ScannerProfile {
	return ScannerProfile{
		Name: "rust",
		Keywords: []string{
			"as", "async", "await", "break", "const", "continue", "crate", "dyn", "else", "enum", "extern",
			"false", "fn", "for", "if", "impl", "in", "let", "loop", "match", "mod", "move", "mut", "pub",
			"ref", "return", "self", "Self", "static", "struct", "super", "trait", "true", "type", "unsafe",
			"use", "where", "while",
		},
		Identifier:   DefaultIdentifierPolicy(),
		LineComments: []string{"//"},
		BlockComments: []BlockCommentRule{
			{Start: "/*", End: "*/", Nestable: true},
		},
		Strings: []StringRule{
			{Prefixes: []string{"b", "c", ""}, Delimiter: "\"", BackslashEscapes: true},
		},
	}
}
