package sourceintelligence

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

// A C++ declaration and a qualified out-of-class definition may share a name
// without proving the same overload. Navigation can expose an explicit candidate,
// but must not assert resolved class ownership or an exact signature match.
func TestProjectQueryCPPExplicitHeaderDefinitionCandidates(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, header, source string
		wantCandidate        bool
	}{
		{
			name:          "single-authored-method",
			header:        "class Codec { public: int Read(int n); };\n",
			source:        "#include \"codec.hpp\"\nint Codec::Read(int n) { return n; }\n",
			wantCandidate: true,
		},
		{
			name:   "unmatched-first-overload",
			header: "class Codec { public: int Read(int n); int Read(double n); };\n",
			source: "#include \"codec.hpp\"\nint Codec::Read(double n) { return 1; }\n",
		},
		{
			name:   "missing-include",
			header: "class Codec { public: int Read(int n); };\n",
			source: "int Codec::Read(int n) { return n; }\n",
		},
		{
			name:   "only-declaration-in-source",
			header: "class Codec { public: int Read(int n); };\n",
			source: "#include \"codec.hpp\"\nint Codec::Read(int n);\n",
		},
		{
			name:   "mismatched-parameter-type-not-a-candidate",
			header: "class Codec { public: int Read(int n); };\n",
			source: "#include \"codec.hpp\"\nint Codec::Read(double n) { return 0; }\n",
		},
		{
			name:   "renamed-parameter-conservative",
			header: "class Codec { public: int Read(int n); };\n",
			source: "#include \"codec.hpp\"\nint Codec::Read(int count) { return count; }\n",
		},
		{
			name:   "different-cv-qualifier",
			header: "class Codec { public: int Read(int n) const; };\n",
			source: "#include \"codec.hpp\"\nint Codec::Read(int n) { return n; }\n",
		},
		{
			name:          "same-cv-qualifier",
			header:        "class Codec { public: int Read(int n) const; };\n",
			source:        "#include \"codec.hpp\"\nint Codec::Read(int n) const { return n; }\n",
			wantCandidate: true,
		},
		{
			name:          "virtual-override-not-in-out-of-class-definition",
			header:        "class Base { public: virtual int Read(int n) const; }; class Codec : public Base { public: int Read(int n) const override; };\n",
			source:        "#include \"codec.hpp\"\nint Codec::Read(int n) const { return n; }\n",
			wantCandidate: true,
		},
		{
			name:   "different-noexcept-qualifier",
			header: "class Codec { public: int Read(int n) noexcept; };\n",
			source: "#include \"codec.hpp\"\nint Codec::Read(int n) { return n; }\n",
		},
		{
			name:          "matching-overload-among-declarations",
			header:        "class Codec { public: int Read(int n); int Read(double n); };\n",
			source:        "#include \"codec.hpp\"\nint Codec::Read(int n) { return n; }\n",
			wantCandidate: true,
		},
		{
			name:   "namespace-not-a-class",
			header: "namespace Codec { int Read(int n); }\n",
			source: "#include \"codec.hpp\"\nint Codec::Read(int n) { return n; }\n",
		},
		{
			name:          "two-definitions-different-overloads",
			header:        "class Codec { public: int Read(int n); int Read(double n); };\n",
			source:        "#include \"codec.hpp\"\nint Codec::Read(int n) { return n; }\nint Codec::Read(double n) { return 0; }\n",
			wantCandidate: true,
		},
		{
			name:   "duplicate-identical-definition-stays-unlinked",
			header: "class Codec { public: int Read(int n); };\n",
			source: "#include \"codec.hpp\"\nint Codec::Read(int n) { return n; }\nint Codec::Read(int n) { return n + 1; }\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join("project", "cpp", tc.name)
			header := projectResolverFacts(t, CPPAnalyzer{}, filepath.Join(root, "codec.hpp"), tc.header)
			implementation := projectResolverFacts(t, CPPAnalyzer{}, filepath.Join(root, "codec.cc"), tc.source)
			id := projectSymbolID(t, header, "Codec.Read")
			model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{implementation, header}, projectResolverLimitsForTest())
			if err != nil {
				t.Fatal(err)
			}
			result, err := model.QueryRelations(context.Background(), RelationDefinitions, querySymbolSelector(header, id), ProjectSelector{}, nil, queryLimits())
			if err != nil {
				t.Fatal(err)
			}
			candidateCount := 0
			for _, record := range result.Records {
				if record.Target.Path != implementation.Path {
					continue
				}
				candidateCount++
				if record.Resolution != ResolutionAmbiguous || record.Evidence != SymbolEvidenceStructural ||
					record.Target.SymbolID == "" || record.Target.QualifiedName != "Codec.Read" ||
					record.Source.Path != header.Path || record.Source.SymbolID != id {
					t.Errorf("unsafe candidate relation: %+v", record)
				}
			}
			if (candidateCount == 1) != tc.wantCandidate || candidateCount > 1 {
				t.Fatalf("candidate count=%d, want candidate=%v; records=%+v", candidateCount, tc.wantCandidate, result.Records)
			}
			filtered, err := model.QueryRelations(context.Background(), RelationDefinitions, querySymbolSelector(header, id),
				ProjectSelector{}, []SymbolEvidence{SymbolEvidenceScopeResolved}, queryLimits())
			if err != nil {
				t.Fatal(err)
			}
			for _, record := range filtered.Records {
				if record.Target.Path == implementation.Path {
					t.Fatalf("scope-only filter leaked a project candidate: %+v", record)
				}
			}
		})
	}
}

// Original C++ move-assignment operators have overloads, including deleted
// copy-assignment members. Only a matching written declarator and a directly
// resolved include justify a single ambiguous cross-file navigation candidate.
func TestProjectQueryCPPAssignmentOperatorCandidates(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	const header = "class Codec {\npublic:\n Codec& operator=(Codec&& other) noexcept;\n Codec& operator=(const Codec& other) = delete;\n};\n"
	for _, tc := range []struct {
		name, source string
		wantMove     bool
	}{
		{"matching-move", "#include \"codec.hpp\"\nCodec& Codec::operator=(Codec&& other) noexcept { return *this; }\n", true},
		{"mismatched-const-reference", "#include \"codec.hpp\"\nCodec& Codec::operator=(const Codec& other) { return *this; }\n", false},
		{"missing-include", "Codec& Codec::operator=(Codec&& other) noexcept { return *this; }\n", false},
		{"duplicate-definitions", "#include \"codec.hpp\"\nCodec& Codec::operator=(Codec&& other) noexcept { return *this; }\nCodec& Codec::operator=(Codec&& other) noexcept { return *this; }\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join("project", "cpp", "operators", tc.name)
			h := projectResolverFacts(t, CPPAnalyzer{}, filepath.Join(root, "codec.hpp"), header)
			s := projectResolverFacts(t, CPPAnalyzer{}, filepath.Join(root, "codec.cc"), tc.source)
			ids := map[int]string{}
			for _, symbol := range h.Analysis.Analysis.Symbols {
				if symbol.NameRange.Start.Line != 3 && symbol.NameRange.Start.Line != 4 {
					continue
				}
				if symbol.Kind != SymbolKindOperator || symbol.Name != "operator=" ||
					symbol.QualifiedName != "Codec.operator=" || symbol.ParentID == "" {
					t.Fatalf("false C++ assignment member: %+v", symbol)
				}
				ids[symbol.NameRange.Start.Line] = symbol.ID
			}
			if len(ids) != 2 {
				t.Fatalf("missing move/deleted assignment overloads: %+v", h.Analysis.Analysis.Symbols)
			}
			model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{h, s}, projectResolverLimitsForTest())
			if err != nil {
				t.Fatal(err)
			}
			for _, line := range []int{3, 4} {
				relations, err := model.QueryRelations(context.Background(), RelationDefinitions, querySymbolSelector(h, ids[line]), ProjectSelector{}, nil, queryLimits())
				if err != nil {
					t.Fatal(err)
				}
				got := 0
				for _, rel := range relations.Records {
					if rel.Target.Path != s.Path {
						continue
					}
					got++
					if rel.Evidence != SymbolEvidenceStructural || rel.Resolution != ResolutionAmbiguous ||
						rel.Source.SymbolID != ids[line] || rel.Target.SymbolID == "" ||
						rel.Target.QualifiedName != "Codec.operator=" {
						t.Errorf("unsafe operator candidate: %+v", rel)
					}
				}
				want := line == 3 && tc.wantMove
				if (got == 1) != want || got > 1 {
					t.Errorf("line=%d candidates=%d want=%v: %+v", line, got, want, relations.Records)
				}
			}
		})
	}
}

// An unresolved out-of-class C++ constructor/destructor remains a file-local
// function. Project navigation may expose only a structurally ambiguous candidate.
func TestProjectQueryCPPQualifiedLifecycleCandidates(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, header, source        string
		wantConstruct, wantDestruct bool
	}{
		{"one-ctor-one-dtor",
			"class Codec { public: Codec(int n); ~Codec(); };\n",
			"#include \"codec.hpp\"\nCodec::Codec(int n) {}\nCodec::~Codec() {}\n", true, true},
		{"overloaded-constructors",
			"class Codec { public: Codec(int n); Codec(double n); ~Codec(); };\n",
			"#include \"codec.hpp\"\nCodec::Codec(int n) {}\nCodec::~Codec() {}\n", true, true},
		{"no-include",
			"class Codec { public: Codec(int n); ~Codec(); };\n",
			"Codec::Codec(int n) {}\nCodec::~Codec() {}\n", false, false},
		{"declarations-only",
			"class Codec { public: Codec(int n); ~Codec(); };\n",
			"#include \"codec.hpp\"\nCodec::Codec(int n);\nCodec::~Codec();\n", false, false},
		{"invalid-return-type",
			"class Codec { public: Codec(int n); ~Codec(); };\n",
			"#include \"codec.hpp\"\nint Codec::Codec(int n) { return n; }\nCodec::~Codec() {}\n", false, true},
		{"duplicate-destructor-definitions",
			"class Codec { public: Codec(int n); ~Codec(); };\n",
			"#include \"codec.hpp\"\nCodec::Codec(int n) {}\nCodec::~Codec() {}\nCodec::~Codec() {}\n", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join("project", "cpp", "lifecycle", tc.name)
			header := projectResolverFacts(t, CPPAnalyzer{}, filepath.Join(root, "codec.hpp"), tc.header)
			source := projectResolverFacts(t, CPPAnalyzer{}, filepath.Join(root, "codec.cc"), tc.source)
			model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{source, header}, projectResolverLimitsForTest())
			if err != nil {
				t.Fatal(err)
			}
			for _, kind := range []struct {
				value  SymbolKind
				want   bool
				native string
			}{
				{SymbolKindConstructor, tc.wantConstruct, "qualified-constructor-definition"},
				{SymbolKindDestructor, tc.wantDestruct, "qualified-destructor-definition"},
			} {
				var declaration *NormalizedSymbol
				for i := range header.Analysis.Analysis.Symbols {
					s := &header.Analysis.Analysis.Symbols[i]
					if s.Kind == kind.value {
						declaration = s
						break
					}
				}
				if declaration == nil {
					t.Fatalf("missing %s declaration: %+v", kind.value, header.Analysis.Analysis.Symbols)
				}
				relations, err := model.QueryRelations(context.Background(), RelationDefinitions, querySymbolSelector(header, declaration.ID), ProjectSelector{}, nil, queryLimits())
				if err != nil {
					t.Fatal(err)
				}
				wantQualified := "Codec.Codec"
				if kind.value == SymbolKindDestructor {
					wantQualified = "Codec.~Codec"
				}
				candidates := 0
				for _, rel := range relations.Records {
					if rel.Target.Path != source.Path {
						continue
					}
					candidates++
					if rel.Evidence != SymbolEvidenceStructural || rel.Resolution != ResolutionAmbiguous ||
						rel.Source.SymbolID != declaration.ID || rel.Target.SymbolID == "" || rel.Target.QualifiedName != wantQualified {
						t.Errorf("unsafe %s link: %+v", kind.value, rel)
					}
					for _, symbol := range source.Analysis.Analysis.Symbols {
						if symbol.ID == rel.Target.SymbolID && (symbol.Kind != SymbolKindFunction || symbol.NativeKind != kind.native || symbol.ParentID != "" || symbol.BodyRange == nil) {
							t.Errorf("not a lexical %s definition: %+v", kind.value, symbol)
						}
					}
				}
				if (candidates == 1) != kind.want || candidates > 1 {
					t.Errorf("%s candidate count=%d, want=%v: %+v", kind.value, candidates, kind.want, relations.Records)
				}
			}
		})
	}
}

func TestProjectQueryDependenciesDependentsTraceImpactAndCycles(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("project", "phase13", "graph")
	aPath := filepath.Join(root, "a.ts")
	bPath := filepath.Join(root, "b.ts")
	cPath := filepath.Join(root, "c.ts")
	a := projectResolverFacts(t, TypeScriptAnalyzer{}, aPath, "import { B } from \"./b\";\nexport class A extends B {}\n")
	b := projectResolverFacts(t, TypeScriptAnalyzer{}, bPath, "import { C } from \"./c\";\nexport class B extends C {}\n")
	c := projectResolverFacts(t, TypeScriptAnalyzer{}, cPath, "import { A } from \"./a\";\nexport class C {}\n")
	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{c, a, b}, projectResolverLimitsForTest())
	if err != nil {
		t.Fatal(err)
	}
	limits := queryLimits()

	dependencies, err := model.QueryRelations(context.Background(), RelationDependencies, queryPathSelector(a), ProjectSelector{}, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(dependencies.Records) != 1 || dependencies.Records[0].Source.Path != aPath || dependencies.Records[0].Target.Path != bPath || dependencies.Records[0].Evidence != SymbolEvidenceProjectResolved || dependencies.Records[0].Resolution != ResolutionResolved {
		t.Fatalf("dependencies = %+v", dependencies)
	}

	dependents, err := model.QueryRelations(context.Background(), RelationDependents, queryPathSelector(b), ProjectSelector{}, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(dependents.Records) != 1 || dependents.Records[0].Source.Path != aPath || dependents.Records[0].Target.Path != bPath {
		t.Fatalf("dependents = %+v", dependents)
	}

	trace, err := model.QueryRelations(context.Background(), RelationTrace, queryPathSelector(a), queryPathSelector(c), nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(trace.Records) != 2 || trace.Records[0].Source.Path != aPath || trace.Records[0].Target.Path != bPath || trace.Records[1].Source.Path != bPath || trace.Records[1].Target.Path != cPath {
		t.Fatalf("trace = %+v", trace)
	}
	for _, record := range trace.Records {
		if record.Kind != RelationTrace {
			t.Fatalf("trace kind = %q", record.Kind)
		}
	}

	impact, err := model.QueryRelations(context.Background(), RelationImpact, queryPathSelector(c), ProjectSelector{}, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(impact.Records) != 2 || impact.Records[0].Source.Path != bPath || impact.Records[0].Target.Path != cPath || impact.Records[1].Source.Path != aPath || impact.Records[1].Target.Path != bPath {
		t.Fatalf("impact = %+v", impact)
	}

	cycles, err := model.QueryRelations(context.Background(), RelationCycles, ProjectSelector{}, ProjectSelector{}, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(cycles.Records) != 3 {
		t.Fatalf("cycles = %+v", cycles)
	}
	for _, record := range cycles.Records {
		if record.Kind != RelationCycles || record.Resolution != ResolutionResolved {
			t.Fatalf("cycle record = %+v", record)
		}
	}
}

func TestProjectQueryReferencesDefinitionsInheritanceAndImplementations(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("project", "phase13", "types")
	contractPath := filepath.Join(root, "Contract.java")
	basePath := filepath.Join(root, "Base.java")
	implPath := filepath.Join(root, "Impl.java")
	contract := projectResolverFacts(t, JavaAnalyzer{}, contractPath, "package demo; public interface Contract {}\n")
	base := projectResolverFacts(t, JavaAnalyzer{}, basePath, "package demo; public class Base {}\n")
	impl := projectResolverFacts(t, JavaAnalyzer{}, implPath, "package demo; public class Impl extends Base implements Contract {}\n")
	contractID := projectSymbolID(t, contract, "demo.Contract")
	baseID := projectSymbolID(t, base, "demo.Base")
	implID := projectSymbolID(t, impl, "demo.Impl")
	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{impl, base, contract}, projectResolverLimitsForTest())
	if err != nil {
		t.Fatal(err)
	}
	limits := queryLimits()

	inheritance, err := model.QueryRelations(context.Background(), RelationInheritance, querySymbolSelector(impl, implID), ProjectSelector{}, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(inheritance.Records) != 1 || inheritance.Records[0].Source.SymbolID != implID || inheritance.Records[0].Target.SymbolID != baseID || inheritance.Records[0].Evidence != SymbolEvidenceProjectResolved {
		t.Fatalf("inheritance = %+v", inheritance)
	}

	implementations, err := model.QueryRelations(context.Background(), RelationImplementations, querySymbolSelector(contract, contractID), ProjectSelector{}, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(implementations.Records) != 1 || implementations.Records[0].Source.SymbolID != implID || implementations.Records[0].Target.SymbolID != contractID {
		t.Fatalf("implementations = %+v", implementations)
	}

	references, err := model.QueryRelations(context.Background(), RelationReferences, querySymbolSelector(base, baseID), ProjectSelector{}, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(references.Records) != 1 || references.Records[0].Source.SymbolID != implID || references.Records[0].Target.SymbolID != baseID {
		t.Fatalf("references = %+v", references)
	}

	baseReference := projectReferenceByTarget(t, model.References(), implPath, "Base")
	definitions, err := model.QueryRelations(context.Background(), RelationDefinitions, ProjectSelector{
		Kind: ProjectSelectorPosition, Path: implPath, Position: &baseReference.Range.Start, SourceFingerprint: impl.SourceFingerprint,
	}, ProjectSelector{}, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions.Records) != 1 || definitions.Records[0].Target.SymbolID != baseID || definitions.Records[0].Resolution != ResolutionResolved {
		t.Fatalf("definitions = %+v", definitions)
	}
}

func TestProjectForwardSourceIDIndexPreservesLegacyRelations(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, ext  string
		analyzer   SourceAnalyzer
		typeScript bool
	}{
		{"java", "java", JavaAnalyzer{}, false},
		{"typescript", "ts", TypeScriptAnalyzer{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join("project", "source-ids", tc.name)
			baseText := "package demo; public class Base {}\n"
			if tc.typeScript {
				baseText = "export class Base {}\n"
			}
			base := projectResolverFacts(t, tc.analyzer, filepath.Join(root, "Base."+tc.ext), baseText)
			facts := []ProjectFileFacts{base}
			var duplicateID = "duplicated-across-source-files"
			for index := 0; index < 70; index++ {
				source := fmt.Sprintf("package demo; public class Child%03d extends Base {}\n", index)
				if tc.typeScript {
					source = fmt.Sprintf("import { Base } from './Base';\nexport class Child%03d extends Base {}\n", index)
				}
				fact := projectResolverFacts(t, tc.analyzer, filepath.Join(root, fmt.Sprintf("Child%03d.%s", index, tc.ext)), source)
				if index == 1 || index == 2 {
					for position := range fact.Analysis.Analysis.Symbols {
						if fact.Analysis.Analysis.Symbols[position].Kind == SymbolKindClass {
							fact.Analysis.Analysis.Symbols[position].ID = duplicateID
						}
					}
				}
				facts = append(facts, fact)
			}
			model, err := BuildProjectModel(context.Background(), registry, facts, projectResolverLimitsForTest())
			if err != nil {
				t.Fatal(err)
			}
			if len(model.references) != 70 || len(model.referencesBySourceID) == 0 || len(model.referencesBySourceID[duplicateID]) != 2 {
				t.Fatalf("dense forward index incomplete: references=%d indexedIDs=%d duplicateRefs=%d", len(model.references), len(model.referencesBySourceID), len(model.referencesBySourceID[duplicateID]))
			}
			linear := *model
			linear.referencesBySourceID = nil
			qualified := func(name string) string {
				if tc.typeScript {
					return name
				}
				return "demo." + name
			}
			baseID := projectSymbolID(t, base, qualified("Base"))
			lateID := projectSymbolID(t, facts[len(facts)-1], qualified("Child069"))
			for _, tc2 := range []struct {
				name string
				fact ProjectFileFacts
				id   string
			}{
				{"base", base, baseID},
				{"late", facts[len(facts)-1], lateID},
				{"duplicate-first", facts[2], duplicateID},
				{"duplicate-second", facts[3], duplicateID},
			} {
				for _, maxResults := range []int{1, 200} {
					for _, evidence := range [][]SymbolEvidence{nil, {SymbolEvidenceProjectResolved}, {SymbolEvidenceStructural}} {
						limits := ProjectQueryLimits{MaxResults: maxResults, MaxNodes: 200, MaxEdges: 200, MaxDepth: 2}
						selector := ProjectSelector{Kind: ProjectSelectorSymbol, Path: tc2.fact.Path, SourceFingerprint: tc2.fact.SourceFingerprint, SymbolID: tc2.id}
						got, gotErr := model.QueryRelations(context.Background(), RelationInheritance, selector, ProjectSelector{}, evidence, limits)
						want, wantErr := linear.QueryRelations(context.Background(), RelationInheritance, selector, ProjectSelector{}, evidence, limits)
						if operation.KindOf(gotErr) != operation.KindOf(wantErr) || !reflect.DeepEqual(got, want) {
							t.Fatalf("%s/%s max=%d evidence=%v indexed=%+v err=%v linear=%+v err=%v", tc.name, tc2.name, maxResults, evidence, got, gotErr, want, wantErr)
						}
						gotJSON, err := json.Marshal(got)
						if err != nil {
							t.Fatal(err)
						}
						wantJSON, err := json.Marshal(want)
						if err != nil || string(gotJSON) != string(wantJSON) {
							t.Fatalf("serialized parity %s/%s max=%d: indexed=%s linear=%s err=%v", tc.name, tc2.name, maxResults, gotJSON, wantJSON, err)
						}
						if tc2.id == duplicateID && maxResults == 200 && len(evidence) == 0 && len(got.Records) != 2 {
							t.Fatalf("cross-file duplicate IDs must still produce both relations: %+v", got)
						}
					}
				}
			}
			stale := ProjectSelector{Kind: ProjectSelectorSymbol, Path: facts[2].Path, SourceFingerprint: "stale-fingerprint", SymbolID: duplicateID}
			limits := ProjectQueryLimits{MaxResults: 10, MaxNodes: 10, MaxEdges: 10, MaxDepth: 2}
			_, indexedErr := model.QueryRelations(context.Background(), RelationInheritance, stale, ProjectSelector{}, nil, limits)
			_, legacyErr := linear.QueryRelations(context.Background(), RelationInheritance, stale, ProjectSelector{}, nil, limits)
			if operation.KindOf(indexedErr) != operation.KindConflict || operation.KindOf(legacyErr) != operation.KindConflict {
				t.Fatalf("stale source fingerprint parity indexed=%v legacy=%v", indexedErr, legacyErr)
			}
		})
	}
}

func TestProjectPositionSelectorsPreserveCanonicalSourceAliases(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("project", "position-alias")
	base := projectResolverFacts(t, JavaAnalyzer{}, filepath.Join(root, "Base.java"), "package demo; public class Base {}\n")
	impl := projectResolverFacts(t, JavaAnalyzer{}, filepath.Join(root, "Impl.java"), "package demo; public class Impl extends Base {}\n")
	sep := string(filepath.Separator)
	alias := filepath.Dir(impl.Path) + sep + "nested" + sep + ".." + sep + filepath.Base(impl.Path)
	if alias == impl.Path || projectPathKey(alias) != projectPathKey(impl.Path) {
		t.Fatal("test fixture must use a different spelling for the same canonical path")
	}
	for index := range impl.Analysis.Analysis.Symbols {
		impl.Analysis.Analysis.Symbols[index].Path = alias
	}
	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{impl, base}, projectResolverLimitsForTest())
	if err != nil {
		t.Fatal(err)
	}
	var reference ProjectReference
	for _, candidate := range model.References() {
		if candidate.TargetSpelling == "Base" {
			reference = candidate
			break
		}
	}
	if reference.Source.Path != alias {
		t.Fatalf("expected the original symbol path spelling to survive, got %q", reference.Source.Path)
	}
	baseID := projectSymbolID(t, base, "demo.Base")
	position := reference.Range.Start
	selector := ProjectSelector{Kind: ProjectSelectorPosition, Path: impl.Path, Position: &position, SourceFingerprint: impl.SourceFingerprint}
	definitions, err := model.QueryRelations(context.Background(), RelationDefinitions, selector, ProjectSelector{}, nil, queryLimits())
	if err != nil || len(definitions.Records) != 1 || definitions.Records[0].Target.SymbolID != baseID {
		t.Fatalf("canonical source path relation resolution changed: %+v, err=%v", definitions, err)
	}
	plan, err := model.PlanContext(context.Background(), []ProjectSelector{selector}, ProjectContextOptions{
		BudgetBytes: 4096, MaxItems: 8, MaxDepth: 2, BodyPolicy: ProjectContextSignaturesOnly,
	})
	if err != nil || !containsContextSymbol(plan.Candidates, baseID) {
		t.Fatalf("canonical source path context resolution changed: %+v, err=%v", plan, err)
	}
	selector.SourceFingerprint = queryZeroDigest()
	if _, err = model.QueryRelations(context.Background(), RelationDefinitions, selector, ProjectSelector{}, nil, queryLimits()); err == nil {
		t.Fatal("stale source fingerprint was accepted by position selector")
	}
	if _, err = model.PlanContext(context.Background(), []ProjectSelector{selector}, ProjectContextOptions{
		BudgetBytes: 4096, MaxItems: 8, MaxDepth: 2, BodyPolicy: ProjectContextSignaturesOnly,
	}); err == nil {
		t.Fatal("stale source fingerprint was accepted by context selector")
	}
}

func TestProjectPositionSourceIndexRetainsCanonicalSortedOverlaps(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("project", "position-order")
	base := projectResolverFacts(t, JavaAnalyzer{}, filepath.Join(root, "Base.java"), "package demo; public class Base {}\n")
	contract := projectResolverFacts(t, JavaAnalyzer{}, filepath.Join(root, "Contract.java"), "package demo; public interface Contract {}\n")
	impl := projectResolverFacts(t, JavaAnalyzer{}, filepath.Join(root, "Impl.java"), "package demo; public class Impl extends Base implements Contract {}\n")
	if len(impl.Analysis.Relations) < 2 {
		t.Fatalf("expected independent inheritance and implementation references, got %d", len(impl.Analysis.Relations))
	}
	// Two distinct references share one position, so the original sorted
	// first-match rule must survive source bucketing.
	impl.Analysis.Relations[1].Range = impl.Analysis.Relations[0].Range
	sep := string(filepath.Separator)
	alias := filepath.Dir(impl.Path) + sep + "nested" + sep + ".." + sep + filepath.Base(impl.Path)
	for i := range impl.Analysis.Analysis.Symbols {
		impl.Analysis.Analysis.Symbols[i].Path = alias
	}
	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{impl, contract, base}, projectResolverLimitsForTest())
	if err != nil {
		t.Fatal(err)
	}
	expected := make(map[string][]int)
	for i, reference := range model.references {
		key := projectPathKey(reference.Source.Path)
		expected[key] = append(expected[key], i)
	}
	if !reflect.DeepEqual(model.referencesBySource, expected) {
		t.Fatalf("source index changed canonical membership or deterministic order: got=%v want=%v", model.referencesBySource, expected)
	}
	key := projectPathKey(impl.Path)
	if len(expected[key]) < 2 {
		t.Fatalf("test must exercise overlapping source references: %v", expected)
	}
	position := model.references[expected[key][0]].Range.Start
	selector := ProjectSelector{Kind: ProjectSelectorPosition, Path: impl.Path, SourceFingerprint: impl.SourceFingerprint, Position: &position}
	selected, err := model.resolveProjectSelector(selector)
	if err != nil || selected.reference == nil || selected.reference.ID != model.references[expected[key][0]].ID {
		t.Fatalf("position tie-break changed: selected=%+v err=%v", selected, err)
	}
}

func TestStructuralSearchUsesNormalizedSymbolsAndRelations(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("project", "phase13", "search")
	base := projectResolverFacts(t, RustAnalyzer{}, filepath.Join(root, "store.rs"), "pub trait Store<T> { fn get(&self) -> T; }\n")
	itemPath := filepath.Join(root, "item.rs")
	item := projectResolverFacts(t, RustAnalyzer{}, itemPath, "use crate::store::Store;\npub struct Item<T> { value: T }\nimpl<T> Store<T> for Item<T> { fn get(&self) -> T { todo!() } }\n")
	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{item, base}, projectResolverLimitsForTest())
	if err != nil {
		t.Fatal(err)
	}

	symbols, err := model.StructuralSearch(context.Background(), ProjectSearchOptions{Query: "Sto", Match: ProjectSearchPrefix, MaxResults: 16})
	if err != nil {
		t.Fatal(err)
	}
	foundSymbol := false
	for _, match := range symbols.Matches {
		if match.SymbolID == "" {
			if match.Name != "" || match.Kind != "" || match.QualifiedName != "" {
				t.Fatalf("non-symbol search match claims symbol identity: %+v", match)
			}
			continue
		}
		if match.Name == "Store" && match.Kind == SymbolKindTrait && match.QualifiedName != "" {
			foundSymbol = true
		}
	}
	if !foundSymbol {
		t.Fatalf("symbol search = %+v", symbols)
	}

	relations, err := model.StructuralSearch(context.Background(), ProjectSearchOptions{Query: "Store<T>", Match: ProjectSearchExact, Evidence: []SymbolEvidence{SymbolEvidenceProjectResolved}, MaxResults: 16})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, match := range relations.Matches {
		if match.Path == itemPath && match.SymbolID == "" && match.Evidence == SymbolEvidenceProjectResolved {
			if match.Name != "" || match.Kind != "" || match.QualifiedName != "" {
				t.Fatalf("resolved relation search claims declaration identity: %+v", match)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("relation search = %+v", relations)
	}
}

func TestProjectSearchTopKMatchesGlobalSortWithBoundedStorage(t *testing.T) {
	matches := make([]ProjectSearchMatch, 0, 4_096)
	for i := 0; i < 4_096; i++ {
		index := (i * 7919) % 4_096
		matches = append(matches, ProjectSearchMatch{
			Path: fmt.Sprintf("source-%03d.ts", index%113),
			Range: Range{
				Start: Position{Line: index/113 + 1, Column: 2},
				End:   Position{Line: index/113 + 1, Column: 6},
			},
			SymbolID: fmt.Sprintf("s-%04d", index),
			Evidence: SymbolEvidenceStructural,
		})
	}
	for _, maximum := range []int{1, 2, 7, 64, 4_096, 4_097} {
		t.Run(fmt.Sprint(maximum), func(t *testing.T) {
			collector := newProjectSearchTopK(maximum)
			for _, match := range matches {
				collector.append(match)
				if len(collector.matches) > maximum {
					t.Fatalf("collector retained %d matches with limit %d", len(collector.matches), maximum)
				}
			}
			got := collector.result()
			want := finalizeProjectSearch(append([]ProjectSearchMatch(nil), matches...), maximum)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("top-K diverged from global ordering at limit %d: got=%+v want=%+v", maximum, got, want)
			}
		})
	}
}

func TestProjectSearchContinuesWithoutSkippingEqualKeys(t *testing.T) {
	items := []ProjectSearchMatch{
		{Path: "z.ts", Name: "Z"},
		{Path: "a.ts", Name: "A"},
		{Path: "m.ts", Name: "M"},
		{Path: "m.ts", Name: "M"},
		{Path: "m.ts", Name: "M"},
		{Path: "m.ts", Name: "M"},
		{Path: "m.ts", Name: "Z"},
		{Path: "m.ts", Name: "A"},
		{Path: "zz.ts", Name: "Last"},
	}
	want := finalizeProjectSearch(append([]ProjectSearchMatch(nil), items...), len(items))
	for _, pageSize := range []int{1, 2, 3, 5} {
		var got []ProjectSearchMatch
		var after *ProjectSearchMatch
		afterOccurrences := 0
		for page := 0; page < 16; page++ {
			collector := newProjectSearchTopK(pageSize)
			collector.after = after
			collector.afterOccurrences = afterOccurrences
			for _, match := range items {
				collector.append(match)
				if len(collector.matches) > pageSize {
					t.Fatalf("page %d retained too many results", page)
				}
			}
			result := collector.result()
			got = append(got, result.Matches...)
			if !result.Truncated {
				break
			}
			if len(result.Matches) == 0 || result.LastOccurrences <= 0 {
				t.Fatalf("cursor did not advance: %+v", result)
			}
			last := result.Matches[len(result.Matches)-1]
			after = &last
			afterOccurrences = result.LastOccurrences
		}
		if !reflect.DeepEqual(got, want.Matches) {
			t.Fatalf("page size %d changed global order: got=%+v want=%+v", pageSize, got, want.Matches)
		}
	}
}

func TestProjectSearchTopKDefersUnmatchedHeapAllocation(t *testing.T) {
	for _, maximum := range []int{1, 16, 256, 4096} {
		t.Run(fmt.Sprintf("limit-%d", maximum), func(t *testing.T) {
			collector := newProjectSearchTopK(maximum)
			if collector.matches == nil {
				t.Fatal("unmatched search must preserve a non-nil empty slice")
			}
			if capacity := cap(collector.matches); capacity != 0 {
				t.Fatalf("unmatched search reserved %d records before any match", capacity)
			}
			got, err := json.Marshal(collector.result())
			if err != nil {
				t.Fatal(err)
			}
			const want = `{"Matches":[],"Truncated":false,"LastOccurrences":0}`
			if string(got) != want {
				t.Fatalf("empty search JSON = %s, want %s", got, want)
			}
		})
	}
}

func TestProjectSearchTopKUsesSmallStorageUntilSecondMatch(t *testing.T) {
	for _, maximum := range []int{1, 7, 16, 64, 256, 4096} {
		t.Run(fmt.Sprintf("limit-%d", maximum), func(t *testing.T) {
			collector := newProjectSearchTopK(maximum)
			collector.append(ProjectSearchMatch{Name: "First"})
			wantFirst := minInt(maximum, 256)
			if maximum >= 16 {
				wantFirst = 1
			}
			if got := cap(collector.matches); got != wantFirst {
				t.Fatalf("first match capacity=%d, want=%d", got, wantFirst)
			}
			if maximum > 1 {
				collector.append(ProjectSearchMatch{Name: "Second"})
				if got, want := cap(collector.matches), minInt(maximum, 256); got != want {
					t.Fatalf("second match capacity=%d, want=%d", got, want)
				}
			}
		})
	}
}

func TestProjectSearchTopKLazyAllocationMatchesOriginalOrderingAndJSON(t *testing.T) {
	records := make([]ProjectSearchMatch, 320)
	for index := range records {
		// Identical groups exercise stable repeated anchors and last occurrences.
		records[index] = ProjectSearchMatch{
			Path:          fmt.Sprintf("project/item-%03d.ts", index/4),
			Name:          fmt.Sprintf("Item%03d", index/4),
			QualifiedName: fmt.Sprintf("demo.Item%03d", index/4),
		}
	}
	anchor := records[16]
	for _, tc := range []struct {
		name             string
		maximum          int
		records          []ProjectSearchMatch
		after            *ProjectSearchMatch
		afterOccurrences int
	}{
		{name: "empty-limit-one", maximum: 1},
		{name: "empty-large-limit", maximum: 4096},
		{name: "one-late-match", maximum: 512, records: records[319:]},
		{name: "all-small-limit", maximum: 1, records: records},
		{name: "all-truncated", maximum: 32, records: records},
		{name: "all-untruncated", maximum: 512, records: records},
		{name: "repeated-anchor", maximum: 32, records: records, after: &anchor, afterOccurrences: 2},
		{name: "continuation-no-matches", maximum: 512, records: records[:12], after: &anchor, afterOccurrences: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			legacy := &projectSearchTopK{
				matches:          make(projectSearchWorstHeap, 0, minInt(tc.maximum, 256)),
				maximum:          tc.maximum,
				after:            tc.after,
				afterOccurrences: tc.afterOccurrences,
			}
			candidate := newProjectSearchTopK(tc.maximum)
			candidate.after, candidate.afterOccurrences = tc.after, tc.afterOccurrences
			for _, record := range tc.records {
				legacy.append(record)
				candidate.append(record)
			}
			want, got := legacy.result(), candidate.result()
			if !reflect.DeepEqual(got, want) || candidate.seenAfter != legacy.seenAfter {
				t.Fatalf("top-K differs from original: got=%+v want=%+v, seenAfter=%d/%d",
					got, want, candidate.seenAfter, legacy.seenAfter)
			}
			legacyJSON, err := json.Marshal(want)
			if err != nil {
				t.Fatal(err)
			}
			candidateJSON, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			if string(candidateJSON) != string(legacyJSON) {
				t.Fatalf("top-K JSON differs: got=%s want=%s", candidateJSON, legacyJSON)
			}
		})
	}
}

func TestProjectStructuralSearchRejectsInvalidContinuation(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	facts := projectResolverFacts(t, TypeScriptAnalyzer{}, filepath.Join("project", "pages", "only.ts"), "export class Item {}\n")
	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{facts}, projectResolverLimitsForTest())
	if err != nil {
		t.Fatal(err)
	}
	missing := ProjectSearchMatch{Path: "missing.ts", Name: "Item"}
	for _, testCase := range []struct {
		name  string
		after *ProjectSearchMatch
		count int
		want  operation.Kind
	}{
		{name: "count without anchor", count: 1, want: operation.KindInvalidInput},
		{name: "anchor without count", after: &missing, want: operation.KindInvalidInput},
		{name: "missing anchor", after: &missing, count: 1, want: operation.KindConflict},
		{name: "impossible repeated match", after: &ProjectSearchMatch{Path: facts.Path, Name: "Item"}, count: 99, want: operation.KindConflict},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := model.StructuralSearch(context.Background(), ProjectSearchOptions{
				Query: "Item", Match: ProjectSearchExact, MaxResults: 2,
				After: testCase.after, AfterOccurrences: testCase.count,
			})
			if operation.KindOf(err) != testCase.want {
				t.Fatalf("continuation error=%v kind=%v want=%v", err, operation.KindOf(err), testCase.want)
			}
		})
	}
}

func TestProjectStructuralSearchPageMatchesUnpagedResults(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("project", "query", "pages")
	one := projectResolverFacts(t, TypeScriptAnalyzer{}, filepath.Join(root, "one.ts"), "export class Base {}\nexport class Other extends Base {}\n")
	two := projectResolverFacts(t, TypeScriptAnalyzer{}, filepath.Join(root, "two.ts"), "import { Base } from \"./one\";\nexport class Child extends Base {}\n")
	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{two, one}, projectResolverLimitsForTest())
	if err != nil {
		t.Fatal(err)
	}
	want, err := model.StructuralSearch(context.Background(), ProjectSearchOptions{Query: "Base", Match: ProjectSearchExact, MaxResults: 32})
	if err != nil || len(want.Matches) < 3 {
		t.Fatalf("unpaged search: %+v err=%v", want, err)
	}
	var combined []ProjectSearchMatch
	var after *ProjectSearchMatch
	afterOccurrences := 0
	for page := 0; page < 20; page++ {
		result, err := model.StructuralSearch(context.Background(), ProjectSearchOptions{
			Query: "Base", Match: ProjectSearchExact, MaxResults: 2,
			After: after, AfterOccurrences: afterOccurrences,
		})
		if err != nil {
			t.Fatal(err)
		}
		combined = append(combined, result.Matches...)
		if !result.Truncated {
			break
		}
		if len(result.Matches) == 0 || result.LastOccurrences <= 0 {
			t.Fatalf("page did not advance: %+v", result)
		}
		last := result.Matches[len(result.Matches)-1]
		after = &last
		afterOccurrences = result.LastOccurrences
	}
	if !reflect.DeepEqual(combined, want.Matches) {
		t.Fatalf("paged search diverges: got=%+v want=%+v", combined, want.Matches)
	}
}

func TestStructuralSearchTruncatesAfterGlobalOrdering(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("project", "phase13", "search-order")
	earlyPath := filepath.Join(root, "a.ts")
	latePath := filepath.Join(root, "z.ts")
	latestPath := filepath.Join(root, "zz.ts")
	early := projectResolverFacts(t, TypeScriptAnalyzer{}, earlyPath, "export class Child extends Base {}\n")
	late := projectResolverFacts(t, TypeScriptAnalyzer{}, latePath, "export class Base {}\n")
	latest := projectResolverFacts(t, TypeScriptAnalyzer{}, latestPath, "export class Base {}\n")
	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{latest, late, early}, projectResolverLimitsForTest())
	if err != nil {
		t.Fatal(err)
	}

	result, err := model.StructuralSearch(context.Background(), ProjectSearchOptions{Query: "Base", Match: ProjectSearchExact, MaxResults: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Truncated || len(result.Matches) != 1 || result.Matches[0].Path != earlyPath || result.Matches[0].SymbolID != "" {
		t.Fatalf("globally truncated search = %+v", result)
	}
}

func TestProjectQueryDeterminismLimitsCancellationAndUnsupported(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("project", "phase13", "hardening")
	first := projectResolverFacts(t, TypeScriptAnalyzer{}, filepath.Join(root, "a.ts"), "import { B } from \"./b\";\nexport class A extends B {}\n")
	second := projectResolverFacts(t, TypeScriptAnalyzer{}, filepath.Join(root, "b.ts"), "export class B {}\n")
	left, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{first, second}, projectResolverLimitsForTest())
	if err != nil {
		t.Fatal(err)
	}
	right, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{second, first}, projectResolverLimitsForTest())
	if err != nil {
		t.Fatal(err)
	}
	limits := queryLimits()
	leftResult, err := left.QueryRelations(context.Background(), RelationDependencies, queryPathSelector(first), ProjectSelector{}, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	rightResult, err := right.QueryRelations(context.Background(), RelationDependencies, queryPathSelector(first), ProjectSelector{}, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(leftResult, rightResult) {
		t.Fatalf("input order changed query output:\nleft=%+v\nright=%+v", leftResult, rightResult)
	}

	stale := queryPathSelector(first)
	stale.SourceFingerprint = queryZeroDigest()
	if _, err := left.QueryRelations(context.Background(), RelationDependencies, stale, ProjectSelector{}, nil, limits); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("stale selector error = %v kind=%v", err, operation.KindOf(err))
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := left.QueryRelations(cancelled, RelationDependencies, queryPathSelector(first), ProjectSelector{}, nil, limits); operation.KindOf(err) != operation.KindCancelled {
		t.Fatalf("cancel error = %v kind=%v", err, operation.KindOf(err))
	}

	tooSmall := limits
	tooSmall.MaxNodes = 1
	if _, err := left.QueryRelations(context.Background(), RelationTrace, queryPathSelector(first), queryPathSelector(second), nil, tooSmall); operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("node limit error = %v kind=%v", err, operation.KindOf(err))
	}

	if _, err := left.QueryRelations(context.Background(), RelationKind("callers"), querySymbolSelector(second, projectSymbolID(t, second, "B")), ProjectSelector{}, nil, limits); operation.KindOf(err) != operation.KindInvalidInput {
		t.Fatalf("callers error = %v kind=%v", err, operation.KindOf(err))
	}
}

func TestCPPUnresolvedQualifierReusesProjectStructuralSearch(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("project", "cpp-qualified")
	header := projectResolverFacts(t, CPPAnalyzer{}, filepath.Join(root, "printer.hpp"),
		"class printer { public: void print(); };\nnamespace util { void tick(); }\n")
	implementation := projectResolverFacts(t, CPPAnalyzer{}, filepath.Join(root, "printer.cpp"),
		"void printer::print() {}\nvoid util::tick() {}\n")
	model, err := BuildProjectModel(context.Background(), registry,
		[]ProjectFileFacts{header, implementation}, projectResolverLimitsForTest())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		wantCount int
	}{
		{"printer.print", 2},
		{"util.tick", 2},
	} {
		result, err := model.StructuralSearch(context.Background(),
			ProjectSearchOptions{Query: tc.name, Match: ProjectSearchExact, MaxResults: 8})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Matches) != tc.wantCount || result.Truncated {
			t.Fatalf("project structural search %q returned %+v", tc.name, result)
		}
		var sawHeader, sawImplementation bool
		for _, match := range result.Matches {
			if match.QualifiedName != tc.name {
				t.Fatalf("project scope was lost in match %+v", match)
			}
			switch match.Path {
			case header.Path:
				sawHeader = true
			case implementation.Path:
				sawImplementation = true
				if match.Kind != SymbolKindFunction {
					t.Fatalf("unproven C++ owner classified as a class: %+v", match)
				}
			default:
				t.Fatalf("unexpected project path: %+v", match)
			}
		}
		if !sawHeader || !sawImplementation {
			t.Fatalf("project search %q lost a distinct file: %+v", tc.name, result.Matches)
		}
	}
}

func queryPathSelector(facts ProjectFileFacts) ProjectSelector {
	return ProjectSelector{Kind: ProjectSelectorPath, Path: facts.Path, SourceFingerprint: facts.SourceFingerprint}
}

func querySymbolSelector(facts ProjectFileFacts, symbolID string) ProjectSelector {
	return ProjectSelector{Kind: ProjectSelectorSymbol, Path: facts.Path, SymbolID: symbolID, SourceFingerprint: facts.SourceFingerprint}
}

func queryZeroDigest() string {
	return "0000000000000000000000000000000000000000000000000000000000000000"
}

func queryLimits() ProjectQueryLimits {
	return ProjectQueryLimits{MaxResults: 256, MaxNodes: 256, MaxEdges: 1024, MaxDepth: 16}
}
