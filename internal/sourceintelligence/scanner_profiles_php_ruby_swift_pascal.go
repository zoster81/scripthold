package sourceintelligence

// PHPScannerProfile covers declaration-oriented PHP syntax. Heredoc/nowdoc bodies
// are masked by the analyzer because their terminators are dynamic identifiers.
func PHPScannerProfile() ScannerProfile {
	return ScannerProfile{
		Name: "php",
		Keywords: []string{
			"abstract", "as", "class", "const", "enum", "extends", "final", "function", "implements", "include", "include_once",
			"interface", "namespace", "private", "protected", "public", "readonly", "require", "require_once", "static", "trait", "use",
		},
		Identifier:    IdentifierPolicy{UnicodeLetters: true, UnicodeDigits: true, UnicodeMarks: true, Underscore: true, ExtraStart: "$", ExtraContinue: "$"},
		LineComments:  []string{"//", "#"},
		BlockComments: []BlockCommentRule{{Start: "/*", End: "*/"}},
		Strings: []StringRule{
			{Prefixes: []string{""}, Delimiter: "\"", BackslashEscapes: true},
			{Prefixes: []string{""}, Delimiter: "'", BackslashEscapes: true},
		},
	}
}

// RubyScannerProfile covers Ruby declarations and explicit end-delimited scopes.
func RubyScannerProfile() ScannerProfile {
	return ScannerProfile{
		Name: "ruby",
		Keywords: []string{
			"begin", "case", "class", "def", "do", "else", "elsif", "end", "ensure", "extend", "for", "if", "include", "module",
			"private", "protected", "public", "require", "require_relative", "rescue", "unless", "until", "while",
		},
		Identifier:   IdentifierPolicy{UnicodeLetters: true, UnicodeDigits: true, UnicodeMarks: true, Underscore: true, ExtraStart: "@$", ExtraContinue: "@$?!="},
		LineComments: []string{"#"},
		Strings: []StringRule{
			{Prefixes: []string{""}, Delimiter: "\"", BackslashEscapes: true},
			{Prefixes: []string{""}, Delimiter: "'", BackslashEscapes: true},
		},
	}
}

// SwiftScannerProfile covers Swift declarations. Nested block comments and
// multiline strings are handled by the shared scanner.
func SwiftScannerProfile() ScannerProfile {
	return ScannerProfile{
		Name: "swift",
		Keywords: []string{
			"associatedtype", "class", "convenience", "deinit", "enum", "extension", "fileprivate", "final", "func", "import", "init",
			"internal", "let", "mutating", "nonisolated", "open", "override", "private", "protocol", "public", "required", "static", "struct",
			"typealias", "var",
		},
		LineComments:  []string{"//"},
		BlockComments: []BlockCommentRule{{Start: "/*", End: "*/", Nestable: true}},
		Strings: []StringRule{
			{Prefixes: []string{""}, Delimiter: "\"\"\"", Multiline: true, BackslashEscapes: true},
			{Prefixes: []string{""}, Delimiter: "\"", BackslashEscapes: true},
		},
	}
}

// PascalScannerProfile covers Pascal/Object Pascal's case-insensitive lexical
// core. Compiler directives remain opaque inside brace comments.
func PascalScannerProfile() ScannerProfile {
	return ScannerProfile{
		Name:            "pascal",
		CaseInsensitive: true,
		Keywords: []string{
			"begin", "case", "class", "const", "constructor", "destructor", "end", "forward", "function", "generic", "helper", "implementation",
			"interface", "object", "overload", "package", "private", "procedure", "program", "property", "protected", "public", "published", "record",
			"strict", "type", "unit", "uses", "var", "virtual",
		},
		LineComments:  []string{"//"},
		BlockComments: []BlockCommentRule{{Start: "{", End: "}", Nestable: true}, {Start: "(*", End: "*)", Nestable: true}},
		Strings:       []StringRule{{Prefixes: []string{""}, Delimiter: "'", DoubledDelimiterEscape: true}},
	}
}

func DelphiScannerProfile() ScannerProfile {
	profile := PascalScannerProfile()
	profile.Name = "delphi"
	profile.Keywords = append(profile.Keywords, "abstract", "automated", "classoperator", "deprecated", "experimental", "inline", "sealed", "static")
	return profile
}
