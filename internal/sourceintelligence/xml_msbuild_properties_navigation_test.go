package sourceintelligence

import (
	"context"
	"testing"
)

func TestXMLMSBuildPropertyNavigation(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		want, absent []string
	}{
		{
			name: "classic msbuild namespace and scoped properties",
			source: "<?xml version=\"1.0\"?><Project xmlns=\"http://schemas.microsoft.com/developer/msbuild/2003\">\n" +
				"  <PropertyGroup Condition=\"'$(Configuration)'=='Release'\">\n" +
				"    <TargetFrameworkVersion>v4.0</TargetFrameworkVersion>\n" +
				"    <OptionStrict Condition=\"'$(Mode)' == 'Safe'\">On</OptionStrict>\n" +
				"  </PropertyGroup>\n" +
				"  <ItemGroup><Reference Include=\"System.Xml\" /></ItemGroup>\n" +
				"  <!-- <PropertyGroup><Ghost>fake</Ghost></PropertyGroup> -->\n" +
				"</Project>",
			want:   []string{"TargetFrameworkVersion", "OptionStrict"},
			absent: []string{"Ghost", "Reference", "PropertyGroup", "ItemGroup"},
		},
		{
			name: "sdk project and simple leaf properties",
			source: "<Project Sdk=\"Microsoft.NET.Sdk\">\n" +
				"  <PropertyGroup><LangVersion>14</LangVersion><RestorePackagesWithLockFile>true</RestorePackagesWithLockFile></PropertyGroup>\n" +
				"  <ItemGroup><PackageReference Include=\"Example\" Version=\"1\"/></ItemGroup>\n" +
				"</Project>",
			want:   []string{"LangVersion", "RestorePackagesWithLockFile"},
			absent: []string{"PackageReference", "Example"},
		},
		{
			name:   "no generic xml property claims",
			source: "<config><PropertyGroup><LangVersion>14</LangVersion></PropertyGroup><setting id=\"explicit\"/></config>",
			want:   []string{"explicit"},
			absent: []string{"LangVersion"},
		},
		{
			name:   "nested values and malformed project are not fabricated",
			source: "<Project><PropertyGroup><Nested><LangVersion>14</LangVersion></Nested></PropertyGroup>",
			absent: []string{"Nested", "LangVersion"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (XMLAnalyzer{}).Analyze(context.Background(), testSourceDocument("project.xml", tc.source), testAnalyzeOptions(false, 64))
			if err != nil {
				t.Fatal(err)
			}
			found := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, name := range tc.want {
				s, ok := found[name]
				if !ok {
					t.Errorf("missing %s; found=%v", name, sortedSymbolQualifiedNames(result.Analysis.Symbols))
					continue
				}
				if name != "explicit" && (s.Kind != SymbolKindProperty || s.NativeKind != "msbuild-property") {
					t.Errorf("%s kind=%s native=%s", name, s.Kind, s.NativeKind)
				}
			}
			for _, name := range tc.absent {
				if _, ok := found[name]; ok {
					t.Errorf("fabricated %s", name)
				}
			}
		})
	}
}
