package sourceintelligence

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/zoster81/scripthold/internal/operation"
)

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
