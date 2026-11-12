package sourceintelligence

import (
	"context"
	"testing"
)

// XAML resource keys are source-backed resource identities, not XML attribute
// text or runtime bindings. Preserve their native kind and source position.
func TestXAMLResourceKeysHaveSafeStructuralDeclarations(t *testing.T) {
	source := `<Application x:Class="Demo.App" xmlns:x="http://schemas.microsoft.com/winfx/2009/xaml">
  <Application.Resources>
    <Color x:Key="PageBackgroundColor">#512bdf</Color>
    <Color x:Key='PrimaryTextColor'>White</Color>
    <Button x:Name="NamedButton" Content="x:Key='Phantom'" />
    <Color d:Key="DesignOnly" xmlns:d="urn:design">Red</Color>
    <Color x:key="WrongCase">Blue</Color>
    <!-- <Color x:Key="Commented">Yellow</Color> -->
    <![CDATA[<Color x:Key="CDATAFake">Green</Color>]]>
  </Application.Resources>
</Application>`
	result, err := (XAMLAnalyzer{}).Analyze(context.Background(),
		sourceDocumentForScanner(source), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("complete original-style XAML was reported partial: %+v", result.Analysis)
	}
	names := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, tc := range []struct {
		name, native string
		line         int
	}{
		{"Demo.App", "x:Class", 1},
		{"Demo.App.PageBackgroundColor", "x:Key", 3},
		{"Demo.App.PrimaryTextColor", "x:Key", 4},
		{"Demo.App.NamedButton", "x:Name", 5},
	} {
		s, ok := names[tc.name]
		if !ok {
			t.Errorf("real XAML declaration %q missing", tc.name)
			continue
		}
		if s.NativeKind != tc.native || s.NameRange.Start.Line != tc.line {
			t.Errorf("wrong kind or location for %q: %+v", tc.name, s)
		}
	}
	for _, fake := range []string{"Demo.App.Phantom", "Demo.App.DesignOnly", "Demo.App.WrongCase", "Demo.App.Commented", "Demo.App.CDATAFake"} {
		if _, ok := names[fake]; ok {
			t.Errorf("fabricated XAML resource %q", fake)
		}
	}
}
