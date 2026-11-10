package sourceintelligence

import (
	"context"
	"testing"
)

// WPF Name and x:Name identify named controls; namespace-qualified lookalikes,
// quoted attribute text, comments and CDATA must not create fake fields.
func TestXAMLPlainNameControlsAreNavigableWithoutFabrication(t *testing.T) {
	source := `<Window x:Class="Demo.Window" xmlns:x="http://schemas.microsoft.com/winfx/2006/xaml">
  <Grid Name="Root">
    <ListBox Name='listBox1' />
    <TextBlock x:Name="Version" Text="Not an attribute: Name='Phantom'" />
    <TextBlock d:Name="DesignOnly" xmlns:d="urn:design" />
    <TextBlock name="lowercase" />
    <!-- <Button Name="Commented" /> -->
    <![CDATA[<Button Name="CDATAFake" />]]>
  </Grid>
</Window>`
	result, err := (XAMLAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(source), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("XAML selected source unexpectedly partial: %+v", result.Analysis)
	}
	names := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, want := range []string{"Demo.Window", "Demo.Window.Root", "Demo.Window.listBox1", "Demo.Window.Version"} {
		if _, ok := names[want]; !ok {
			t.Errorf("actual XAML declaration missing: %q; symbols=%+v", want, result.Analysis.Symbols)
		}
	}
	for _, bogus := range []string{"Demo.Window.Phantom", "Demo.Window.DesignOnly", "Demo.Window.lowercase", "Demo.Window.Commented", "Demo.Window.CDATAFake"} {
		if _, ok := names[bogus]; ok {
			t.Errorf("fabricated XAML name: %q", bogus)
		}
	}
	for _, sym := range result.Analysis.Symbols {
		if sym.Name == "listBox1" {
			if sym.NativeKind != "Name" || sym.NameRange.Start.Line != 3 {
				t.Fatalf("WPF Name declaration has wrong kind or offset: %+v", sym)
			}
		}
		if sym.Name == "Version" && sym.NativeKind != "x:Name" {
			t.Fatalf("x:Name backward compatibility broken: %+v", sym)
		}
	}
}

func TestXAMLQuotedClassTextCannotInventCodeBehind(t *testing.T) {
	source := `<Grid Text="example x:Class='Fake.View'" Name="Visible" />`
	result, err := (XAMLAnalyzer{}).Analyze(context.Background(),
		sourceDocumentForScanner(source), testAnalyzeOptions(true, 32))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Analysis.Symbols) != 0 {
		t.Fatalf("quoted ordinary text invented a code-behind identity: %+v", result.Analysis.Symbols)
	}
}

func TestXAMLUnclosedElementReportsIncompleteCoverage(t *testing.T) {
	source := `<Window x:Class="Demo.View"><Grid Name="Actual" <Button Name="Fake"`
	result, err := (XAMLAnalyzer{}).Analyze(context.Background(),
		sourceDocumentForScanner(source), testAnalyzeOptions(true, 32))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.CoverageComplete {
		t.Fatalf("unfinished tag incorrectly reported complete: %+v", result.Analysis)
	}
}

func TestXAMLNameQuotedValueDoesNotCreateNestedAttribute(t *testing.T) {
	source := `<Window x:Class="Demo.Window"><TextBlock Text="hello Name='NotAControl'" Name="Actual" /></Window>`
	result, err := (XAMLAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(source), testAnalyzeOptions(true, 32))
	if err != nil {
		t.Fatal(err)
	}
	names := symbolsByQualifiedName(result.Analysis.Symbols)
	if _, ok := names["Demo.Window.Actual"]; !ok {
		t.Fatalf("real Name after quoted text missing: %+v", result.Analysis.Symbols)
	}
	if _, ok := names["Demo.Window.NotAControl"]; ok {
		t.Fatalf("quoted text invented a field: %+v", result.Analysis.Symbols)
	}
}
