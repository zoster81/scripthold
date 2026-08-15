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

// IdentifierPolicy keeps language-specific identifier spelling out of the
// scanner core. The zero value is normalized to DefaultIdentifierPolicy.
type IdentifierPolicy struct {
	UnicodeLetters bool
	UnicodeDigits  bool
	UnicodeMarks   bool
	Underscore     bool
	ExtraStart     string
	ExtraContinue  string
}

func DefaultIdentifierPolicy() IdentifierPolicy {
	return IdentifierPolicy{UnicodeLetters: true, UnicodeDigits: true, UnicodeMarks: true, Underscore: true}
}

// DelimiterRule defines one balanced lexical delimiter pair. Multi-byte pairs
// are supported so later template/DSL analyzers do not require scanner forks.
type DelimiterRule struct {
	Open  string
	Close string
}

// DirectiveRule defines one line-start directive prefix. Leading horizontal
// whitespace is allowed because the scanner retains logical line-start state.
type DirectiveRule struct {
	Prefix          string
	CaseInsensitive bool
}

// HereDocRule defines one shell-like heredoc opener. The delimiter is parsed
// from the opening logical line and bodies are consumed in declaration order.
type HereDocRule struct {
	Operator             string
	AllowQuotedDelimiter bool
	StripLeadingTabs     bool
}

// ScannerProfile contains lexical behavior only. Declaration semantics belong
// to analyzers, not to this shared scanner.
type ScannerProfile struct {
	Name                 string
	CaseInsensitive      bool
	Keywords             []string
	Identifier           IdentifierPolicy
	LineComments         []string
	BlockComments        []BlockCommentRule
	Strings              []StringRule
	Delimiters           []DelimiterRule
	DirectiveRules       []DirectiveRule
	HereDocs             []HereDocRule
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

// CScannerProfile covers ISO C declaration-level lexical structure. Preprocessor
// directives remain opaque tokens so macro bodies cannot leak declarations.
func CScannerProfile() ScannerProfile {
	return ScannerProfile{
		Name: "c",
		Keywords: []string{
			"_Atomic", "_Bool", "_Complex", "_Noreturn", "auto", "break", "case", "char", "const", "continue", "default", "do", "double", "else", "enum", "extern", "float", "for", "goto", "if", "inline", "int", "long", "register", "restrict", "return", "short", "signed", "sizeof", "static", "struct", "switch", "typedef", "union", "unsigned", "void", "volatile", "while",
		},
		LineComments:  []string{"//"},
		BlockComments: []BlockCommentRule{{Start: "/*", End: "*/"}},
		Strings: []StringRule{
			{Prefixes: []string{"u8", "u", "U", "L", ""}, Delimiter: "\"", BackslashEscapes: true},
			{Prefixes: []string{"u", "U", "L", ""}, Delimiter: "'", BackslashEscapes: true},
		},
		Directives: true,
	}
}

// CPPScannerProfile extends C with declaration-oriented C++ keywords. Dynamic
// raw strings are handled conservatively by the C++ analyzer.
func CPPScannerProfile() ScannerProfile {
	profile := CScannerProfile()
	profile.Name = "cpp"
	profile.Keywords = append(profile.Keywords,
		"alignas", "alignof", "and", "and_eq", "asm", "bitand", "bitor", "bool", "catch", "char16_t", "char32_t", "char8_t",
		"class", "compl", "concept", "consteval", "constexpr", "constinit", "const_cast", "co_await", "co_return", "co_yield", "decltype", "delete", "dynamic_cast", "explicit", "export", "false", "friend", "mutable", "namespace", "new", "noexcept", "not", "not_eq", "nullptr", "operator", "or", "or_eq", "override", "private", "protected", "public", "reinterpret_cast", "requires", "static_assert", "static_cast", "template", "this", "thread_local", "throw", "true", "try", "typeid", "typename", "using", "virtual", "wchar_t", "xor", "xor_eq",
	)
	return profile
}

// JavaScannerProfile covers declarations, comments, ordinary literals and text blocks.
func JavaScannerProfile() ScannerProfile {
	return ScannerProfile{
		Name: "java",
		Keywords: []string{
			"abstract", "assert", "boolean", "break", "byte", "case", "catch", "char", "class", "const", "continue", "default", "do", "double", "else", "enum", "exports", "extends", "final", "finally", "float", "for", "goto", "if", "implements", "import", "instanceof", "int", "interface", "long", "module", "native", "new", "non", "open", "opens", "package", "permits", "private", "protected", "provides", "public", "record", "requires", "return", "sealed", "short", "static", "strictfp", "super", "switch", "synchronized", "this", "throw", "throws", "to", "transient", "transitive", "try", "uses", "var", "void", "volatile", "while", "with", "yield",
		},
		LineComments:  []string{"//"},
		BlockComments: []BlockCommentRule{{Start: "/*", End: "*/"}},
		Strings: []StringRule{
			{Prefixes: []string{""}, Delimiter: "\"\"\"", Multiline: true, BackslashEscapes: true},
			{Prefixes: []string{""}, Delimiter: "\"", BackslashEscapes: true},
			{Prefixes: []string{""}, Delimiter: "'", BackslashEscapes: true},
		},
	}
}

// KotlinScannerProfile covers Kotlin declarations and raw/ordinary strings.
func KotlinScannerProfile() ScannerProfile {
	return ScannerProfile{
		Name: "kotlin",
		Keywords: []string{
			"abstract", "actual", "annotation", "as", "class", "companion", "const", "constructor", "crossinline", "data", "delegate", "dynamic", "enum", "expect", "external", "final", "fun", "in", "infix", "inline", "inner", "interface", "internal", "lateinit", "noinline", "object", "open", "operator", "out", "override", "package", "private", "protected", "public", "reified", "sealed", "suspend", "tailrec", "typealias", "val", "var", "vararg", "where",
		},
		LineComments:  []string{"//"},
		BlockComments: []BlockCommentRule{{Start: "/*", End: "*/", Nestable: true}},
		Strings: []StringRule{
			{Prefixes: []string{""}, Delimiter: "\"\"\"", Multiline: true},
			{Prefixes: []string{""}, Delimiter: "\"", BackslashEscapes: true},
			{Prefixes: []string{""}, Delimiter: "'", BackslashEscapes: true},
		},
	}
}
