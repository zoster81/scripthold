package sourceintelligence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

func TestR27Phase12ProjectResolverCrossEcosystemDefinitions(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("project", "phase12")
	tests := []struct {
		name          string
		baseAnalyzer  SourceAnalyzer
		basePath      string
		baseSource    string
		baseQualified string
		childAnalyzer SourceAnalyzer
		childPath     string
		childSource   string
		target        string
		wantStage     ProjectResolutionStage
	}{
		{
			name: "cpp", baseAnalyzer: CPPAnalyzer{}, basePath: filepath.Join(root, "cpp", "base.hpp"),
			baseSource: "namespace Demo { class Base {}; }\n", baseQualified: "Demo.Base",
			childAnalyzer: CPPAnalyzer{}, childPath: filepath.Join(root, "cpp", "child.cpp"),
			childSource: "namespace Demo { class Child : public Base {}; }\n", target: "Base", wantStage: ProjectResolutionSameModule,
		},
		{
			name: "java", baseAnalyzer: JavaAnalyzer{}, basePath: filepath.Join(root, "java", "Base.java"),
			baseSource: "package demo; public class Base {}\n", baseQualified: "demo.Base",
			childAnalyzer: JavaAnalyzer{}, childPath: filepath.Join(root, "java", "Child.java"),
			childSource: "package demo; public class Child extends Base {}\n", target: "Base", wantStage: ProjectResolutionSameModule,
		},
		{
			name: "typescript", baseAnalyzer: TypeScriptAnalyzer{}, basePath: filepath.Join(root, "ts", "base.ts"),
			baseSource: "export interface BaseRepo<T> {}\n", baseQualified: "BaseRepo",
			childAnalyzer: TypeScriptAnalyzer{}, childPath: filepath.Join(root, "ts", "child.ts"),
			childSource: "import { BaseRepo } from \"./base\";\nexport interface Repo<T> extends BaseRepo<T> {}\n", target: "BaseRepo<T>", wantStage: ProjectResolutionDependencyExport,
		},
		{
			name: "ruby", baseAnalyzer: RubyAnalyzer{}, basePath: filepath.Join(root, "ruby", "base.rb"),
			baseSource: "class Base\nend\n", baseQualified: "Base",
			childAnalyzer: RubyAnalyzer{}, childPath: filepath.Join(root, "ruby", "child.rb"),
			childSource: "require_relative \"./base\"\nclass Child < Base\nend\n", target: "Base", wantStage: ProjectResolutionDependencyExport,
		},
		{
			name: "rust", baseAnalyzer: RustAnalyzer{}, basePath: filepath.Join(root, "rust", "store.rs"),
			baseSource: "pub trait Store<T> { fn get(&self) -> T; }\n", baseQualified: "Store",
			childAnalyzer: RustAnalyzer{}, childPath: filepath.Join(root, "rust", "item.rs"),
			childSource: "use crate::store::Store;\npub struct Item<T> { value: T }\nimpl<T> Store<T> for Item<T> { fn get(&self) -> T { todo!() } }\n", target: "Store<T>", wantStage: ProjectResolutionExplicitImport,
		},
		{
			name: "delphi", baseAnalyzer: DelphiAnalyzer{}, basePath: filepath.Join(root, "delphi", "BaseUnit.pas"),
			baseSource: "unit BaseUnit;\ninterface\ntype\n  TBase = class\n  end;\nimplementation\nend.\n", baseQualified: "BaseUnit.TBase",
			childAnalyzer: DelphiAnalyzer{}, childPath: filepath.Join(root, "delphi", "ChildUnit.pas"),
			childSource: "unit ChildUnit;\ninterface\nuses BaseUnit;\ntype\n  TChild = class(TBase)\n  end;\nimplementation\nend.\n", target: "TBase", wantStage: ProjectResolutionDependencyExport,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base := phase12Facts(t, test.baseAnalyzer, test.basePath, test.baseSource)
			child := phase12Facts(t, test.childAnalyzer, test.childPath, test.childSource)
			baseID := phase12SymbolID(t, base, test.baseQualified)
			model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{child, base}, phase12ResolverLimits())
			if err != nil {
				t.Fatal(err)
			}
			reference := phase12ReferenceByTarget(t, model.References(), test.childPath, test.target)
			if reference.Resolution != ResolutionResolved || reference.Evidence != SymbolEvidenceProjectResolved {
				t.Fatalf("reference resolution = %s/%s, want resolved/project-resolved: %+v", reference.Resolution, reference.Evidence, reference)
			}
			if reference.Stage != test.wantStage {
				t.Fatalf("reference stage = %q, want %q: %+v", reference.Stage, test.wantStage, reference)
			}
			definitions := model.Definitions(reference.ID)
			if len(definitions) != 1 || definitions[0].SymbolID != baseID || definitions[0].Path != test.basePath {
				t.Fatalf("definitions = %+v, want %s in %s", definitions, baseID, test.basePath)
			}
			reverse := model.ReferencesTo(baseID)
			if len(reverse) != 1 || reverse[0].ID != reference.ID {
				t.Fatalf("reverse references = %+v, want %s", reverse, reference.ID)
			}
		})
	}
}

func TestR27Phase12ProjectResolverExplicitAliasAndDependencyAdjacency(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	basePath := filepath.Join("project", "kotlin", "Baz.kt")
	childPath := filepath.Join("project", "kotlin", "Child.kt")
	base := phase12Facts(t, KotlinAnalyzer{}, basePath, "package foo.bar\nopen class Baz\n")
	child := phase12Facts(t, KotlinAnalyzer{}, childPath, "package app\nimport foo.bar.Baz as Qux\nclass Child : Qux()\n")
	baseID := phase12SymbolID(t, base, "foo.bar.Baz")

	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{base, child}, phase12ResolverLimits())
	if err != nil {
		t.Fatal(err)
	}
	reference := phase12ReferenceByTarget(t, model.References(), childPath, "Qux")
	if reference.Stage != ProjectResolutionExplicitImport || reference.Resolution != ResolutionResolved || len(reference.Targets) != 1 || reference.Targets[0].SymbolID != baseID {
		t.Fatalf("alias reference = %+v", reference)
	}

	dependencies := model.Dependencies(childPath)
	if len(dependencies) != 1 || dependencies[0].Resolution != ResolutionResolved || len(dependencies[0].Targets) != 1 || dependencies[0].Targets[0].Path != basePath {
		t.Fatalf("dependencies = %+v", dependencies)
	}
	dependents := model.Dependents(basePath)
	if len(dependents) != 1 || dependents[0].Source.Path != childPath {
		t.Fatalf("dependents = %+v", dependents)
	}
}

func TestR27Phase12ProjectResolverSameFilePrecedesBroaderProjectCandidates(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	localPath := filepath.Join("project", "java", "Local.java")
	otherPath := filepath.Join("project", "java", "Other.java")
	local := phase12Facts(t, JavaAnalyzer{}, localPath, "package demo; class Base {} class Child extends Base {}\n")
	other := phase12Facts(t, JavaAnalyzer{}, otherPath, "package other; class Base {}\n")
	localBaseID := phase12SymbolID(t, local, "demo.Base")

	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{other, local}, phase12ResolverLimits())
	if err != nil {
		t.Fatal(err)
	}
	reference := phase12ReferenceByTarget(t, model.References(), localPath, "Base")
	if reference.Stage != ProjectResolutionSameFile || reference.Evidence != SymbolEvidenceScopeResolved || reference.Resolution != ResolutionResolved {
		t.Fatalf("same-file reference = %+v", reference)
	}
	if len(reference.Targets) != 1 || reference.Targets[0].SymbolID != localBaseID {
		t.Fatalf("same-file targets = %+v, want %s", reference.Targets, localBaseID)
	}
}

func TestR27Phase12ProjectResolverKeepsBroadAmbiguityExplicit(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	first := phase12Facts(t, TypeScriptAnalyzer{}, filepath.Join("project", "ts", "one.ts"), "export class Base {}\n")
	second := phase12Facts(t, TypeScriptAnalyzer{}, filepath.Join("project", "ts", "two.ts"), "export class Base {}\n")
	childPath := filepath.Join("project", "ts", "child.ts")
	child := phase12Facts(t, TypeScriptAnalyzer{}, childPath, "export class Child extends Base {}\n")

	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{child, second, first}, phase12ResolverLimits())
	if err != nil {
		t.Fatal(err)
	}
	reference := phase12ReferenceByTarget(t, model.References(), childPath, "Base")
	if reference.Stage != ProjectResolutionProject || reference.Resolution != ResolutionAmbiguous || reference.Evidence != SymbolEvidenceStructural || len(reference.Targets) != 2 {
		t.Fatalf("ambiguous reference = %+v", reference)
	}
	if len(model.Definitions(reference.ID)) != 2 {
		t.Fatalf("ambiguous definitions = %+v", model.Definitions(reference.ID))
	}
}

func TestR27Phase12ProjectResolverDependencyExternalAndDeterministic(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	first := phase12Facts(t, TypeScriptAnalyzer{}, filepath.Join("project", "ts", "one.ts"), "import { Local } from \"./local\";\nimport { missing } from \"./missing\";\nimport thing from \"external-package\";\nexport class One {}\n")
	second := phase12Facts(t, TypeScriptAnalyzer{}, filepath.Join("project", "ts", "local.ts"), "export class Local {}\n")
	unrelated := phase12Facts(t, TypeScriptAnalyzer{}, filepath.Join("project", "ts", "other.ts"), "export class missing {}\n")
	limits := phase12ResolverLimits()
	left, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{first, second, unrelated}, limits)
	if err != nil {
		t.Fatal(err)
	}
	right, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{unrelated, second, first}, limits)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(left.AllDependencies(), right.AllDependencies()) || !reflect.DeepEqual(left.References(), right.References()) {
		t.Fatalf("project model depends on input order:\nleft deps=%+v refs=%+v\nright deps=%+v refs=%+v", left.AllDependencies(), left.References(), right.AllDependencies(), right.References())
	}

	dependencies := left.Dependencies(first.Path)
	if len(dependencies) != 3 {
		t.Fatalf("dependencies = %+v", dependencies)
	}
	states := map[string]ResolutionState{}
	for _, dependency := range dependencies {
		states[dependency.Dependency.Value] = dependency.Resolution
	}
	if states["./local"] != ResolutionResolved || states["./missing"] != ResolutionExternal || states["external-package"] != ResolutionExternal {
		t.Fatalf("dependency states = %+v", states)
	}
}

func TestR27Phase12ProjectResolverFilteringDoesNotMutateSharedIndexSlice(t *testing.T) {
	input := []projectSymbolRecord{{pathKey: "first"}, {pathKey: "second"}}
	filtered := excludePath(input, "first")
	if len(filtered) != 1 || filtered[0].pathKey != "second" {
		t.Fatalf("filtered = %+v", filtered)
	}
	if len(input) != 2 || input[0].pathKey != "first" || input[1].pathKey != "second" {
		t.Fatalf("excludePath mutated its input: %+v", input)
	}
}

func TestR27Phase12ProjectResolverMalformedGenericSpellingFailsClosed(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	base := phase12Facts(t, JavaAnalyzer{}, filepath.Join("project", "java", "Base.java"), "class Base {}\n")
	childPath := filepath.Join("project", "java", "Child.java")
	child := phase12Facts(t, JavaAnalyzer{}, childPath, "class Child extends Base {}\n")
	if len(child.Analysis.Relations) != 1 {
		t.Fatalf("child relations = %+v", child.Analysis.Relations)
	}
	child.Analysis.Relations[0].Target = "Base<"

	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{base, child}, phase12ResolverLimits())
	if err != nil {
		t.Fatal(err)
	}
	reference := phase12ReferenceByTarget(t, model.References(), childPath, "Base<")
	if reference.Resolution != ResolutionUnresolved || reference.Stage != ProjectResolutionNone || len(reference.Targets) != 0 {
		t.Fatalf("malformed generic reference resolved unexpectedly: %+v", reference)
	}
}

func TestR27Phase12ProjectResolverCandidateLimitFailsClosed(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	child := phase12Facts(t, TypeScriptAnalyzer{}, filepath.Join("project", "ts", "child.ts"), "export class Child extends Base {}\n")
	first := phase12Facts(t, TypeScriptAnalyzer{}, filepath.Join("project", "ts", "one.ts"), "export class Base {}\n")
	second := phase12Facts(t, TypeScriptAnalyzer{}, filepath.Join("project", "ts", "two.ts"), "export class Base {}\n")
	third := phase12Facts(t, TypeScriptAnalyzer{}, filepath.Join("project", "ts", "three.ts"), "export class Base {}\n")
	limits := phase12ResolverLimits()
	limits.MaxCandidatesPerResolution = 2
	if _, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{child, first, second, third}, limits); operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("candidate limit error = %v kind=%v", err, operation.KindOf(err))
	}
}

func TestR27Phase12ProjectResolverLimitsCancellationAndDuplicatePath(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	first := phase12Facts(t, JavaAnalyzer{}, filepath.Join("project", "java", "A.java"), "class A {}\n")
	second := phase12Facts(t, JavaAnalyzer{}, filepath.Join("project", "java", "B.java"), "class B {}\n")

	limits := phase12ResolverLimits()
	limits.MaxFiles = 1
	if _, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{first, second}, limits); operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("file limit error = %v kind=%v", err, operation.KindOf(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := BuildProjectModel(ctx, registry, []ProjectFileFacts{first}, phase12ResolverLimits()); operation.KindOf(err) != operation.KindCancelled {
		t.Fatalf("cancellation error = %v kind=%v", err, operation.KindOf(err))
	}

	duplicate := second
	duplicate.Path = first.Path
	if _, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{first, duplicate}, phase12ResolverLimits()); operation.KindOf(err) != operation.KindInvalidInput {
		t.Fatalf("duplicate path error = %v kind=%v", err, operation.KindOf(err))
	}
}

func phase12Facts(t *testing.T, analyzer SourceAnalyzer, path, text string) ProjectFileFacts {
	t.Helper()
	document := sourceDocumentForScanner(text)
	document.Path = path
	sum := sha256.Sum256([]byte(text))
	document.SourceFingerprint = hex.EncodeToString(sum[:])
	result, err := analyzer.Analyze(context.Background(), document, phase3AnalyzeOptions(false, 512))
	if err != nil {
		t.Fatalf("analyze %s: %v", path, err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("analysis for %s is partial: %+v", path, result.Analysis.Diagnostics)
	}
	return ProjectFileFacts{Path: path, Language: analyzer.Language(), SourceFingerprint: document.SourceFingerprint, Analysis: result}
}

func phase12SymbolID(t *testing.T, facts ProjectFileFacts, qualifiedName string) string {
	t.Helper()
	for _, symbol := range facts.Analysis.Analysis.Symbols {
		if symbol.QualifiedName == qualifiedName {
			return symbol.ID
		}
	}
	t.Fatalf("symbol %q not found in %s: %+v", qualifiedName, facts.Path, facts.Analysis.Analysis.Symbols)
	return ""
}

func phase12ReferenceByTarget(t *testing.T, references []ProjectReference, sourcePath, target string) ProjectReference {
	t.Helper()
	for _, reference := range references {
		if reference.Source.Path == sourcePath && reference.TargetSpelling == target {
			return reference
		}
	}
	t.Fatalf("reference to %q not found in %s: %+v", target, sourcePath, references)
	return ProjectReference{}
}

func phase12ResolverLimits() ProjectResolverLimits {
	return ProjectResolverLimits{MaxFiles: 128, MaxSymbols: 4096, MaxDependencies: 4096, MaxReferences: 4096}
}
