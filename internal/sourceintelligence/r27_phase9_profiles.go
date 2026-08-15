package sourceintelligence

func FortranScannerProfile() ScannerProfile {
	return ScannerProfile{
		Name:            "fortran",
		CaseInsensitive: true,
		Keywords: []string{
			"abstract", "block", "contains", "end", "function", "interface", "module", "procedure", "program", "submodule", "subroutine", "type", "use",
		},
		Identifier:   IdentifierPolicy{UnicodeLetters: true, UnicodeDigits: true, UnicodeMarks: true, Underscore: true},
		LineComments: []string{"!"},
		Strings: []StringRule{
			{Prefixes: []string{""}, Delimiter: "\"", DoubledDelimiterEscape: true},
			{Prefixes: []string{""}, Delimiter: "'", DoubledDelimiterEscape: true},
		},
		ExplicitContinuation: "&",
	}
}

func COBOLScannerProfile() ScannerProfile {
	return ScannerProfile{
		Name:            "cobol",
		CaseInsensitive: true,
		Keywords: []string{
			"class-id", "copy", "division", "end-program", "function-id", "identification", "method-id", "program-id", "section",
		},
		Identifier:   IdentifierPolicy{UnicodeLetters: true, UnicodeDigits: true, UnicodeMarks: true, Underscore: true, ExtraContinue: "-"},
		LineComments: []string{"*>"},
		Strings: []StringRule{
			{Prefixes: []string{""}, Delimiter: "\"", DoubledDelimiterEscape: true},
			{Prefixes: []string{""}, Delimiter: "'", DoubledDelimiterEscape: true},
		},
	}
}

func AdaScannerProfile() ScannerProfile {
	return ScannerProfile{
		Name:            "ada",
		CaseInsensitive: true,
		Keywords: []string{
			"body", "end", "function", "is", "package", "private", "procedure", "protected", "record", "renames", "subtype", "task", "type", "use", "with",
		},
		Identifier:   IdentifierPolicy{UnicodeLetters: true, UnicodeDigits: true, UnicodeMarks: true, Underscore: true},
		LineComments: []string{"--"},
		Strings:      []StringRule{{Prefixes: []string{""}, Delimiter: "\"", DoubledDelimiterEscape: true}},
	}
}

func MATLABScannerProfile(name string) ScannerProfile {
	comments := []string{"%"}
	if name == "octave" {
		comments = []string{"%", "#"}
	}
	return ScannerProfile{
		Name: name,
		Keywords: []string{
			"classdef", "end", "endclassdef", "endfunction", "function", "import", "methods", "pkg", "properties",
		},
		Identifier:   IdentifierPolicy{UnicodeLetters: true, UnicodeDigits: true, UnicodeMarks: true, Underscore: true, ExtraContinue: "."},
		LineComments: comments,
		BlockComments: []BlockCommentRule{
			{Start: "%{", End: "%}"},
		},
		Strings: []StringRule{
			{Prefixes: []string{""}, Delimiter: "\"", DoubledDelimiterEscape: true},
		},
		ExplicitContinuation: "...",
	}
}

func JuliaScannerProfile() ScannerProfile {
	return ScannerProfile{
		Name: "julia",
		Keywords: []string{
			"abstract", "baremodule", "const", "end", "function", "import", "macro", "module", "mutable", "primitive", "struct", "type", "using",
		},
		Identifier:    IdentifierPolicy{UnicodeLetters: true, UnicodeDigits: true, UnicodeMarks: true, Underscore: true, ExtraContinue: "!?"},
		LineComments:  []string{"#"},
		BlockComments: []BlockCommentRule{{Start: "#=", End: "=#", Nestable: true}},
		Strings: []StringRule{
			{Prefixes: []string{"raw", ""}, Delimiter: "\"\"\"", Multiline: true, BackslashEscapes: true},
			{Prefixes: []string{"raw", ""}, Delimiter: "\"", BackslashEscapes: true},
		},
	}
}

func RScannerProfile() ScannerProfile {
	return ScannerProfile{
		Name:         "r",
		Keywords:     []string{"function", "library", "require"},
		Identifier:   IdentifierPolicy{UnicodeLetters: true, UnicodeDigits: true, UnicodeMarks: true, Underscore: true, ExtraStart: ".", ExtraContinue: "."},
		LineComments: []string{"#"},
		Strings: []StringRule{
			{Prefixes: []string{""}, Delimiter: "\"", BackslashEscapes: true},
			{Prefixes: []string{""}, Delimiter: "'", BackslashEscapes: true},
		},
	}
}

func HaskellScannerProfile() ScannerProfile {
	return ScannerProfile{
		Name: "haskell",
		Keywords: []string{
			"as", "class", "data", "deriving", "hiding", "import", "instance", "module", "newtype", "qualified", "safe", "type", "where",
		},
		Identifier:    IdentifierPolicy{UnicodeLetters: true, UnicodeDigits: true, UnicodeMarks: true, Underscore: true, ExtraContinue: ".'"},
		LineComments:  []string{"--"},
		BlockComments: []BlockCommentRule{{Start: "{-", End: "-}", Nestable: true}},
		Strings: []StringRule{
			{Prefixes: []string{""}, Delimiter: "\"", BackslashEscapes: true},
		},
	}
}

func OCamlScannerProfile() ScannerProfile {
	return ScannerProfile{
		Name: "ocaml",
		Keywords: []string{
			"and", "class", "end", "include", "let", "module", "open", "rec", "struct", "type",
		},
		Identifier:    IdentifierPolicy{UnicodeLetters: true, UnicodeDigits: true, UnicodeMarks: true, Underscore: true, ExtraContinue: "'"},
		BlockComments: []BlockCommentRule{{Start: "(*", End: "*)", Nestable: true}},
		Strings:       []StringRule{{Prefixes: []string{""}, Delimiter: "\"", BackslashEscapes: true}},
	}
}

func CommonLispScannerProfile() ScannerProfile {
	profile := phase9LispScannerProfile("common-lisp", []string{"defclass", "defconstant", "defmacro", "defpackage", "defparameter", "defstruct", "defun", "defvar", "in-package", "require"})
	profile.BlockComments = []BlockCommentRule{{Start: "#|", End: "|#", Nestable: true}}
	return profile
}

func ClojureScannerProfile() ScannerProfile {
	return phase9LispScannerProfile("clojure", []string{"def", "defmacro", "defn", "defn-", "defprotocol", "defrecord", "deftype", "ns", "require"})
}

func EmacsLispScannerProfile() ScannerProfile {
	return phase9LispScannerProfile("emacs-lisp", []string{"defclass", "defconst", "defcustom", "defmacro", "defun", "defvar", "provide", "require"})
}

func phase9LispScannerProfile(name string, keywords []string) ScannerProfile {
	return ScannerProfile{
		Name:         name,
		Keywords:     keywords,
		Identifier:   IdentifierPolicy{UnicodeLetters: true, UnicodeDigits: true, UnicodeMarks: true, Underscore: true, ExtraStart: ":*+-/<>=!?", ExtraContinue: ":*+-.?/<>=!"},
		LineComments: []string{";"},
		Strings:      []StringRule{{Prefixes: []string{""}, Delimiter: "\"", BackslashEscapes: true}},
		Delimiters:   []DelimiterRule{{Open: "(", Close: ")"}, {Open: "[", Close: "]"}, {Open: "{", Close: "}"}},
	}
}
