package sourceintelligence

import (
	"context"
	"testing"
)

func TestWebFormsCodeBehindDependenciesOnlyFromAuthenticDirectives(t *testing.T) {
	source := `<%@ Page Language="C#" CodeBehind="Dashboard.aspx.cs" Inherits="Demo.Dashboard" %>
<%-- <%@ Page CodeBehind="ServerCommentFake.cs" %> --%>
<!-- <%@ Control CodeFile='HTMLCommentFake.cs' %> -->
<%@ Control Description="CodeFile='DescriptionFake.cs'" CodeFile='Widget.ascx.cs' %>
<div data-note="CodeBehind='NotADirective.cs'"></div>
<div data-note="<%@ Page CodeBehind='QuotedTagFake.cs' %>"></div>
<script>const sample = "<%@ Page CodeFile='ScriptFake.cs' %>";</script>
`
	result, err := (ASPNetWebFormsAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(source), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	// CodeBehind/CodeFile links are externally navigable source evidence:
	// emit actual directives and never names mentioned in comments/values.
	if len(result.Dependencies) != 2 {
		t.Fatalf("expected only two genuine code-behind links, got %+v", result.Dependencies)
	}
	for i, want := range []struct {
		name string
		line int
	}{
		{"Dashboard.aspx.cs", 1},
		{"Widget.ascx.cs", 4},
	} {
		got := result.Dependencies[i]
		if got.Kind != StructuralDependencyInclude || got.Value != want.name || got.Range.Start.Line != want.line {
			t.Fatalf("code-behind link[%d] = %+v, want %q at line %d", i, got, want.name, want.line)
		}
	}
}

func TestWebFormsMasterDirectiveRetainsLiteralCodeFile(t *testing.T) {
	source := `<%@ Master Language='VB' CodeFile='Layouts/Site.master.vb' Inherits='Demo.Site' %>`
	result, err := (ASPNetWebFormsAnalyzer{}).Analyze(context.Background(),
		sourceDocumentForScanner(source), testAnalyzeOptions(true, 32))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Dependencies) != 1 || result.Dependencies[0].Value != "Layouts/Site.master.vb" ||
		result.Dependencies[0].Kind != StructuralDependencyInclude {
		t.Fatalf("genuine Master CodeFile dependency missing: %+v", result.Dependencies)
	}
}

func TestWebFormsCodeBehindRejectsMalformedDirectives(t *testing.T) {
	for _, source := range []string{
		`<%@ Page CodeBehind="NeverClosed.cs"`,
		`<%@ Page CodeBehind=Unquoted.cs %>`,
		`<%@ Page CodeBehind="Quote'Broken.cs' %>`,
		`<%@ PageData CodeBehind="NotAPage.cs" %>`,
	} {
		result, err := (ASPNetWebFormsAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(source), testAnalyzeOptions(true, 64))
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Dependencies) != 0 {
			t.Fatalf("non-directive/malformed code-behind link fabricated from %q: %+v", source, result.Dependencies)
		}
	}
}
