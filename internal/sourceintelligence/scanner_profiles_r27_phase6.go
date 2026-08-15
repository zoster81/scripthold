package sourceintelligence

func BasicScannerProfile(name string) ScannerProfile {
	return ScannerProfile{
		Name: name, CaseInsensitive: true,
		Keywords: []string{
			"as", "attribute", "byref", "byval", "class", "const", "constructor", "declare", "def", "dim", "end", "endclass", "endenumeration", "endfunction", "endinterface", "endmodule", "endprocedure", "endstructure", "enum", "enumeration", "event", "function", "get", "global", "implements", "in", "interface", "lib", "let", "module", "namespace", "private", "procedure", "property", "protected", "public", "rem", "set", "shared", "static", "structure", "sub", "type", "xincludefile", "includefile",
		},
		Identifier:    IdentifierPolicy{UnicodeLetters: true, UnicodeDigits: true, UnicodeMarks: true, Underscore: true, ExtraContinue: "$%&!#"},
		LineComments:  []string{"'"},
		BlockComments: []BlockCommentRule{{Start: "/'", End: "'/"}},
		Strings:       []StringRule{{Prefixes: []string{""}, Delimiter: "\"", DoubledDelimiterEscape: true}},
		Directives:    true, ExplicitContinuation: "_",
	}
}

func VB6ScannerProfile() ScannerProfile          { return BasicScannerProfile("vb6") }
func VBAScannerProfile() ScannerProfile          { return BasicScannerProfile("vba") }
func VBScriptScannerProfile() ScannerProfile     { return BasicScannerProfile("vbscript") }
func QBasicScannerProfile() ScannerProfile       { return BasicScannerProfile("qbasic") }
func ClassicBasicScannerProfile() ScannerProfile { return BasicScannerProfile("classic-basic") }
func FreeBasicScannerProfile() ScannerProfile    { return BasicScannerProfile("freebasic") }
func PureBasicScannerProfile() ScannerProfile {
	profile := BasicScannerProfile("purebasic")
	profile.LineComments = []string{";"}
	profile.ExplicitContinuation = ""
	return profile
}
