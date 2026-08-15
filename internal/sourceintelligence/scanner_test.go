package sourceintelligence

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/zoster81/scripthold/internal/operation"
)

var phase4ScannerLimits = ScannerLimits{
	MaxTokens:     4096,
	MaxTokenBytes: 16 * 1024,
	MaxNesting:    128,
}

func TestScannerCSharpOpaqueCommentsAndStringFamilies(t *testing.T) {
	text := `#if DEBUG
// class FakeComment { void Nope() {} }
/* class FakeBlock { } */
class Real<T> {
    string a = "class FakeNormal { }";
    string b = @"class ""FakeVerbatim"" { }";
    string c = $"class FakeInterpolated {value}";
    string d = $@"class FakeBoth {value} ""quoted""";
    string e = """class FakeRaw { }""";
    char ch = '}';
    void Work() { Call(); }
}
#endif
`
	result := scanSourceText(t, text, CSharpScannerProfile(), phase4ScannerLimits)
	if !result.Complete || len(result.Diagnostics) != 0 {
		t.Fatalf("C# scan incomplete: %+v", result.Diagnostics)
	}
	if got := keywordTexts(result.Tokens, "class"); !reflect.DeepEqual(got, []string{"class"}) {
		t.Fatalf("C# class keywords = %v, want only real declaration", got)
	}
	if got := keywordTexts(result.Tokens, "void"); !reflect.DeepEqual(got, []string{"void"}) {
		t.Fatalf("C# void keywords = %v, want only Work", got)
	}
	if countKind(result.Tokens, TokenDirective) != 2 {
		t.Fatalf("C# directive tokens = %d, want 2", countKind(result.Tokens, TokenDirective))
	}
	if result.MaxDepth < 2 {
		t.Fatalf("C# max delimiter depth = %d, want nested type/member delimiters", result.MaxDepth)
	}
	assertTokenOffsetsValid(t, text, result.Tokens)
}

func TestScannerVBNetCaseInsensitiveStringsCommentsAndContinuation(t *testing.T) {
	text := "cLaSs Réal\r\n" +
		"    ' Class FakeComment\r\n" +
		"    Dim text = \"Class \"\"FakeString\"\"\"\r\n" +
		"    Dim total = 1 _\r\n" +
		"        + 2\r\n" +
		"    sUb Work() : Dim x = 1\r\n" +
		"    eNd sUb\r\n" +
		"eNd cLaSs\r\n"
	result := scanSourceText(t, text, VBNetScannerProfile(), phase4ScannerLimits)
	if !result.Complete || len(result.Diagnostics) != 0 {
		t.Fatalf("VB.NET scan incomplete: %+v", result.Diagnostics)
	}
	if got := keywordTexts(result.Tokens, "class"); len(got) != 2 {
		t.Fatalf("VB.NET class keywords = %v, want opening+End Class only", got)
	}
	if got := keywordTexts(result.Tokens, "sub"); len(got) != 2 {
		t.Fatalf("VB.NET sub keywords = %v, want opening+End Sub only", got)
	}
	if !hasIdentifier(result.Tokens, "Réal") {
		t.Fatalf("VB.NET Unicode identifier missing: %+v", result.Tokens)
	}
	physicalBreaks := strings.Count(text, "\n")
	logicalBreaks := countKind(result.Tokens, TokenNewline)
	if logicalBreaks != physicalBreaks-1 {
		t.Fatalf("VB.NET logical newlines = %d, physical=%d; continuation was not suppressed exactly once", logicalBreaks, physicalBreaks)
	}
	assertTokenOffsetsValid(t, text, result.Tokens)
}

func TestScannerVBNetMultilineStringLiteral(t *testing.T) {
	text := "Module Demo\n" +
		"    Sub Work()\n" +
		"        Dim value = \"first\n" +
		"second\n" +
		"third\"\n" +
		"    End Sub\n" +
		"End Module\n"
	result := scanSourceText(t, text, VBNetScannerProfile(), phase4ScannerLimits)
	if !result.Complete || len(result.Diagnostics) != 0 {
		t.Fatalf("VB.NET multiline string scan incomplete: %+v", result.Diagnostics)
	}
	if countKind(result.Tokens, TokenString) != 1 {
		t.Fatalf("VB.NET multiline string tokens = %d, want 1", countKind(result.Tokens, TokenString))
	}
	assertTokenOffsetsValid(t, text, result.Tokens)
}

func TestScannerPythonIndentationTripleRawFStringsAndLogicalLines(t *testing.T) {
	text := `class Réal:
    fake = "class FakeString:"
    triple = '''def FakeTriple():
        pass
'''
    raw = r"def FakeRaw():\\n"
    formatted = f"def FakeFormatted(): {value}"
    values = (
        1,
        2,
    )
    total = 1 + \
        2
    async def work():
        # def FakeComment():
        return values

def outside():
    return 1
`
	result := scanSourceText(t, text, PythonScannerProfile(), phase4ScannerLimits)
	if !result.Complete || len(result.Diagnostics) != 0 {
		t.Fatalf("Python scan incomplete: %+v", result.Diagnostics)
	}
	if got := keywordTexts(result.Tokens, "class"); !reflect.DeepEqual(got, []string{"class"}) {
		t.Fatalf("Python class keywords = %v", got)
	}
	if got := keywordTexts(result.Tokens, "def"); len(got) != 2 {
		t.Fatalf("Python def keywords = %v, want work+outside", got)
	}
	if countKind(result.Tokens, TokenIndent) < 2 || countKind(result.Tokens, TokenDedent) < 2 {
		t.Fatalf("Python indent/dedent tokens missing: indents=%d dedents=%d", countKind(result.Tokens, TokenIndent), countKind(result.Tokens, TokenDedent))
	}
	physicalBreaks := strings.Count(text, "\n")
	logicalBreaks := countKind(result.Tokens, TokenNewline)
	if logicalBreaks >= physicalBreaks {
		t.Fatalf("Python implicit/explicit continuations did not suppress logical newlines: logical=%d physical=%d", logicalBreaks, physicalBreaks)
	}
	if !hasIdentifier(result.Tokens, "Réal") {
		t.Fatalf("Python Unicode identifier missing")
	}
	assertTokenOffsetsValid(t, text, result.Tokens)
}

func TestScannerNestedBlockCommentsWhenProfileEnablesThem(t *testing.T) {
	profile := ScannerProfile{
		Name:          "nested-comment-test",
		Keywords:      []string{"class"},
		BlockComments: []BlockCommentRule{{Start: "/*", End: "*/", Nestable: true}},
	}
	text := "/* outer /* class Fake */ still comment */ class Real"
	result := scanSourceText(t, text, profile, phase4ScannerLimits)
	if !result.Complete || len(result.Diagnostics) != 0 {
		t.Fatalf("nested-comment scan incomplete: %+v", result.Diagnostics)
	}
	if got := keywordTexts(result.Tokens, "class"); !reflect.DeepEqual(got, []string{"class"}) {
		t.Fatalf("nested comment leaked declaration keyword: %v", got)
	}
}

func TestScannerPrefersLongerBlockCommentOverSharedLineCommentPrefix(t *testing.T) {
	profile := ScannerProfile{
		Name:          "nim-comment-prefix-test",
		Keywords:      []string{"proc"},
		LineComments:  []string{"#"},
		BlockComments: []BlockCommentRule{{Start: "#[", End: "]#", Nestable: true}},
	}
	text := "#[\nproc Fake()\n#[ nested proc AlsoFake() ]#\n]#\nproc Real()\n"
	result := scanSourceText(t, text, profile, phase4ScannerLimits)
	if !result.Complete || len(result.Diagnostics) != 0 {
		t.Fatalf("shared-prefix block comment scan incomplete: %+v", result.Diagnostics)
	}
	if got := keywordTexts(result.Tokens, "proc"); !reflect.DeepEqual(got, []string{"proc"}) {
		t.Fatalf("shared-prefix block comment leaked declaration keyword: %v", got)
	}

	invalid := scanSourceText(t, "#[\nproc Fake()\n", profile, phase4ScannerLimits)
	if invalid.Complete || !hasScannerDiagnostic(invalid.Diagnostics, "unterminated-comment") {
		t.Fatalf("unterminated shared-prefix block comment was accepted: %+v", invalid)
	}
}

func TestScannerCSharpInterpolatedExpressionsMaySpanLines(t *testing.T) {
	text := `class Real {
    string a = $"{string.Join(
        "/",
        values)}";
    string b = $"{Format("class Fake { }")}";
    void Work() { }
}`
	result := scanSourceText(t, text, CSharpScannerProfile(), phase4ScannerLimits)
	if !result.Complete || len(result.Diagnostics) != 0 {
		t.Fatalf("C# multiline interpolation scan incomplete: %+v", result.Diagnostics)
	}
	if got := keywordTexts(result.Tokens, "class"); !reflect.DeepEqual(got, []string{"class"}) {
		t.Fatalf("C# class keywords = %v, want only real declaration", got)
	}
	if got := keywordTexts(result.Tokens, "void"); !reflect.DeepEqual(got, []string{"void"}) {
		t.Fatalf("C# void keywords = %v, want only Work", got)
	}

	invalid := scanSourceText(t, "class Real { string bad = $\"literal\ntext\"; }", CSharpScannerProfile(), phase4ScannerLimits)
	if invalid.Complete || !hasScannerDiagnostic(invalid.Diagnostics, "unterminated-string") {
		t.Fatalf("literal newline in normal interpolated string was accepted: %+v", invalid)
	}
}

func TestScannerMalformedInputReturnsBoundedDiagnostics(t *testing.T) {
	tests := []struct {
		name    string
		profile ScannerProfile
		text    string
		code    string
	}{
		{name: "unterminated C# block comment", profile: CSharpScannerProfile(), text: "class Real { /* class Fake", code: "unterminated-comment"},
		{name: "unterminated C# string", profile: CSharpScannerProfile(), text: `class Real { string x = "class Fake`, code: "unterminated-string"},
		{name: "unterminated Python triple", profile: PythonScannerProfile(), text: `class Real:\n    x = '''def Fake`, code: "unterminated-string"},
		{name: "mismatched delimiter", profile: CSharpScannerProfile(), text: "class Real { void Work(] {}", code: "mismatched-delimiter"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			result := scanSourceText(t, testCase.text, testCase.profile, phase4ScannerLimits)
			if result.Complete {
				t.Fatalf("malformed scan reported complete: %+v", result)
			}
			if !hasScannerDiagnostic(result.Diagnostics, testCase.code) {
				t.Fatalf("diagnostics = %+v, want %s", result.Diagnostics, testCase.code)
			}
			if len(result.Diagnostics) > ScannerMaxDiagnostics {
				t.Fatalf("diagnostics exceeded cap: %d", len(result.Diagnostics))
			}
		})
	}
}

func TestScannerLimitsNestingTokensAndTokenBytes(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		limits ScannerLimits
	}{
		{name: "nesting", text: strings.Repeat("(", 20), limits: ScannerLimits{MaxTokens: 100, MaxTokenBytes: 1024, MaxNesting: 8}},
		{name: "tokens", text: strings.Repeat("x ", 100), limits: ScannerLimits{MaxTokens: 16, MaxTokenBytes: 1024, MaxNesting: 8}},
		{name: "token bytes", text: strings.Repeat("x", 200), limits: ScannerLimits{MaxTokens: 16, MaxTokenBytes: 32, MaxNesting: 8}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := ScanSource(context.Background(), sourceDocumentForScanner(testCase.text), CSharpScannerProfile(), testCase.limits)
			if operation.KindOf(err) != operation.KindLimit {
				t.Fatalf("limit error = %v, kind=%v", err, operation.KindOf(err))
			}
		})
	}
}

func TestScannerDeterminismCancellationAndUnicodeBoundary(t *testing.T) {
	text := "class Δelta { void Café变量() { return; } }"
	document := sourceDocumentForScanner(text)
	first, err := ScanSource(context.Background(), document, CSharpScannerProfile(), phase4ScannerLimits)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ScanSource(context.Background(), document, CSharpScannerProfile(), phase4ScannerLimits)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("scanner is nondeterministic:\nfirst=%+v\nsecond=%+v", first, second)
	}
	for _, identifier := range []string{"Δelta", "Café变量"} {
		if !hasIdentifier(first.Tokens, identifier) {
			t.Fatalf("missing Unicode identifier %q", identifier)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = ScanSource(ctx, document, CSharpScannerProfile(), phase4ScannerLimits)
	if operation.KindOf(err) != operation.KindCancelled {
		t.Fatalf("cancel error = %v, kind=%v", err, operation.KindOf(err))
	}
}

func TestScannerDirectiveOnlyAtLogicalLineStart(t *testing.T) {
	text := "#if DEBUG\nclass Real { string x = \"#if FAKE\"; }\nvalue # notDirective\n#endif\n"
	result := scanSourceText(t, text, CSharpScannerProfile(), phase4ScannerLimits)
	if countKind(result.Tokens, TokenDirective) != 2 {
		t.Fatalf("directive count = %d, want 2", countKind(result.Tokens, TokenDirective))
	}
}

func FuzzScannerNoPanic(f *testing.F) {
	for _, seed := range []string{
		"class X {}",
		`class X { string s = $"{value}"; }`,
		"Class X\r\nEnd Class\r\n",
		"def x():\n    return 1\n",
		"/* /* nested */",
		"'''unterminated",
		"Δ café 变量",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, text string) {
		if len(text) > 32*1024 || !utf8.ValidString(text) {
			t.Skip()
		}
		limits := ScannerLimits{MaxTokens: 2048, MaxTokenBytes: 4096, MaxNesting: 64}
		for _, profile := range []ScannerProfile{CSharpScannerProfile(), VBNetScannerProfile(), PythonScannerProfile()} {
			_, err := ScanSource(context.Background(), sourceDocumentForScanner(text), profile, limits)
			if err != nil && operation.KindOf(err) != operation.KindLimit {
				t.Fatalf("unexpected scanner error for %s: %v", profile.Name, err)
			}
		}
	})
}

func scanSourceText(t *testing.T, text string, profile ScannerProfile, limits ScannerLimits) ScanResult {
	t.Helper()
	result, err := ScanSource(context.Background(), sourceDocumentForScanner(text), profile, limits)
	if err != nil {
		t.Fatalf("ScanSource(%s): %v", profile.Name, err)
	}
	return result
}

func sourceDocumentForScanner(text string) *SourceDocument {
	return &SourceDocument{Path: "scanner.fixture", Text: text, Encoding: "utf-8", lineStarts: buildLineStarts(text)}
}

func keywordTexts(tokens []Token, keyword string) []string {
	var result []string
	for _, token := range tokens {
		if token.Kind == TokenKeyword && strings.EqualFold(token.Text, keyword) {
			result = append(result, token.Text)
		}
	}
	return result
}

func countKind(tokens []Token, kind TokenKind) int {
	count := 0
	for _, token := range tokens {
		if token.Kind == kind {
			count++
		}
	}
	return count
}

func hasIdentifier(tokens []Token, text string) bool {
	for _, token := range tokens {
		if token.Kind == TokenIdentifier && token.Text == text {
			return true
		}
	}
	return false
}

func hasScannerDiagnostic(diagnostics []ScannerDiagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func assertTokenOffsetsValid(t *testing.T, text string, tokens []Token) {
	t.Helper()
	previous := 0
	for index, token := range tokens {
		if token.StartOffset < previous || token.StartOffset < 0 || token.EndOffset < token.StartOffset || token.EndOffset > len(text) {
			t.Fatalf("token[%d] invalid offsets: %+v", index, token)
		}
		if token.Kind != TokenIndent && token.Kind != TokenDedent && token.Kind != TokenEOF && text[token.StartOffset:token.EndOffset] != token.Text {
			t.Fatalf("token[%d] text/range mismatch: token=%+v source=%q", index, token, text[token.StartOffset:token.EndOffset])
		}
		previous = token.StartOffset
	}
}
