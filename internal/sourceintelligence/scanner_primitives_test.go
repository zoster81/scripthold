package sourceintelligence

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
	"unsafe"

	"github.com/zoster81/scripthold/internal/operation"
)

func TestTokenRepresentationRemainsCompact(t *testing.T) {
	var kind TokenKind
	if size := unsafe.Sizeof(kind); size != 1 {
		t.Fatalf("TokenKind size = %d bytes, want 1", size)
	}
	maxTokenSize := uintptr(6) * unsafe.Sizeof(int(0))
	if size := unsafe.Sizeof(Token{}); size > maxTokenSize {
		t.Fatalf("Token size = %d bytes, want <= %d", size, maxTokenSize)
	}
}

func TestScannerCaseInsensitiveKeywordsPreserveUnicodeLowerSemantics(t *testing.T) {
	profile := ScannerProfile{
		Name:            "case-insensitive-keywords",
		CaseInsensitive: true,
		Keywords:        []string{"σ", "k"},
	}
	result := scanSourceText(t, "Σ σ ς K k", profile, scannerTestLimits)
	var got []Token
	for _, token := range result.Tokens {
		if token.Kind == TokenIdentifier || token.Kind == TokenKeyword {
			got = append(got, token)
		}
	}
	wantText := []string{"Σ", "σ", "ς", "K", "k"}
	wantKind := []TokenKind{TokenKeyword, TokenKeyword, TokenIdentifier, TokenKeyword, TokenKeyword}
	if len(got) != len(wantText) {
		t.Fatalf("case-insensitive identifier count = %d, want %d: %+v", len(got), len(wantText), got)
	}
	for index := range got {
		if got[index].Text != wantText[index] || got[index].Kind != wantKind[index] {
			t.Fatalf("token %d = {%q %v}, want {%q %v}", index, got[index].Text, got[index].Kind, wantText[index], wantKind[index])
		}
	}
}

func TestScannerCaseInsensitiveKeywordHashCollisionsAreVerified(t *testing.T) {
	hash := caseInsensitiveKeywordHash("target")
	scanner := sourceScanner{
		profile:                 ScannerProfile{CaseInsensitive: true},
		caseInsensitiveKeywords: map[uint64]string{hash: "different"},
		caseInsensitiveKeywordCollisions: map[uint64][]string{
			hash: {"TARGET"},
		},
	}
	if !scanner.isKeyword("target") {
		t.Fatal("verified collision bucket did not find the matching keyword")
	}
	delete(scanner.caseInsensitiveKeywordCollisions, hash)
	if scanner.isKeyword("target") {
		t.Fatal("hash collision produced a false keyword")
	}
}

func TestScannerCaseInsensitiveKeywordLookupDoesNotAllocatePerIdentifier(t *testing.T) {
	profile := FortranScannerProfile()
	text := strings.Repeat("MoDuLe Alpha FuNcTiOn Beta SuBrOuTiNe Gamma\n", 512)
	document := sourceDocumentForScanner(text)

	var result ScanResult
	var scanErr error
	allocations := testing.AllocsPerRun(5, func() {
		result, scanErr = ScanSource(context.Background(), document, profile, scannerTestLimits)
	})
	if scanErr != nil {
		t.Fatal(scanErr)
	}
	if !result.Complete {
		t.Fatalf("case-insensitive scan is partial: %+v", result.Diagnostics)
	}
	if allocations > 64 {
		t.Fatalf("case-insensitive scanner allocations = %.0f, want <= 64", allocations)
	}
}

func TestScannerProfileIdentifierDelimiterAndDirectivePolicies(t *testing.T) {
	profile := ScannerProfile{
		Name:     "r27-profiled",
		Keywords: []string{"begin", "end"},
		Identifier: IdentifierPolicy{
			UnicodeLetters: true,
			UnicodeDigits:  true,
			UnicodeMarks:   true,
			Underscore:     true,
			ExtraStart:     "$",
			ExtraContinue:  "$",
		},
		Delimiters:     []DelimiterRule{{Open: "(", Close: ")"}, {Open: "<%", Close: "%>"}},
		DirectiveRules: []DirectiveRule{{Prefix: "%"}},
	}
	text := "%pragma once\nbegin <% $value(foo) %> end\n"
	result := scanSourceText(t, text, profile, scannerTestLimits)
	if !result.Complete || len(result.Diagnostics) != 0 {
		t.Fatalf("profiled scan incomplete: %+v", result.Diagnostics)
	}
	if countKind(result.Tokens, TokenDirective) != 1 {
		t.Fatalf("directive count = %d, want 1", countKind(result.Tokens, TokenDirective))
	}
	if !hasIdentifier(result.Tokens, "$value") {
		t.Fatalf("profile-specific identifier missing: %+v", result.Tokens)
	}
	if result.MaxDepth != 2 {
		t.Fatalf("max depth = %d, want 2", result.MaxDepth)
	}
	pairs := PairDelimiterTokens(result.Tokens, profile.Delimiters)
	if len(pairs) != 4 {
		t.Fatalf("delimiter pair map entries = %d, want 4", len(pairs))
	}
}

func TestScannerDelimiterDispatchPreservesUTF8Delimiters(t *testing.T) {
	profile := ScannerProfile{
		Name:       "utf8-delimiters",
		Delimiters: []DelimiterRule{{Open: "«", Close: "»"}},
	}
	result := scanSourceText(t, "«value»\n", profile, scannerTestLimits)
	if !result.Complete || len(result.Diagnostics) != 0 {
		t.Fatalf("UTF-8 delimiter scan reported partial: %+v", result.Diagnostics)
	}
	pairs := PairDelimiterTokens(result.Tokens, profile.Delimiters)
	if len(pairs) != 2 {
		t.Fatalf("UTF-8 delimiter pair map entries = %d, want 2 tokens=%+v", len(pairs), result.Tokens)
	}
}

func TestPairDelimiterTokensDensePairingAllocationBounded(t *testing.T) {
	const repeats = 1024
	text := strings.Repeat("call(alpha[beta{gamma(delta)}], other);\n", repeats)
	profile := CSharpScannerProfile()
	scan, err := ScanSource(context.Background(), sourceDocumentForScanner(text), profile, ScannerLimits{
		MaxTokens: scannerTokenBudget(text), MaxTokenBytes: 1024 * 1024, MaxNesting: 256,
	})
	if err != nil {
		t.Fatal(err)
	}
	const wantPairEntries = repeats * 8
	allocations := testing.AllocsPerRun(5, func() {
		pairs := PairDelimiterTokens(scan.Tokens, profile.Delimiters)
		if len(pairs) != wantPairEntries {
			t.Fatalf("delimiter pair entries = %d, want %d", len(pairs), wantPairEntries)
		}
	})
	if allocations > 40 {
		t.Fatalf("dense delimiter pairing allocations = %.0f, want <= 40", allocations)
	}
}

func TestPairDelimiterTokensDelayedDensePairingAllocationBounded(t *testing.T) {
	const repeats = 1024
	text := strings.Repeat("value ", 600) + strings.Repeat("call(alpha[beta{gamma(delta)}], other);\n", repeats)
	profile := CSharpScannerProfile()
	scan, err := ScanSource(context.Background(), sourceDocumentForScanner(text), profile, ScannerLimits{
		MaxTokens: scannerTokenBudget(text), MaxTokenBytes: 1024 * 1024, MaxNesting: 256,
	})
	if err != nil {
		t.Fatal(err)
	}
	const wantPairEntries = repeats * 8
	allocations := testing.AllocsPerRun(5, func() {
		pairs := PairDelimiterTokens(scan.Tokens, profile.Delimiters)
		if len(pairs) != wantPairEntries {
			t.Fatalf("delayed delimiter pair entries = %d, want %d", len(pairs), wantPairEntries)
		}
	})
	if allocations > 40 {
		t.Fatalf("delayed dense delimiter pairing allocations = %.0f, want <= 40", allocations)
	}
}

func TestPairDelimiterTokensSparseInputAllocationBounded(t *testing.T) {
	tokens := make([]Token, 32*1024)
	for index := range tokens {
		tokens[index] = Token{Kind: TokenIdentifier, Text: "value"}
	}
	allocations := testing.AllocsPerRun(5, func() {
		pairs := PairDelimiterTokens(tokens, nil)
		if len(pairs) != 0 {
			t.Fatalf("sparse delimiter pairs = %d, want 0", len(pairs))
		}
	})
	if allocations > 16 {
		t.Fatalf("sparse delimiter pairing allocations = %.0f, want <= 16", allocations)
	}
}

func TestScannerDirectiveBackslashContinuationsStayOpaque(t *testing.T) {
	for _, testCase := range []struct {
		name string
		eol  string
	}{
		{name: "LF", eol: "\n"},
		{name: "CRLF", eol: "\r\n"},
		{name: "CR", eol: "\r"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			text := "#define JSON_CAST(T, expr) (__extension__ ({ \\" + testCase.eol +
				"    ((T) (expr)); \\" + testCase.eol +
				"}))" + testCase.eol +
				"struct After { int value; };" + testCase.eol
			result := scanSourceText(t, text, CPPScannerProfile(), scannerTestLimits)
			if !result.Complete || len(result.Diagnostics) != 0 {
				t.Fatalf("continued C/C++ directive leaked delimiter state: %+v", result.Diagnostics)
			}
			if countKind(result.Tokens, TokenDirective) != 1 {
				t.Fatalf("continued directive count = %d, want 1", countKind(result.Tokens, TokenDirective))
			}
			if !hasIdentifier(result.Tokens, "After") {
				t.Fatalf("declaration after continued directive is missing: %+v", result.Tokens)
			}
		})
	}
}

func TestScannerBackslashEscapedPhysicalNewlinesHandleCRLFAtomically(t *testing.T) {
	profile := ScannerProfile{
		Name:    "r27-escaped-newline",
		Strings: []StringRule{{Prefixes: []string{""}, Delimiter: "\"", BackslashEscapes: true}},
	}
	for _, testCase := range []struct {
		name     string
		text     string
		complete bool
	}{
		{name: "LF continuation", text: "value = \"alpha\\\nbeta\"\n", complete: true},
		{name: "CRLF continuation", text: "value = \"alpha\\\r\nbeta\"\r\n", complete: true},
		{name: "CR continuation", text: "value = \"alpha\\\rbeta\"\r", complete: true},
		{name: "unescaped CRLF", text: "value = \"alpha\r\nbeta\"\r\n", complete: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			result := scanSourceText(t, testCase.text, profile, scannerTestLimits)
			if result.Complete != testCase.complete {
				t.Fatalf("coverage complete=%v want %v diagnostics=%+v", result.Complete, testCase.complete, result.Diagnostics)
			}
			if testCase.complete && len(result.Diagnostics) != 0 {
				t.Fatalf("valid escaped physical newline reported diagnostics: %+v", result.Diagnostics)
			}
			if !testCase.complete && !hasScannerDiagnostic(result.Diagnostics, "unterminated-string") {
				t.Fatalf("unescaped physical newline missing unterminated-string: %+v", result.Diagnostics)
			}
		})
	}

	interpolated := ScannerProfile{
		Name:    "r27-interpolated-escaped-newline",
		Strings: []StringRule{{Prefixes: []string{"$"}, Delimiter: "\"", BackslashEscapes: true, InterpolationMarker: "$"}},
	}
	result := scanSourceText(t, "$\"alpha\\\r\nbeta\"\r\n", interpolated, scannerTestLimits)
	if !result.Complete || len(result.Diagnostics) != 0 {
		t.Fatalf("interpolated escaped CRLF reported partial: %+v", result.Diagnostics)
	}
}

func TestScannerCustomEscapePrefixPreservesQuotedStringBoundaries(t *testing.T) {
	profile := ScannerProfile{
		Name:    "custom-escape-prefix",
		Strings: []StringRule{{Prefixes: []string{""}, Delimiter: "\"", Multiline: true, EscapePrefix: "`"}},
	}
	text := "value = \"C:\\tools\\\"\nquoted = \"say `\"hello`\"\"\nmultiline = \"alpha\nbeta\"\n"
	result := scanSourceText(t, text, profile, scannerTestLimits)
	if !result.Complete || len(result.Diagnostics) != 0 {
		t.Fatalf("custom escape-prefix strings reported partial: %+v", result.Diagnostics)
	}
	if countKind(result.Tokens, TokenString) != 3 {
		t.Fatalf("custom escape-prefix string count=%d want 3 tokens=%+v", countKind(result.Tokens, TokenString), result.Tokens)
	}
}

func TestScannerStringDispatchPreservesCaseInsensitivePrefixes(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		text   string
	}{
		{name: "ascii", prefix: "r", text: `R"value"`},
		{name: "unicode", prefix: "é", text: `É"value"`},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			profile := ScannerProfile{
				Name: "case-insensitive-string-prefix",
				Strings: []StringRule{{
					Prefixes:              []string{testCase.prefix},
					Delimiter:             "\"",
					CaseInsensitivePrefix: true,
				}},
			}
			result := scanSourceText(t, testCase.text, profile, scannerTestLimits)
			if !result.Complete || len(result.Diagnostics) != 0 {
				t.Fatalf("case-insensitive string prefix reported partial: %+v", result.Diagnostics)
			}
			if countKind(result.Tokens, TokenString) != 1 {
				t.Fatalf("case-insensitive string count=%d want 1 tokens=%+v", countKind(result.Tokens, TokenString), result.Tokens)
			}
		})
	}
}

func TestScannerCustomInterpolationClosePreservesParenthesizedExpressions(t *testing.T) {
	profile := ScannerProfile{
		Name: "custom-interpolation-close",
		Strings: []StringRule{{
			Prefixes:           []string{""},
			Delimiter:          "\"",
			Interpolated:       true,
			InterpolationOpen:  "$(",
			InterpolationClose: ")",
		}},
	}
	result := scanSourceText(t, "call(\"$(\"inner\")\")\n", profile, scannerTestLimits)
	if !result.Complete || len(result.Diagnostics) != 0 {
		t.Fatalf("custom interpolation closer reported partial: %+v", result.Diagnostics)
	}
	if countKind(result.Tokens, TokenString) != 1 {
		t.Fatalf("custom interpolation string count=%d want 1 tokens=%+v", countKind(result.Tokens, TokenString), result.Tokens)
	}
}

func TestScannerInterpolationBracePolicies(t *testing.T) {
	csharp := ScannerProfile{
		Name: "csharp-interpolation-braces",
		Strings: []StringRule{{
			Prefixes:            []string{"$"},
			Delimiter:           "\"",
			BackslashEscapes:    true,
			InterpolationMarker: "$",
			DoubledBraceEscape:  true,
		}},
	}
	result := scanSourceText(t, "$\"{{literal}} {call()}\"", csharp, scannerTestLimits)
	if !result.Complete || len(result.Diagnostics) != 0 {
		t.Fatalf("C# doubled-brace interpolation escape reported partial: %+v", result.Diagnostics)
	}

	luau := ScannerProfile{
		Name: "luau-interpolation-braces",
		Strings: []StringRule{{
			Prefixes:            []string{""},
			Delimiter:           "`",
			BackslashEscapes:    true,
			Interpolated:        true,
			RejectDoubledBraces: true,
		}},
	}
	result = scanSourceText(t, "`outer {`nested {\"value\"}`} \\{ \\``", luau, scannerTestLimits)
	if !result.Complete || len(result.Diagnostics) != 0 {
		t.Fatalf("valid Luau nested interpolation reported partial: %+v", result.Diagnostics)
	}
	result = scanSourceText(t, "`invalid {{value}}`", luau, scannerTestLimits)
	if result.Complete || len(result.Diagnostics) == 0 || result.Diagnostics[0].Code != "invalid-interpolation-brace" {
		t.Fatalf("invalid Luau doubled braces were not diagnosed: %+v", result)
	}
}

func TestScannerSExpressionProfileUsesSharedBalancedForms(t *testing.T) {
	profile := ScannerProfile{
		Name:         "r27-lisp",
		Keywords:     []string{"defun"},
		LineComments: []string{";"},
		Strings:      []StringRule{{Prefixes: []string{""}, Delimiter: "\"", BackslashEscapes: true}},
		Delimiters:   []DelimiterRule{{Open: "(", Close: ")"}},
	}
	text := "(defun real () (list \"(fake)\")) ; (defun hidden ())\n"
	result := scanSourceText(t, text, profile, scannerTestLimits)
	if !result.Complete || len(result.Diagnostics) != 0 {
		t.Fatalf("S-expression scan incomplete: %+v", result.Diagnostics)
	}
	if got := keywordTexts(result.Tokens, "defun"); !reflect.DeepEqual(got, []string{"defun"}) {
		t.Fatalf("S-expression keyword visibility = %v", got)
	}
	pairs := PairDelimiterTokens(result.Tokens, profile.Delimiters)
	if len(pairs) != 6 {
		t.Fatalf("S-expression pair entries = %d, want 6", len(pairs))
	}
}

func TestShellCommentsRequireWordStart(t *testing.T) {
	text := "value=$((10#1))\n" +
		"echo foo#bar function Inline { :; }\n" +
		"echo ok # function Fake { :; }\n" +
		"# function AlsoFake { :; }\n" +
		"function Real { :; }\n"
	result := scanSourceText(t, text, ShellScannerProfile("bash"), scannerTestLimits)
	if !result.Complete || len(result.Diagnostics) != 0 {
		t.Fatalf("valid Bash hash usage reported partial: %+v", result.Diagnostics)
	}
	if got := keywordTexts(result.Tokens, "function"); !reflect.DeepEqual(got, []string{"function", "function"}) {
		t.Fatalf("Bash comment boundary hid or leaked function keywords: %v", got)
	}
}

func TestScannerHeredocsHideDeclarationLookingTextAndSupportMultipleBodies(t *testing.T) {
	profile := ScannerProfile{
		Name:         "r27-shell",
		Keywords:     []string{"function"},
		LineComments: []string{"#"},
		Strings: []StringRule{
			{Prefixes: []string{""}, Delimiter: "'", Multiline: false},
			{Prefixes: []string{""}, Delimiter: "\"", Multiline: false, BackslashEscapes: true},
		},
		HereDocs: []HereDocRule{
			{Operator: "<<", AllowQuotedDelimiter: true},
			{Operator: "<<-", AllowQuotedDelimiter: true, StripLeadingTabs: true},
		},
	}
	text := "cat <<FIRST <<-'SECOND'\n" +
		"function FakeFirst\n" +
		"FIRST\n" +
		"\tfunction FakeSecond\n" +
		"\tSECOND\n" +
		"function Real\n"
	result := scanSourceText(t, text, profile, scannerTestLimits)
	if !result.Complete || len(result.Diagnostics) != 0 {
		t.Fatalf("heredoc scan incomplete: %+v", result.Diagnostics)
	}
	if got := keywordTexts(result.Tokens, "function"); !reflect.DeepEqual(got, []string{"function"}) {
		t.Fatalf("heredoc leaked declaration-looking text: %v", got)
	}
	if countKind(result.Tokens, TokenHereDoc) != 2 {
		t.Fatalf("heredoc tokens = %d, want 2", countKind(result.Tokens, TokenHereDoc))
	}
	lines := BuildLogicalLines(result.Tokens, LogicalLineProfile{})
	foundReal := false
	for _, line := range lines {
		if strings.HasPrefix(logicalLineText(line), "function Real") {
			foundReal = true
			break
		}
	}
	if !foundReal {
		t.Fatalf("heredoc terminator newline was not preserved as a logical-line boundary: %+v", lines)
	}
	assertTokenOffsetsValid(t, text, result.Tokens)
}

func TestScannerReportsAllPendingHeredocsWhenFirstIsUnterminated(t *testing.T) {
	profile := ScannerProfile{Name: "r27-heredoc-errors", HereDocs: []HereDocRule{{Operator: "<<"}}}
	result := scanSourceText(t, "cat <<FIRST <<SECOND\nbody\n", profile, scannerTestLimits)
	if result.Complete {
		t.Fatal("unterminated heredocs unexpectedly reported complete coverage")
	}
	count := 0
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "unterminated-heredoc" {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("unterminated heredoc diagnostics = %d, want 2: %+v", count, result.Diagnostics)
	}
}

func TestLogicalLineBuilderHandlesBasicSeparatorsAndIndentation(t *testing.T) {
	vbText := "Class Demo\n    Dim total = 1 _\n        + 2 : Dim other = 3\nEnd Class\n"
	vbScan := scanSourceText(t, vbText, VBNetScannerProfile(), scannerTestLimits)
	vbLines := BuildLogicalLines(vbScan.Tokens, LogicalLineProfile{Separators: []string{":"}, SkipDirectives: true})
	if len(vbLines) != 4 {
		t.Fatalf("VB logical lines = %d, want 4: %+v", len(vbLines), vbLines)
	}
	if got := logicalLineText(vbLines[1]); !strings.Contains(got, "total") || !strings.Contains(got, "+ 2") {
		t.Fatalf("continued VB line = %q", got)
	}

	pyText := "class Demo:\n    def work():\n        return 1\nvalue = 2\n"
	pyScan := scanSourceText(t, pyText, PythonScannerProfile(), scannerTestLimits)
	pyLines := BuildLogicalLines(pyScan.Tokens, LogicalLineProfile{TrackIndentation: true, SkipDirectives: true})
	if got := []int{pyLines[0].Indent, pyLines[1].Indent, pyLines[2].Indent, pyLines[3].Indent}; !reflect.DeepEqual(got, []int{0, 1, 2, 0}) {
		t.Fatalf("Python logical indentation = %v", got)
	}
}

func TestBuildLogicalLinesUsesBoundedIsolatedStorage(t *testing.T) {
	const lineCount = 2048
	tokens := make([]Token, 0, lineCount*4+1)
	for line := 0; line < lineCount; line++ {
		offset := line * 16
		tokens = append(tokens,
			Token{Kind: TokenIdentifier, Text: "value", StartOffset: offset, EndOffset: offset + 5},
			Token{Kind: TokenOperator, Text: "=", StartOffset: offset + 6, EndOffset: offset + 7},
			Token{Kind: TokenNumber, Text: "1", StartOffset: offset + 8, EndOffset: offset + 9},
			Token{Kind: TokenNewline, Text: "\n", StartOffset: offset + 9, EndOffset: offset + 10},
		)
	}
	tokens = append(tokens, Token{Kind: TokenEOF, StartOffset: lineCount * 16, EndOffset: lineCount * 16})

	var lines []LogicalLine
	allocations := testing.AllocsPerRun(10, func() {
		lines = BuildLogicalLines(tokens, LogicalLineProfile{})
	})
	if allocations > 8 {
		t.Fatalf("BuildLogicalLines allocations = %.0f, want <= 8 for %d lines", allocations, lineCount)
	}
	if len(lines) != lineCount {
		t.Fatalf("logical line count = %d, want %d", len(lines), lineCount)
	}

	inputFirst := tokens[0]
	lines[0].Tokens[0].Text = "changed"
	if tokens[0] != inputFirst {
		t.Fatalf("logical-line tokens alias input tokens: input=%+v", tokens[0])
	}

	secondFirst := lines[1].Tokens[0]
	first := append(lines[0].Tokens, Token{Kind: TokenIdentifier, Text: "extra"})
	if len(first) != 4 || lines[1].Tokens[0] != secondFirst {
		t.Fatalf("appending to one logical line corrupted adjacent storage: first=%+v second=%+v", first, lines[1].Tokens)
	}

	afterEOF := BuildLogicalLines([]Token{
		{Kind: TokenEOF},
		{Kind: TokenIdentifier, Text: "ignored", StartOffset: 1, EndOffset: 8},
	}, LogicalLineProfile{})
	if afterEOF != nil {
		t.Fatalf("tokens after EOF produced logical lines: %+v", afterEOF)
	}
}

func TestKeywordScopePairingIsTopOnlyAndDeterministic(t *testing.T) {
	events := []KeywordScopeEvent{
		{Line: 0, Label: "begin", Open: true},
		{Line: 1, Label: "case", Open: true},
		{Line: 2, Label: "begin"},
		{Line: 3, Label: "CASE"},
		{Line: 4, Label: "BEGIN"},
	}
	pairing := PairKeywordScopes(events, true)
	if !reflect.DeepEqual(pairing.Pairs, map[int]int{0: 4, 1: 3}) {
		t.Fatalf("keyword pairs = %#v", pairing.Pairs)
	}
	if !reflect.DeepEqual(pairing.Unmatched, []int{2}) {
		t.Fatalf("unmatched keyword events = %v, want [2]", pairing.Unmatched)
	}
}

func TestFixedAndFreeLineModelsPreserveOffsetsContinuationAndLabels(t *testing.T) {
	text := "C fixed comment\n12345 X = 1\n     & + 2\n"
	document := sourceDocumentForScanner(text)
	lines, err := BuildSourceLines(context.Background(), document, LineModelProfile{
		Kind: LineModelFixed,
		Fixed: FixedLineProfile{
			CommentColumnOne:   []string{"C", "c", "*", "!"},
			LabelStartColumn:   1,
			LabelEndColumn:     6,
			ContinuationColumn: 6,
			CodeStartColumn:    7,
			CodeEndColumn:      73,
		},
	}, LineModelLimits{MaxLines: 16, MaxLineBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 3 || !lines[0].Comment || lines[1].Continuation || !lines[2].Continuation {
		t.Fatalf("fixed lines = %+v", lines)
	}
	if got := text[lines[1].Code.Start:lines[1].Code.End]; got != "X = 1" {
		t.Fatalf("fixed code = %q", got)
	}
	fixedLabel, ok := RecognizeLineLabel(document, lines[1], LineLabelProfile{Style: LineLabelFixedField})
	if !ok || fixedLabel.Name != "12345" || text[fixedLabel.Range.Start:fixedLabel.Range.End] != "12345" {
		t.Fatalf("fixed label = %+v, %v", fixedLabel, ok)
	}

	freeText := "start: mov ax, bx\nnext line\n"
	freeDoc := sourceDocumentForScanner(freeText)
	freeLines, err := BuildSourceLines(context.Background(), freeDoc, LineModelProfile{Kind: LineModelFree}, LineModelLimits{MaxLines: 8, MaxLineBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	label, ok := RecognizeLineLabel(freeDoc, freeLines[0], LineLabelProfile{Style: LineLabelColon, Identifier: DefaultIdentifierPolicy()})
	if !ok || label.Name != "start" {
		t.Fatalf("colon label = %+v, %v", label, ok)
	}
}

func TestCompositeSegmentationAndMaskingPreserveUTF8Offsets(t *testing.T) {
	text := "héllo <% class Real {} %> tail {{value}}\r\nfin\n"
	document := sourceDocumentForScanner(text)
	segments, complete, err := SegmentCompositeSource(context.Background(), document, CompositeProfile{
		HostKind: "host", HostLanguage: "html",
		Rules: []CompositeDelimiterRule{
			{Open: "<%", Close: "%>", Kind: "server", Language: "csharp"},
			{Open: "{{", Close: "}}", Kind: "expression", Language: "template"},
		},
	}, 32)
	if err != nil {
		t.Fatal(err)
	}
	if !complete || len(segments) != 5 {
		t.Fatalf("segments complete=%v count=%d: %+v", complete, len(segments), segments)
	}
	server := segments[1]
	if got := text[server.Content.Start:server.Content.End]; strings.TrimSpace(got) != "class Real {}" {
		t.Fatalf("server content = %q", got)
	}
	masked, err := MaskOutsideRanges(text, []OffsetRange{server.Content})
	if err != nil {
		t.Fatal(err)
	}
	if len(masked) != len(text) || !utf8.ValidString(masked) {
		t.Fatalf("masked source length/UTF-8 changed: len=%d/%d valid=%v", len(masked), len(text), utf8.ValidString(masked))
	}
	if masked[server.Content.Start:server.Content.End] != text[server.Content.Start:server.Content.End] {
		t.Fatal("kept composite content changed during masking")
	}
	for index := range text {
		if (text[index] == '\r' || text[index] == '\n') && masked[index] != text[index] {
			t.Fatalf("line ending byte %d changed during masking", index)
		}
	}
}

func TestScannerPrimitiveLimitsCancellationAndMalformedProfiles(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := BuildSourceLines(ctx, sourceDocumentForScanner("x\n"), LineModelProfile{Kind: LineModelFree}, LineModelLimits{MaxLines: 2, MaxLineBytes: 16})
	if operation.KindOf(err) != operation.KindCancelled {
		t.Fatalf("line-model cancellation = %v kind=%v", err, operation.KindOf(err))
	}

	_, err = BuildSourceLines(context.Background(), sourceDocumentForScanner(strings.Repeat("x", 32)), LineModelProfile{Kind: LineModelFree}, LineModelLimits{MaxLines: 2, MaxLineBytes: 8})
	if operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("line-model limit = %v kind=%v", err, operation.KindOf(err))
	}

	_, _, err = SegmentCompositeSource(context.Background(), sourceDocumentForScanner("<%x%><%y%>"), CompositeProfile{
		HostKind: "host", Rules: []CompositeDelimiterRule{{Open: "<%", Close: "%>", Kind: "server"}},
	}, 1)
	if operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("composite segment limit = %v kind=%v", err, operation.KindOf(err))
	}

	invalid := ScannerProfile{Name: "invalid", Delimiters: []DelimiterRule{{Open: "(", Close: ""}}}
	_, err = ScanSource(context.Background(), sourceDocumentForScanner("x"), invalid, scannerTestLimits)
	if operation.KindOf(err) != operation.KindInvalidInput {
		t.Fatalf("invalid delimiter profile = %v kind=%v", err, operation.KindOf(err))
	}
}

func FuzzScannerPrimitiveProfiles(f *testing.F) {
	for _, seed := range []string{
		"begin <% $value(foo) %> end\n",
		"cat <<EOF\nfunction fake\nEOF\nfunction real\n",
		"(defun x () (list \"value\"))\n",
		"héllo <% code %> tail {{value}}\r\n",
	} {
		f.Add(seed, uint8(0))
		f.Add(seed, uint8(1))
	}
	f.Fuzz(func(t *testing.T, text string, selector uint8) {
		profiles := []ScannerProfile{
			{
				Name: "fuzz-delimiters", Keywords: []string{"begin", "end", "function"},
				Identifier: IdentifierPolicy{UnicodeLetters: true, UnicodeDigits: true, UnicodeMarks: true, Underscore: true, ExtraStart: "$", ExtraContinue: "$"},
				Delimiters: []DelimiterRule{{Open: "(", Close: ")"}, {Open: "<%", Close: "%>"}}, DirectiveRules: []DirectiveRule{{Prefix: "%"}},
			},
			{
				Name: "fuzz-heredoc", Keywords: []string{"function"}, LineComments: []string{"#"},
				Strings:  []StringRule{{Prefixes: []string{""}, Delimiter: "'"}, {Prefixes: []string{""}, Delimiter: "\"", BackslashEscapes: true}},
				HereDocs: []HereDocRule{{Operator: "<<", AllowQuotedDelimiter: true}, {Operator: "<<-", AllowQuotedDelimiter: true, StripLeadingTabs: true}},
			},
		}
		profile := profiles[int(selector)%len(profiles)]
		document := sourceDocumentForScanner(text)
		result, err := ScanSource(context.Background(), document, profile, ScannerLimits{MaxTokens: 4096, MaxTokenBytes: 4096, MaxNesting: 64})
		if err == nil {
			assertTokenOffsetsValid(t, text, result.Tokens)
		} else if kind := operation.KindOf(err); kind != operation.KindInvalidInput && kind != operation.KindLimit {
			t.Fatalf("unexpected scanner fuzz error: %v kind=%v", err, kind)
		}

		_, lineErr := BuildSourceLines(context.Background(), document, LineModelProfile{Kind: LineModelFree}, LineModelLimits{MaxLines: 512, MaxLineBytes: 4096})
		if lineErr != nil {
			if kind := operation.KindOf(lineErr); kind != operation.KindInvalidInput && kind != operation.KindLimit {
				t.Fatalf("unexpected line-model fuzz error: %v kind=%v", lineErr, kind)
			}
		}

		segments, _, segmentErr := SegmentCompositeSource(context.Background(), document, CompositeProfile{
			HostKind: "host", Rules: []CompositeDelimiterRule{{Open: "<%", Close: "%>", Kind: "server"}, {Open: "{{", Close: "}}", Kind: "expression"}},
		}, 128)
		if segmentErr == nil {
			for _, segment := range segments {
				if segment.Full.Start < 0 || segment.Full.End < segment.Full.Start || segment.Full.End > len(text) || segment.Content.Start < segment.Full.Start || segment.Content.End > segment.Full.End {
					t.Fatalf("invalid composite fuzz segment: %+v len=%d", segment, len(text))
				}
			}
		} else if kind := operation.KindOf(segmentErr); kind != operation.KindInvalidInput && kind != operation.KindLimit {
			t.Fatalf("unexpected composite fuzz error: %v kind=%v", segmentErr, kind)
		}
	})
}

func logicalLineText(line LogicalLine) string {
	var parts []string
	for _, token := range line.Tokens {
		parts = append(parts, token.Text)
	}
	return strings.Join(parts, " ")
}
