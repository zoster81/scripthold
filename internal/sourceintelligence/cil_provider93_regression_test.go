package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestCILProvider93MethodsAndNegativeNames(t *testing.T) {
	source := `.assembly Demo {}
.class public Demo
{
 .method public hidebysig instance void GenericMethod<T>(!!T value) cil managed
 {
   ret
 }
 .method public static void Neighbor() cil managed
 {
   ret
 }
}
.method assembly static void modopt([mscorlib]System.Runtime.CompilerServices.CallConvCdecl)
        test_pointer_operations(int32 param) cil managed
{
 ret
}
.method static void Main()
{
 ret
}
.method private hidebysig static void Main(string[] args) cil managed
{
 ret
}
`
	result, err := (CILAnalyzer{}).Analyze(context.Background(), testSourceDocument("provider93.il", source), testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
		t.Fatalf("source analysis incomplete: %+v", result.Analysis)
	}
	want := map[string][]int{
		"GenericMethod":           {4},
		"Neighbor":                {8},
		"test_pointer_operations": {14},
		"Main":                    {18, 22},
	}
	for _, sym := range result.Analysis.Symbols {
		if sym.Kind != SymbolKindMethod {
			continue
		}
		if sym.Name == "T" || sym.Name == "modopt" || sym.Name == "CallConvCdecl" || sym.Name == "void" {
			t.Errorf("parameter or return-type token fabricated as method: %+v", sym)
		}
		expected, ok := want[sym.Name]
		if !ok {
			continue
		}
		line := sym.NameRange.Start.Line
		for i, number := range expected {
			if line == number {
				want[sym.Name] = append(expected[:i], expected[i+1:]...)
				break
			}
		}
	}
	for name, lines := range want {
		if len(lines) > 0 {
			t.Errorf("missing method %s at source lines %v", name, lines)
		}
	}
}

func TestCILProvider93IndependentOriginalSources(t *testing.T) {
	cases := []struct {
		name, env, required string
		methods             map[string][]int
		forbidden           []string
	}{
		{
			name:     "Linguist generic method and neighboring members",
			env:      "SCRIPTHOLD_REAL_CIL_LINGUIST",
			required: ".class public auto ansi beforefieldinit GenericWithModifiers",
			methods: map[string][]int{
				"MethodWithGenericParameter": {32},
				"GenericMethod":              {39},
				"get_GenericField":           {48},
			},
			forbidden: []string{"T", "modopt", "IsConst"},
		},
		{
			name:     "KDE overloaded global Main and Cdecl pointer method",
			env:      "SCRIPTHOLD_REAL_CIL_KDE",
			required: "test_pointer_operations(int32 param) cil managed",
			methods: map[string][]int{
				"Main":                    {10, 19},
				"test_pointer_operations": {56},
			},
			forbidden: []string{"modopt", "CallConvCdecl", "void"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("original CIL project not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("original source must be an existing regular file under 1 MiB: info=%v err=%v", info, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), tc.required) {
				t.Fatal("original CIL source oracle mismatch")
			}
			result, err := (CILAnalyzer{}).Analyze(context.Background(), testSourceDocument(path, string(raw)), testAnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
				t.Fatalf("original source analysis incomplete: %+v", result.Analysis)
			}
			found := make(map[string][]int)
			for _, sym := range result.Analysis.Symbols {
				if sym.Kind != SymbolKindMethod {
					continue
				}
				found[sym.Name] = append(found[sym.Name], sym.NameRange.Start.Line)
				for _, bad := range tc.forbidden {
					if sym.Name == bad {
						t.Errorf("type/signature token fabricated as method: %+v", sym)
					}
				}
			}
			for name, lines := range tc.methods {
				for _, line := range lines {
					present := false
					for _, actual := range found[name] {
						if actual == line {
							present = true
						}
					}
					if !present {
						t.Errorf("missing original CIL method %s line %d; found=%v", name, line, found)
					}
				}
			}
		})
	}
}
