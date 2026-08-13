package sourceintelligence

// BlockCommentRule defines one shared block-comment family.
type BlockCommentRule struct {
	Start    string
	End      string
	Nestable bool
}

// StringRule describes one opaque string family. Prefixes are matched before
// Delimiter; repeated delimiters support raw quote runs such as C# raw strings.
type StringRule struct {
	Prefixes               []string
	Delimiter              string
	RepeatedDelimiterMin   int
	Multiline              bool
	BackslashEscapes       bool
	DoubledDelimiterEscape bool
	CaseInsensitivePrefix  bool
	InterpolationMarker    string
}

// ScannerProfile contains lexical behavior only. Declaration semantics belong
// to analyzers, not to this shared scanner.
type ScannerProfile struct {
	Name                 string
	CaseInsensitive      bool
	Keywords             []string
	LineComments         []string
	BlockComments        []BlockCommentRule
	Strings              []StringRule
	Directives           bool
	Indentation          bool
	ExplicitContinuation string
	ImplicitContinuation bool
}

// CSharpScannerProfile covers the lexical families needed by the R25 C# canary.
func CSharpScannerProfile() ScannerProfile {
	rawPrefixes := []string{"", "$", "$$", "$$$", "$$$$", "$$$$$", "$$$$$$", "$$$$$$$", "$$$$$$$$"}
	return ScannerProfile{
		Name: "csharp",
		Keywords: []string{
			"abstract", "async", "base", "bool", "byte", "char", "class", "const", "decimal", "delegate", "double",
			"enum", "event", "explicit", "extern", "false", "file", "float", "get", "implicit", "in", "int", "interface",
			"internal", "long", "namespace", "new", "null", "object", "operator", "out", "override", "partial", "private",
			"protected", "public", "readonly", "record", "ref", "required", "return", "sealed", "set", "short", "static",
			"string", "struct", "this", "true", "uint", "ulong", "unsafe", "ushort", "using", "virtual", "void", "volatile",
		},
		LineComments:  []string{"//"},
		BlockComments: []BlockCommentRule{{Start: "/*", End: "*/"}},
		Strings: []StringRule{
			{Prefixes: rawPrefixes, Delimiter: "\"", RepeatedDelimiterMin: 3, Multiline: true},
			{Prefixes: []string{"$@", "@$", "@"}, Delimiter: "\"", Multiline: true, DoubledDelimiterEscape: true, InterpolationMarker: "$"},
			{Prefixes: []string{"$", ""}, Delimiter: "\"", BackslashEscapes: true, InterpolationMarker: "$"},
			{Prefixes: []string{""}, Delimiter: "'", BackslashEscapes: true},
		},
		Directives: true,
	}
}

// VBNetScannerProfile covers the line-oriented lexical families needed by R25.
func VBNetScannerProfile() ScannerProfile {
	return ScannerProfile{
		Name:            "vbnet",
		CaseInsensitive: true,
		Keywords: []string{
			"ansi", "as", "async", "auto", "byref", "byval", "class", "const", "constructor", "custom", "declare", "default", "delegate", "dim", "end", "enum", "event",
			"friend", "function", "get", "implements", "imports", "inherits", "interface", "iterator", "module", "mustinherit", "mustoverride", "narrowing", "new",
			"notinheritable", "notoverridable", "overloads", "overridable", "overrides", "partial", "private", "property", "protected", "public", "readonly",
			"set", "shadows", "shared", "structure", "sub", "unicode", "widening", "writeonly",
		},
		LineComments:         []string{"'"},
		Strings:              []StringRule{{Prefixes: []string{""}, Delimiter: "\"", Multiline: true, DoubledDelimiterEscape: true}},
		Directives:           true,
		ExplicitContinuation: "_",
	}
}

// PythonScannerProfile covers indentation and opaque string families needed by R25.
func PythonScannerProfile() ScannerProfile {
	prefixes := []string{"", "b", "r", "u", "f", "br", "rb", "fr", "rf"}
	return ScannerProfile{
		Name: "python",
		Keywords: []string{
			"and", "as", "assert", "async", "await", "break", "class", "continue", "def", "del", "elif", "else", "except",
			"False", "finally", "for", "from", "global", "if", "import", "in", "is", "lambda", "None", "nonlocal", "not",
			"or", "pass", "raise", "return", "True", "try", "while", "with", "yield",
		},
		LineComments: []string{"#"},
		Strings: []StringRule{
			{Prefixes: prefixes, Delimiter: "\"\"\"", Multiline: true, BackslashEscapes: true, CaseInsensitivePrefix: true},
			{Prefixes: prefixes, Delimiter: "'''", Multiline: true, BackslashEscapes: true, CaseInsensitivePrefix: true},
			{Prefixes: prefixes, Delimiter: "\"", BackslashEscapes: true, CaseInsensitivePrefix: true},
			{Prefixes: prefixes, Delimiter: "'", BackslashEscapes: true, CaseInsensitivePrefix: true},
		},
		Indentation:          true,
		ExplicitContinuation: "\\",
		ImplicitContinuation: true,
	}
}
