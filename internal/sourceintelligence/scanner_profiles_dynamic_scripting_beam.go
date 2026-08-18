package sourceintelligence

func PerlScannerProfile() ScannerProfile {
	return ScannerProfile{
		Name:         "perl",
		Keywords:     []string{"package", "sub", "use", "require", "my", "our", "state", "constant"},
		Identifier:   IdentifierPolicy{UnicodeLetters: true, UnicodeDigits: true, UnicodeMarks: true, Underscore: true, ExtraStart: "$@%", ExtraContinue: "$@%:'"},
		LineComments: []string{"#"},
		Strings: []StringRule{
			{Prefixes: []string{""}, Delimiter: "\"", BackslashEscapes: true},
			{Prefixes: []string{""}, Delimiter: "'", BackslashEscapes: true},
		},
	}
}

func LuaScannerProfile(name string) ScannerProfile {
	return ScannerProfile{
		Name: name,
		Keywords: []string{
			"and", "break", "do", "else", "elseif", "end", "export", "for", "function", "if", "in", "local", "repeat", "return", "then", "type", "until", "while",
		},
		Identifier:   DefaultIdentifierPolicy(),
		LineComments: []string{"--"},
		Strings: []StringRule{
			{Prefixes: []string{""}, Delimiter: "\"", BackslashEscapes: true},
			{Prefixes: []string{""}, Delimiter: "'", BackslashEscapes: true},
		},
	}
}

func ElixirScannerProfile() ScannerProfile {
	return ScannerProfile{
		Name:         "elixir",
		Keywords:     []string{"alias", "case", "cond", "def", "defguard", "defmacro", "defmodule", "defp", "do", "else", "end", "fn", "for", "if", "import", "quote", "receive", "require", "try", "unless", "use", "with"},
		Identifier:   IdentifierPolicy{UnicodeLetters: true, UnicodeDigits: true, UnicodeMarks: true, Underscore: true, ExtraStart: "@", ExtraContinue: "?!@"},
		LineComments: []string{"#"},
		Strings: []StringRule{
			{Prefixes: []string{""}, Delimiter: "\"\"\"", Multiline: true, BackslashEscapes: true},
			{Prefixes: []string{""}, Delimiter: "'''", Multiline: true, BackslashEscapes: true},
			{Prefixes: []string{""}, Delimiter: "\"", Multiline: true, BackslashEscapes: true},
			{Prefixes: []string{""}, Delimiter: "'", Multiline: true, BackslashEscapes: true},
		},
	}
}

func ErlangScannerProfile() ScannerProfile {
	return ScannerProfile{
		Name:         "erlang",
		Keywords:     []string{"after", "begin", "case", "catch", "end", "fun", "if", "of", "receive", "try", "when"},
		Identifier:   IdentifierPolicy{UnicodeLetters: true, UnicodeDigits: true, UnicodeMarks: true, Underscore: true, ExtraStart: "@", ExtraContinue: "@"},
		LineComments: []string{"%"},
		Strings: []StringRule{
			{Prefixes: []string{""}, Delimiter: "\"", BackslashEscapes: true},
			{Prefixes: []string{""}, Delimiter: "'", BackslashEscapes: true},
		},
	}
}

func GleamScannerProfile() ScannerProfile {
	return ScannerProfile{
		Name:         "gleam",
		Keywords:     []string{"const", "fn", "import", "opaque", "pub", "type"},
		Identifier:   DefaultIdentifierPolicy(),
		LineComments: []string{"//"},
		Strings:      []StringRule{{Prefixes: []string{""}, Delimiter: "\"", BackslashEscapes: true}},
	}
}

func GroovyScannerProfile() ScannerProfile {
	return ScannerProfile{
		Name:          "groovy",
		Keywords:      []string{"abstract", "class", "def", "enum", "extends", "final", "implements", "import", "interface", "package", "private", "protected", "public", "static", "trait"},
		Identifier:    DefaultIdentifierPolicy(),
		LineComments:  []string{"//"},
		BlockComments: []BlockCommentRule{{Start: "/*", End: "*/"}},
		Strings: []StringRule{
			{Prefixes: []string{""}, Delimiter: "\"\"\"", Multiline: true, BackslashEscapes: true},
			{Prefixes: []string{""}, Delimiter: "'''", Multiline: true, BackslashEscapes: true},
			{Prefixes: []string{""}, Delimiter: "\"", BackslashEscapes: true},
			{Prefixes: []string{""}, Delimiter: "'", BackslashEscapes: true},
		},
	}
}

func ShellScannerProfile(name string) ScannerProfile {
	return ScannerProfile{
		Name:                         name,
		Keywords:                     []string{"case", "do", "done", "elif", "else", "esac", "fi", "for", "function", "if", "in", "select", "then", "until", "while"},
		Identifier:                   IdentifierPolicy{UnicodeLetters: true, UnicodeDigits: true, UnicodeMarks: true, Underscore: true, ExtraStart: "$", ExtraContinue: "$-"},
		LineComments:                 []string{"#"},
		LineCommentRequiresWordStart: true,
		Strings: []StringRule{
			{Prefixes: []string{""}, Delimiter: "\"", Multiline: true, BackslashEscapes: true},
			{Prefixes: []string{""}, Delimiter: "'", Multiline: true},
			{Prefixes: []string{""}, Delimiter: "`", Multiline: true, BackslashEscapes: true},
		},
		HereDocs: []HereDocRule{
			{Operator: "<<-", AllowQuotedDelimiter: true, StripLeadingTabs: true},
			{Operator: "<<", AllowQuotedDelimiter: true},
		},
	}
}

func TclScannerProfile() ScannerProfile {
	return ScannerProfile{
		Name:         "tcl",
		Keywords:     []string{"namespace", "package", "proc", "source"},
		Identifier:   IdentifierPolicy{UnicodeLetters: true, UnicodeDigits: true, UnicodeMarks: true, Underscore: true, ExtraStart: "$:", ExtraContinue: "$:-"},
		LineComments: []string{"#"},
		Strings:      []StringRule{{Prefixes: []string{""}, Delimiter: "\"", BackslashEscapes: true}},
	}
}

func AutoHotkeyScannerProfile() ScannerProfile {
	return ScannerProfile{
		Name:                         "autohotkey",
		CaseInsensitive:              true,
		Keywords:                     []string{"class", "extends", "static"},
		Identifier:                   DefaultIdentifierPolicy(),
		LineComments:                 []string{";"},
		LineCommentRequiresWordStart: true,
		BlockComments:                []BlockCommentRule{{Start: "/*", End: "*/"}},
		Strings:                      []StringRule{{Prefixes: []string{""}, Delimiter: "\"", DoubledDelimiterEscape: true}},
		DirectiveRules:               []DirectiveRule{{Prefix: "#", CaseInsensitive: true}},
		Directives:                   true,
	}
}
