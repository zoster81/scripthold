package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

func TestGraphQLEnumMemberDeclarations(t *testing.T) {
	const original = `# enum Phantom { GHOST }
enum Site {
  DESKTOP
  MOBILE @deprecated
  "Quoted description mentioning INVENTED"
  INVALID: String
  # COMMENT_ONLY
}
type Device {
  MOBILE: String
}
enum Role {
  ADMIN
  OWNER @onEnumValue
}
enum BackupRole {
  ADMIN
}
enum InvalidKeywords {
  true
  false
  null
}
`
	for _, tc := range []struct {
		name string
		text string
	}{
		{"LF", original},
		{"CRLF", strings.ReplaceAll(original, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := (GraphQLAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.text),
				testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			byName := symbolsByQualifiedName(res.Analysis.Symbols)
			for _, want := range []struct {
				qualified, parent string
				line              int
			}{
				{"Site.DESKTOP", "Site", 3},
				{"Site.MOBILE", "Site", 4},
				{"Role.ADMIN", "Role", 13},
				{"Role.OWNER", "Role", 14},
				{"BackupRole.ADMIN", "BackupRole", 17},
			} {
				s, ok := byName[want.qualified]
				if !ok || s.Kind != SymbolKindConstant || s.NativeKind != "enum-value" ||
					s.ParentQualifiedName != want.parent || s.NameRange.Start.Line != want.line ||
					s.DeclarationRange.Start.Line != want.line {
					t.Errorf("enum value %s at line %d not indexed correctly: %+v", want.qualified, want.line, s)
				}
			}
			for _, forbidden := range []string{
				"Site.INVENTED", "Site.INVALID", "Site.COMMENT_ONLY", "Phantom.GHOST",
				"Device.MOBILE.ADMIN", "MOBILE", "ADMIN",
				"InvalidKeywords.true", "InvalidKeywords.false", "InvalidKeywords.null",
			} {
				if got, ok := byName[forbidden]; ok {
					t.Errorf("non-declaration invented: %s %+v", forbidden, got)
				}
			}
			if d, ok := byName["Device.MOBILE"]; !ok || d.Kind != SymbolKindField {
				t.Errorf("similarly named field must remain a field: %+v", d)
			}
			if !res.Analysis.CoverageComplete || res.Analysis.Truncated {
				t.Errorf("valid enum selections should be complete: %+v", res.Analysis)
			}
		})
	}
}
