package sourceintelligence

func MQLScannerProfile(name string) ScannerProfile {
	profile := CPPScannerProfile()
	profile.Name = name
	profile.Keywords = append(profile.Keywords,
		"input", "sinput", "property", "import", "define",
	)
	return profile
}

func ObjectiveCScannerProfile(name string) ScannerProfile {
	profile := CPPScannerProfile()
	profile.Name = name
	profile.Keywords = append(profile.Keywords,
		"interface", "implementation", "protocol", "property", "end", "selector", "synthesize", "dynamic", "optional", "required",
	)
	profile.Strings = append([]StringRule{{Prefixes: []string{"@"}, Delimiter: "\"", BackslashEscapes: true}}, profile.Strings...)
	return profile
}

func DartScannerProfile() ScannerProfile {
	return ScannerProfile{
		Name: "dart",
		Keywords: []string{
			"abstract", "as", "async", "await", "base", "class", "const", "covariant", "enum", "extends", "extension", "external", "factory", "final", "function", "get", "implements", "import", "interface", "late", "library", "mixin", "on", "part", "required", "sealed", "set", "static", "typedef", "var", "void", "with",
		},
		Identifier:    DefaultIdentifierPolicy(),
		LineComments:  []string{"//"},
		BlockComments: []BlockCommentRule{{Start: "/*", End: "*/"}},
		Strings: []StringRule{
			{Prefixes: []string{"r", ""}, Delimiter: "\"\"\"", Multiline: true, BackslashEscapes: true, CaseInsensitivePrefix: true},
			{Prefixes: []string{"r", ""}, Delimiter: "'''", Multiline: true, BackslashEscapes: true, CaseInsensitivePrefix: true},
			{Prefixes: []string{"r", ""}, Delimiter: "\"", BackslashEscapes: true, CaseInsensitivePrefix: true},
			{Prefixes: []string{"r", ""}, Delimiter: "'", BackslashEscapes: true, CaseInsensitivePrefix: true},
		},
	}
}

func DScannerProfile() ScannerProfile {
	return ScannerProfile{
		Name: "d",
		Keywords: []string{
			"abstract", "alias", "class", "const", "enum", "extern", "final", "function", "immutable", "import", "in", "interface", "module", "override", "private", "protected", "public", "shared", "static", "struct", "this", "typedef", "union", "version", "void",
		},
		Identifier:   DefaultIdentifierPolicy(),
		LineComments: []string{"//"},
		BlockComments: []BlockCommentRule{
			{Start: "/+", End: "+/", Nestable: true},
			{Start: "/*", End: "*/"},
		},
		Strings: []StringRule{
			{Prefixes: []string{"r", "x", ""}, Delimiter: "\"", BackslashEscapes: true},
			{Prefixes: []string{""}, Delimiter: "`", Multiline: true},
			{Prefixes: []string{""}, Delimiter: "'", BackslashEscapes: true},
		},
	}
}

func SolidityScannerProfile() ScannerProfile {
	return ScannerProfile{
		Name: "solidity",
		Keywords: []string{
			"abstract", "address", "contract", "constructor", "enum", "error", "event", "external", "fallback", "function", "import", "interface", "internal", "is", "library", "mapping", "modifier", "override", "payable", "private", "public", "receive", "returns", "struct", "using", "view", "virtual",
		},
		Identifier:    DefaultIdentifierPolicy(),
		LineComments:  []string{"//"},
		BlockComments: []BlockCommentRule{{Start: "/*", End: "*/"}},
		Strings: []StringRule{
			{Prefixes: []string{"unicode", "hex", ""}, Delimiter: "\"", BackslashEscapes: true},
			{Prefixes: []string{"unicode", "hex", ""}, Delimiter: "'", BackslashEscapes: true},
		},
	}
}

func ApexScannerProfile() ScannerProfile {
	profile := JavaScannerProfile()
	profile.Name = "apex"
	profile.CaseInsensitive = true
	profile.Keywords = append(profile.Keywords,
		"trigger", "on", "before", "after", "insert", "update", "delete", "undelete", "with", "without", "sharing", "webservice", "global",
	)
	return profile
}

func ZigScannerProfile() ScannerProfile {
	return ScannerProfile{
		Name:         "zig",
		Keywords:     []string{"align", "allowzero", "anyframe", "anytype", "asm", "callconv", "comptime", "const", "enum", "error", "extern", "fn", "opaque", "packed", "pub", "struct", "test", "threadlocal", "union", "usingnamespace", "var", "volatile"},
		Identifier:   DefaultIdentifierPolicy(),
		LineComments: []string{"//"},
		Strings: []StringRule{
			{Prefixes: []string{"c", ""}, Delimiter: "\"", BackslashEscapes: true},
			{Prefixes: []string{""}, Delimiter: "'", BackslashEscapes: true},
		},
	}
}

func NimScannerProfile() ScannerProfile {
	return ScannerProfile{
		Name:            "nim",
		CaseInsensitive: true,
		Keywords:        []string{"const", "func", "import", "include", "iterator", "let", "method", "object", "proc", "ref", "template", "type", "var"},
		Identifier:      IdentifierPolicy{UnicodeLetters: true, UnicodeDigits: true, UnicodeMarks: true, Underscore: true, ExtraContinue: "*"},
		LineComments:    []string{"#"},
		BlockComments: []BlockCommentRule{
			{Start: "##[", End: "]##"},
			{Start: "#[", End: "]#", Nestable: true},
		},
		Strings: []StringRule{
			{Prefixes: []string{"fmt", "f", "r", ""}, Delimiter: "\"\"\"", Multiline: true, BackslashEscapes: true, CaseInsensitivePrefix: true},
			{Prefixes: []string{"fmt", "f", "r", ""}, Delimiter: "\"", BackslashEscapes: true, CaseInsensitivePrefix: true},
			{Prefixes: []string{""}, Delimiter: "'", BackslashEscapes: true},
		},
		Indentation: true,
	}
}

func ALScannerProfile() ScannerProfile {
	return ScannerProfile{
		Name:            "al",
		CaseInsensitive: true,
		Keywords:        []string{"actions", "begin", "codeunit", "enum", "enumextension", "extends", "field", "fields", "interface", "local", "namespace", "page", "pageextension", "permissionset", "procedure", "protected", "query", "report", "reportextension", "table", "tableextension", "using", "var", "xmlport"},
		Identifier:      DefaultIdentifierPolicy(),
		LineComments:    []string{"//"},
		BlockComments:   []BlockCommentRule{{Start: "/*", End: "*/"}},
		Strings: []StringRule{
			{Prefixes: []string{""}, Delimiter: "'", DoubledDelimiterEscape: true},
			{Prefixes: []string{""}, Delimiter: "\"", DoubledDelimiterEscape: true},
		},
	}
}
