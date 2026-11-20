package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// Comment boundaries take precedence over greedy operator tokenization in every
// shared scanner profile: an adjacent comment is not part of an operator.
func TestScannerOperatorStopsBeforeAdjacentComment(t *testing.T) {
	for _, tc := range []struct {
		name, source   string
		profile        ScannerProfile
		wantOperator   string
		wantIdentifier string
	}{
		{"c-block", "a||/* it doesn't leak */b", CScannerProfile(), "||", "b"},
		{"c-line", "a||// it's a comment\nb", CScannerProfile(), "||", "b"},
		{"cpp-block", "a&&/* doesn't leak */b", CPPScannerProfile(), "&&", "b"},
		{"java-block", "a+/* doesn't leak */b", JavaScannerProfile(), "+", "b"},
		{"javascript-block", "a&&/* doesn't leak */b", JavaScriptScannerProfile(), "&&", "b"},
		{"lua-line", "a+-- it's a comment\nb", LuaScannerProfile("lua"), "+", "b"},
	} {
		for _, eol := range []struct{ name, value string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
			t.Run(tc.name+"/"+eol.name, func(t *testing.T) {
				doc := sourceDocumentForScanner(strings.ReplaceAll(tc.source, "\n", eol.value))
				scan, err := ScanSource(context.Background(), doc, tc.profile, ScannerLimits{MaxTokens: 128, MaxTokenBytes: 1024, MaxNesting: 64})
				if err != nil {
					t.Fatal(err)
				}
				if !scan.Complete || len(scan.Diagnostics) != 0 {
					t.Fatalf("valid adjacent comment cannot produce scanner warnings: %+v", scan.Diagnostics)
				}
				var words []string
				for _, tok := range scan.Tokens {
					if tok.Kind == TokenIdentifier || tok.Kind == TokenOperator {
						words = append(words, tok.Text)
					}
				}
				if len(words) != 3 || words[0] != "a" || words[1] != tc.wantOperator || words[2] != tc.wantIdentifier {
					t.Fatalf("comment swallowed as operator or emitted tokens: %v", words)
				}
			})
		}
	}
}
