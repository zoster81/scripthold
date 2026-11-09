package sourceintelligence

import (
	"context"
	"testing"
)

func TestVBNetQualifiedNamespaceDeclarations(t *testing.T) {
	source := `Namespace Global.Company.Catalog
    Public Class Reader
        Public Function Read() As Integer
            Return 1
        End Function
    End Class
End Namespace
Namespace Microsoft.SmallVisualBasic.LanguageService
    Public Class CompletionProvider
        Public Sub CommitItem()
        End Sub
    End Class
End Namespace
Namespace Global
    Namespace Extra.Tools
        Public Class Helper
        End Class
    End Namespace
End Namespace
`
	doc := sourceDocumentForScanner(source)
	doc.Path = "qualified-namespaces.vb"
	result, err := (VBNetAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("valid qualified namespace source reported incomplete: %+v", result.Analysis)
	}
	found := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, want := range []string{
		"Company.Catalog", "Company.Catalog.Reader", "Company.Catalog.Reader.Read",
		"Microsoft.SmallVisualBasic.LanguageService",
		"Microsoft.SmallVisualBasic.LanguageService.CompletionProvider",
		"Microsoft.SmallVisualBasic.LanguageService.CompletionProvider.CommitItem",
		"Extra.Tools", "Extra.Tools.Helper",
	} {
		if _, ok := found[want]; !ok {
			t.Errorf("missing correctly owned declaration %q (actual=%v)", want, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	for _, forbidden := range []string{"Global", "Global.Reader", "Global.Company", "Microsoft.CompletionProvider", "Microsoft.SmallVisualBasic", "Extra"} {
		if _, ok := found[forbidden]; ok {
			t.Errorf("invented namespace/owner %q", forbidden)
		}
	}
}
