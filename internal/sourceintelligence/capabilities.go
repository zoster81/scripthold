package sourceintelligence

import (
	"fmt"
	"sort"
	"strings"
)

// LanguageCapabilityRow is the mechanically checkable R27 support statement for
// one canonical language/source-format entry. A row exists even when analysis is
// intentionally unimplemented, preventing routing metadata from being mistaken
// for production source-intelligence support.
type LanguageCapabilityRow struct {
	ID                string
	Family            string
	DetectionEvidence []EvidenceKind
	ScannerProfile    string
	CompositeBehavior string
	Capabilities      LanguageCapabilities
	EncodingCoverage  string
	Analyzer          AnalyzerID
	AnalyzerStrategy  string
	AnalyzerVersion   string
	KnownLimitations  []string
}

func (registry *LanguageRegistry) CapabilityRows() []LanguageCapabilityRow {
	if registry == nil {
		return nil
	}
	ids := make([]string, 0, len(registry.byID))
	for id := range registry.byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	rows := make([]LanguageCapabilityRow, 0, len(ids))
	for _, id := range ids {
		descriptor := registry.byID[id]
		rows = append(rows, LanguageCapabilityRow{
			ID: descriptor.ID, Family: descriptor.Family,
			DetectionEvidence: append([]EvidenceKind(nil), descriptor.DetectionEvidence...),
			ScannerProfile:    descriptor.ScannerProfile, CompositeBehavior: descriptor.CompositeBehavior,
			Capabilities: descriptor.Capabilities, EncodingCoverage: descriptor.EncodingCoverage,
			Analyzer: descriptor.Analyzer, AnalyzerStrategy: descriptor.AnalyzerStrategy, AnalyzerVersion: descriptor.AnalyzerVersion,
			KnownLimitations: append([]string(nil), descriptor.KnownLimitations...),
		})
	}
	return rows
}

// RenderLanguageCapabilityMatrixMarkdown projects the authoritative registry
// rows into deterministic public documentation. The checked-in document is
// tested byte-for-byte against this renderer.
func RenderLanguageCapabilityMatrixMarkdown(registry *LanguageRegistry) string {
	rows := registry.CapabilityRows()
	var builder strings.Builder
	builder.WriteString("# R27 Language Capability Matrix\n\n")
	builder.WriteString("This file is a generated projection of the native source-intelligence registry. Do not infer production support from detection/routing alone: rows whose analyzer strategy is `unimplemented` are explicitly planned metadata only. The checked-in file is verified byte-for-byte against the registry by tests.\n\n")
	builder.WriteString("Capability codes: `decl` declarations, `hier` hierarchy, `sig` signatures, `range` decoded-source ranges, `dep` imports/includes/dependencies, `inh` inheritance/implements structural relations, `call` syntactic calls, `scope-ref` scope-resolved references, `project-ref` project-resolved references, `project-def` project-resolved definitions, `impl` implementations, `override` overrides, `semantic` semantic relations, `index` incremental indexing. Missing codes mean the capability is not currently claimed.\n\n")
	builder.WriteString("| Canonical ID | Family | Detection evidence | Scanner/lexer profile | Composite | Capabilities | Encoding coverage | Analyzer | Known limitations |\n")
	builder.WriteString("|---|---|---|---|---|---|---|---|---|\n")
	for _, row := range rows {
		builder.WriteString("| `")
		builder.WriteString(markdownCell(row.ID))
		builder.WriteString("` | ")
		builder.WriteString(markdownCell(row.Family))
		builder.WriteString(" | ")
		builder.WriteString(markdownCell(joinEvidence(row.DetectionEvidence)))
		builder.WriteString(" | `")
		builder.WriteString(markdownCell(row.ScannerProfile))
		builder.WriteString("` | ")
		builder.WriteString(markdownCell(row.CompositeBehavior))
		builder.WriteString(" | ")
		builder.WriteString(markdownCell(capabilityCodes(row.Capabilities)))
		builder.WriteString(" | ")
		builder.WriteString(markdownCell(row.EncodingCoverage))
		builder.WriteString(" | ")
		builder.WriteString(markdownCell(analyzerSummary(row)))
		builder.WriteString(" | ")
		builder.WriteString(markdownCell(strings.Join(row.KnownLimitations, "; ")))
		builder.WriteString(" |\n")
	}
	return builder.String()
}

func joinEvidence(values []EvidenceKind) string {
	parts := make([]string, len(values))
	for index, value := range values {
		parts[index] = string(value)
	}
	return strings.Join(parts, ", ")
}

func capabilityCodes(value LanguageCapabilities) string {
	codes := make([]string, 0, 14)
	for _, item := range []struct {
		enabled bool
		code    string
	}{
		{value.Declarations, "decl"}, {value.Hierarchy, "hier"}, {value.Signatures, "sig"}, {value.Ranges, "range"},
		{value.Dependencies, "dep"}, {value.InheritanceRelations, "inh"}, {value.SyntacticCalls, "call"},
		{value.ScopeResolvedReferences, "scope-ref"}, {value.ProjectResolvedReferences, "project-ref"}, {value.ProjectResolvedDefinitions, "project-def"},
		{value.Implementations, "impl"}, {value.Overrides, "override"}, {value.SemanticRelations, "semantic"}, {value.IncrementalIndex, "index"},
	} {
		if item.enabled {
			codes = append(codes, item.code)
		}
	}
	if len(codes) == 0 {
		return "none"
	}
	return strings.Join(codes, ", ")
}

func analyzerSummary(row LanguageCapabilityRow) string {
	if row.Analyzer == "" {
		return row.AnalyzerStrategy + "/" + row.AnalyzerVersion
	}
	return string(row.Analyzer) + " - " + row.AnalyzerStrategy + "/" + row.AnalyzerVersion
}

func markdownCell(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}

func enrichLanguageDescriptor(descriptor LanguageDescriptor) LanguageDescriptor {
	if descriptor.Family == "" {
		descriptor.Family = languageFamily(descriptor.ID)
	}
	if len(descriptor.DetectionEvidence) == 0 {
		descriptor.DetectionEvidence = detectionEvidenceForDescriptor(descriptor)
	}
	if descriptor.EncodingCoverage == "" {
		descriptor.EncodingCoverage = "decoded-source-document"
	}
	if descriptor.CompositeBehavior == "" {
		if descriptor.Capabilities.Composite {
			descriptor.CompositeBehavior = "host-and-embedded"
		} else {
			descriptor.CompositeBehavior = "none"
		}
	}
	if descriptor.Capabilities.SourceAnalysis {
		descriptor.Capabilities.Declarations = true
		descriptor.Capabilities.Hierarchy = true
		descriptor.Capabilities.Signatures = true
		descriptor.Capabilities.Ranges = true
		descriptor.Capabilities.Dependencies = true
		switch descriptor.ID {
		case "vbnet", "cpp", "java", "kotlin", "javascript", "typescript", "rust", "php", "ruby", "swift", "pascal", "delphi", "fsharp", "cpp-cli", "jscript-net", "cil", "powershell", "mql4", "mql5", "objective-c", "objective-cpp", "dart", "d", "nim", "solidity", "apex", "al", "arduino", "groovy", "autohotkey":
			descriptor.Capabilities.InheritanceRelations = true
		}
		if descriptor.ScannerProfile == "" || descriptor.AnalyzerStrategy == "" || descriptor.AnalyzerVersion == "" {
			descriptor.ScannerProfile, descriptor.AnalyzerStrategy, descriptor.AnalyzerVersion = activeProviderMetadata(descriptor.Analyzer)
		}
		if len(descriptor.KnownLimitations) == 0 {
			switch descriptor.ID {
			case "classic-asp":
				descriptor.KnownLimitations = []string{"VBScript and JScript server regions are structural only; dynamic ASP runtime objects, unsupported script engines, include/project resolution, semantic relations, and incremental indexing are not implemented"}
			case "c", "cpp":
				descriptor.KnownLimitations = []string{"macro expansion, conditional-preprocessor evaluation, compile-database/type resolution, semantic relations, and incremental indexing are not implemented"}
			case "java", "kotlin":
				descriptor.KnownLimitations = []string{"classpath/build-model/type resolution, semantic relations, and incremental indexing are not implemented"}
			case "javascript":
				descriptor.KnownLimitations = []string{"dynamic property/prototype resolution, non-literal module loading, JSX component semantics, project resolution, semantic relations, and incremental indexing are not implemented"}
			case "typescript":
				descriptor.KnownLimitations = []string{"TypeScript type checking/project resolution, non-literal module loading, TSX component semantics, semantic relations, and incremental indexing are not implemented"}
			case "rust":
				descriptor.KnownLimitations = []string{"macro expansion, cfg evaluation, Cargo/project/type/trait resolution, semantic relations, and incremental indexing are not implemented"}
			case "php":
				descriptor.KnownLimitations = []string{"dynamic/non-literal includes, autoload/runtime metaprogramming, project/type resolution, semantic relations, and incremental indexing are not implemented"}
			case "ruby":
				descriptor.KnownLimitations = []string{"reopened declarations remain structural; DSL/metaprogramming, dynamic dispatch, project resolution, semantic relations, and incremental indexing are not implemented"}
			case "swift":
				descriptor.KnownLimitations = []string{"macro expansion, conditional compilation, package/type resolution, semantic relations, and incremental indexing are not implemented"}
			case "pascal":
				descriptor.KnownLimitations = []string{"compiler directives and conditional compilation are not evaluated; project/type resolution, semantic relations, and incremental indexing are not implemented"}
			case "delphi":
				descriptor.KnownLimitations = []string{"compiler directives and conditional compilation are not evaluated; helpers/generics remain structural; project/package/type resolution, semantic relations, and incremental indexing are not implemented"}
			case "vb6", "vba":
				descriptor.KnownLimitations = []string{"COM/project references, designer/event binding, conditional compilation, runtime dispatch, project/type resolution, semantic relations, and incremental indexing are not implemented"}
			case "vbscript":
				descriptor.KnownLimitations = []string{"Eval/Execute/CreateObject and runtime dispatch are not evaluated; project/host resolution, semantic relations, and incremental indexing are not implemented"}
			case "qbasic", "classic-basic":
				descriptor.KnownLimitations = []string{"line-number and GOTO/GOSUB control-flow resolution, runtime typing, cross-dialect disambiguation, semantic relations, and incremental indexing are not implemented"}
			case "freebasic", "purebasic":
				descriptor.KnownLimitations = []string{"macro/preprocessor expansion, project/type resolution, runtime semantics, semantic relations, and incremental indexing are not implemented"}
			case "fsharp":
				descriptor.KnownLimitations = []string{"type inference, computation-expression semantics, signature/project linking, compiler resolution, semantic relations, and incremental indexing are not implemented"}
			case "cpp-cli":
				descriptor.KnownLimitations = []string{"CLR generic/attribute/ref semantics, metadata/project/type resolution, preprocessor evaluation, semantic relations, and incremental indexing are not implemented"}
			case "jscript-net":
				descriptor.KnownLimitations = []string{"CLR binding, package/type resolution, dynamic runtime semantics, project resolution, semantic relations, and incremental indexing are not implemented"}
			case "cil":
				descriptor.KnownLimitations = []string{"metadata token resolution, stack/control-flow analysis, assembly binding, semantic relations, and incremental indexing are not implemented"}
			case "powershell":
				descriptor.KnownLimitations = []string{"profiles, dynamic invocation, module/class resolution, interpolation semantics, semantic relations, and incremental indexing are not implemented"}
			case "aspnet-webforms":
				descriptor.KnownLimitations = []string{"declared C#/VB server regions plus host HTML and supported client script/style regions are analyzed structurally; generated partial classes, page lifecycle/event binding, template/runtime semantics, project resolution, semantic relations, and incremental indexing are not implemented"}
			case "razor", "blazor":
				descriptor.KnownLimitations = []string{"balanced @code/@functions regions plus host HTML and supported client script/style regions are analyzed structurally; inline Razor expressions/directives outside supported member regions, generated component/render semantics, project/type resolution, semantic relations, and incremental indexing are not implemented"}
			case "xaml":
				descriptor.KnownLimitations = []string{"x:Class/x:Name/xmlns declarations are structural only; bindings, resources, code-behind resolution, semantic relations, and incremental indexing are not implemented"}
			case "mql4", "mql5":
				descriptor.KnownLimitations = []string{"macro expansion and conditional preprocessing are not evaluated; imported binaries, trading runtime state, project/type resolution, semantic relations, and incremental indexing are not implemented"}
			case "objective-c", "objective-cpp":
				descriptor.KnownLimitations = []string{"preprocessor/macro expansion, categories/runtime dispatch, framework/project/type resolution, semantic relations, and incremental indexing are not implemented"}
			case "dart":
				descriptor.KnownLimitations = []string{"extension/member binding, mixin/type inference, package/project resolution, semantic relations, and incremental indexing are not implemented"}
			case "d":
				descriptor.KnownLimitations = []string{"templates/mixins/CTFE/version evaluation, package/type resolution, semantic relations, and incremental indexing are not implemented"}
			case "zig":
				descriptor.KnownLimitations = []string{"comptime evaluation, inferred/container semantics, build/project resolution, semantic relations, and incremental indexing are not implemented"}
			case "nim":
				descriptor.KnownLimitations = []string{"macro/template expansion, conditional compilation, module/type resolution, semantic relations, and incremental indexing are not implemented"}
			case "solidity":
				descriptor.KnownLimitations = []string{"modifier execution, inheritance linearization, ABI/type/project resolution, semantic relations, and incremental indexing are not implemented"}
			case "apex":
				descriptor.KnownLimitations = []string{"SOQL/runtime metadata, trigger dispatch, org/project/type resolution, semantic relations, and incremental indexing are not implemented"}
			case "al":
				descriptor.KnownLimitations = []string{"object IDs and extension targets remain structural; package/application binding, generated/runtime behavior, semantic relations, and incremental indexing are not implemented"}
			case "arduino":
				descriptor.KnownLimitations = []string{"Arduino prototype generation, board/core preprocessing, library/project resolution, semantic relations, and incremental indexing are not implemented"}
			case "perl":
				descriptor.KnownLimitations = []string{"runtime symbol-table mutation, BEGIN/eval/metaprogramming, non-literal module loading, method dispatch, project resolution, semantic relations, and incremental indexing are not implemented"}
			case "lua", "luau":
				descriptor.KnownLimitations = []string{"metatables, runtime table/member mutation, non-literal require targets, dynamic dispatch, project/type resolution, semantic relations, and incremental indexing are not implemented"}
			case "elixir":
				descriptor.KnownLimitations = []string{"macro expansion, quoted/generated code, protocol/behaviour resolution, runtime dispatch, Mix/project resolution, semantic relations, and incremental indexing are not implemented"}
			case "erlang":
				descriptor.KnownLimitations = []string{"preprocessor expansion, parse transforms, generated code, behaviour/call resolution, OTP/project resolution, semantic relations, and incremental indexing are not implemented"}
			case "gleam":
				descriptor.KnownLimitations = []string{"type inference/checking, generated code, package/project resolution, call resolution, semantic relations, and incremental indexing are not implemented"}
			case "groovy":
				descriptor.KnownLimitations = []string{"AST transforms, scripts/DSL metaprogramming, dynamic dispatch, Gradle/project/type resolution, semantic relations, and incremental indexing are not implemented"}
			case "shell", "bash":
				descriptor.KnownLimitations = []string{"expansion/eval/source execution, aliases, dynamically generated functions, command resolution, environment/runtime semantics, project resolution, semantic relations, and incremental indexing are not implemented"}
			case "tcl":
				descriptor.KnownLimitations = []string{"eval/uplevel/runtime command creation, dynamic namespaces, non-literal source/package targets, project resolution, semantic relations, and incremental indexing are not implemented"}
			case "autohotkey":
				descriptor.KnownLimitations = []string{"runtime hotkey/label creation, dynamic calls/properties, include expression evaluation, project resolution, semantic relations, and incremental indexing are not implemented"}
			case "fortran":
				descriptor.KnownLimitations = []string{"preprocessor/include expansion, generic/interface and module/type resolution, compiler/project semantics, semantic relations, and incremental indexing are not implemented; fixed source form is selected only for fixed-form extensions"}
			case "cobol":
				descriptor.KnownLimitations = []string{"COPY replacement, compiler directives, data/control-flow resolution, dialect-specific semantics, project resolution, semantic relations, and incremental indexing are not implemented"}
			case "ada":
				descriptor.KnownLimitations = []string{"generic instantiation, renames/overload/tasking semantics, project/type resolution, semantic relations, and incremental indexing are not implemented"}
			case "matlab", "octave":
				descriptor.KnownLimitations = []string{"dynamic workspace/path behavior, eval/metaprogramming, class dispatch, toolbox/package/project resolution, semantic relations, and incremental indexing are not implemented; shared .m files remain ambiguity-safe without distinctive evidence"}
			case "julia":
				descriptor.KnownLimitations = []string{"macro/generated-code expansion, multiple-dispatch/type inference, package/project resolution, semantic relations, and incremental indexing are not implemented"}
			case "r":
				descriptor.KnownLimitations = []string{"non-standard evaluation, environment/S3/S4/R6 runtime semantics, dynamic package resolution, project resolution, semantic relations, and incremental indexing are not implemented"}
			case "haskell":
				descriptor.KnownLimitations = []string{"CPP, Template Haskell, type inference/typeclass resolution, package/module project resolution, semantic relations, and incremental indexing are not implemented"}
			case "ocaml":
				descriptor.KnownLimitations = []string{"functor/type/module inference, PPX expansion, Dune/project resolution, semantic relations, and incremental indexing are not implemented"}
			case "common-lisp":
				descriptor.KnownLimitations = []string{"reader-macro and macro expansion, CLOS/runtime package mutation, implementation/project resolution, semantic relations, and incremental indexing are not implemented"}
			case "clojure":
				descriptor.KnownLimitations = []string{"macro expansion, reader conditionals/tagged literals, protocol/runtime dispatch, classpath/project resolution, semantic relations, and incremental indexing are not implemented"}
			case "emacs-lisp":
				descriptor.KnownLimitations = []string{"macro/advice/autoload expansion, runtime symbol mutation, package/load-path resolution, semantic relations, and incremental indexing are not implemented"}
			case "sql":
				descriptor.KnownLimitations = []string{"SQL support is structural and profile-bounded to common DDL plus selected PostgreSQL and SQL Server forms; query planning, schema/catalog resolution, procedural semantics, database connectivity, semantic relations, and incremental indexing are not implemented"}
			case "plsql":
				descriptor.KnownLimitations = []string{"package/procedure/function declarations are structural only; overload/type/schema resolution, dynamic SQL, database connectivity, semantic relations, and incremental indexing are not implemented"}
			case "graphql":
				descriptor.KnownLimitations = []string{"schema/type/field and named operation structure is extracted without schema composition, fragment/variable/type validation, resolver/runtime semantics, project resolution, semantic relations, or incremental indexing"}
			case "terraform":
				descriptor.KnownLimitations = []string{"Terraform and generic HCL blocks are structural only; expression evaluation, interpolation, provider/module registry resolution, state/plan semantics, dependency graph resolution, semantic relations, and incremental indexing are not implemented"}
			case "nix":
				descriptor.KnownLimitations = []string{"let bindings are structural only; lazy evaluation, imports, attribute-set/module/package semantics, derivation evaluation, project resolution, semantic relations, and incremental indexing are not implemented"}
			case "proto":
				descriptor.KnownLimitations = []string{"package/message/enum/service/rpc structure and literal imports are extracted without descriptor linking, option evaluation, generated-code semantics, type/project resolution, semantic relations, or incremental indexing"}
			case "vhdl":
				descriptor.KnownLimitations = []string{"entity/architecture/signal structure is extracted without elaboration, generic/port/type binding, library/project resolution, simulation semantics, semantic relations, or incremental indexing"}
			case "verilog", "systemverilog":
				descriptor.KnownLimitations = []string{"module/interface/package/signal structure is bounded and macro-free; preprocessing, generate/elaboration, parameter/type/net binding, simulation semantics, project resolution, semantic relations, and incremental indexing are not implemented"}
			case "assembly":
				descriptor.KnownLimitations = []string{"GAS/NASM-style labels, numeric local labels, and MASM PROC declarations are structural only; macro expansion, instruction decoding, control-flow/symbol/linker resolution, architecture semantics, semantic relations, and incremental indexing are not implemented"}
			case "html", "xml":
				descriptor.KnownLimitations = []string{"only elements with explicit id attributes are navigation symbols; DOM/schema/namespace validation, template/runtime semantics, cross-file resolution, semantic relations, and incremental indexing are not implemented"}
			case "css", "scss", "sass", "less":
				descriptor.KnownLimitations = []string{"selector and supported variable structure is lexical/structural only; nesting expansion, mixins/functions, imports/modules, cascade/style computation, semantic relations, and incremental indexing are not implemented"}
			case "json":
				descriptor.KnownLimitations = []string{"object keys are navigation symbols; arrays and scalar values are not independently indexed, and schema validation, reference resolution, semantic relations, and incremental indexing are not implemented"}
			case "yaml":
				descriptor.KnownLimitations = []string{"mapping keys are indentation-based structural navigation; anchors, aliases, tags, merge semantics, complex keys, schema validation, semantic relations, and incremental indexing are not implemented"}
			case "toml":
				descriptor.KnownLimitations = []string{"sections and simple keys are structural navigation; arrays-of-tables, dotted/quoted key edge cases, semantic validation, cross-file resolution, semantic relations, and incremental indexing are not implemented"}
			case "markdown":
				descriptor.KnownLimitations = []string{"ATX headings outside fenced code are indexed; setext headings, link/reference resolution, embedded-language semantics, semantic relations, and incremental indexing are not implemented"}
			case "openapi":
				descriptor.KnownLimitations = []string{"root OpenAPI 3.x detection plus operationId/component-schema navigation is structural only; YAML/JSON schema semantics, $ref resolution, path/parameter validation, code generation, semantic relations, and incremental indexing are not implemented"}
			case "ansible-yaml":
				descriptor.KnownLimitations = []string{"root play/task name navigation is structural only; roles/includes/imports, variables, inventory, module/plugin resolution, execution semantics, semantic relations, and incremental indexing are not implemented"}
			case "vue", "svelte", "astro":
				descriptor.KnownLimitations = []string{"host markup plus supported script/style regions are analyzed with offset-preserving masking; framework compilation, template expression semantics, component resolution, generated code, semantic relations, and incremental indexing are not implemented"}
			case "php-html", "jsp", "ejs":
				descriptor.KnownLimitations = []string{"host markup plus explicit embedded code regions are analyzed structurally with original host coordinates; template/runtime execution, generated output, project resolution, semantic relations, and incremental indexing are not implemented"}
			case "jinja", "twig":
				descriptor.KnownLimitations = []string{"host markup plus structural block/macro declarations are indexed; template expressions, filters/tests, inheritance resolution, runtime rendering, semantic relations, and incremental indexing are not implemented"}
			case "blade":
				descriptor.KnownLimitations = []string{"host markup, section declarations and explicit @php regions are indexed; Blade expression/directive execution, component/view resolution, generated output, semantic relations, and incremental indexing are not implemented"}
			default:
				descriptor.KnownLimitations = []string{"project resolution, syntactic call graph, semantic relations, and incremental indexing are not implemented"}
			}
		}
	} else {
		if descriptor.ScannerProfile == "" {
			descriptor.ScannerProfile = "unimplemented"
		}
		if descriptor.AnalyzerStrategy == "" {
			descriptor.AnalyzerStrategy = "unimplemented"
		}
		if descriptor.AnalyzerVersion == "" {
			descriptor.AnalyzerVersion = "none"
		}
		if len(descriptor.KnownLimitations) == 0 {
			descriptor.KnownLimitations = []string{"source analysis is not implemented yet"}
		}
	}
	return descriptor
}

func activeProviderMetadata(analyzer AnalyzerID) (scannerProfile, strategy, version string) {
	switch analyzer {
	case AnalyzerGo:
		return "go-stdlib-ast", "stdlib-ast", "r25-v1"
	case AnalyzerCSharp:
		return "csharp", "native-token-structural", "r25-v1"
	case AnalyzerVBNet:
		return "vbnet", "native-token-structural", "r25-v1"
	case AnalyzerPython:
		return "python", "native-token-structural", "r25-v1"
	case AnalyzerClassicASP:
		return "classic-asp-composite", "native-composite-delegating", "r27-p6-v1"
	case AnalyzerC:
		return "c", "native-token-structural", "r27-p3-v1"
	case AnalyzerCPP:
		return "cpp", "native-token-structural", "r27-p3-v1"
	case AnalyzerJava:
		return "java", "native-token-structural", "r27-p3-v1"
	case AnalyzerKotlin:
		return "kotlin", "native-token-structural", "r27-p3-v1"
	case AnalyzerJavaScript:
		return "javascript", "native-token-structural", "r27-p4-v1"
	case AnalyzerTypeScript:
		return "typescript", "native-token-structural", "r27-p4-v1"
	case AnalyzerRust:
		return "rust", "native-token-structural", "r27-p4-v1"
	case AnalyzerPHP:
		return "php", "native-token-structural", "r27-p5-v1"
	case AnalyzerRuby:
		return "ruby", "native-line-structural", "r27-p5-v1"
	case AnalyzerSwift:
		return "swift", "native-token-structural", "r27-p5-v1"
	case AnalyzerPascal:
		return "pascal", "native-line-structural", "r27-p5-v1"
	case AnalyzerDelphi:
		return "delphi", "native-line-structural", "r27-p5-v1"
	case AnalyzerVB6, AnalyzerVBA, AnalyzerVBScript, AnalyzerQBasic, AnalyzerClassicBasic, AnalyzerFreeBasic, AnalyzerPureBasic:
		return "basic-dialect", "native-line-structural", "r27-p6-v1"
	case AnalyzerFSharp:
		return "fsharp", "native-line-structural", "r27-p6-v1"
	case AnalyzerCPPCLI:
		return "cpp-cli", "native-adapter-structural", "r27-p6-v1"
	case AnalyzerJScriptNet:
		return "jscript-net", "native-composite-structural", "r27-p6-v1"
	case AnalyzerCIL:
		return "cil", "native-line-structural", "r27-p6-v1"
	case AnalyzerPowerShell:
		return "powershell", "native-token-structural", "r27-p6-v1"
	case AnalyzerASPNetWebForms, AnalyzerRazor, AnalyzerBlazor:
		return "dotnet-composite", "native-composite-delegating", "r27-p11-v1"
	case AnalyzerXAML:
		return "xaml", "native-document-structural", "r27-p6-v1"
	case AnalyzerMQL4, AnalyzerMQL5:
		return "mql", "native-adapter-structural", "r27-p7-v1"
	case AnalyzerObjectiveC, AnalyzerObjectiveCPP:
		return "objective-c", "native-hybrid-structural", "r27-p7-v1"
	case AnalyzerDart:
		return "dart", "native-token-structural", "r27-p7-v1"
	case AnalyzerD:
		return "d", "native-token-structural", "r27-p7-v1"
	case AnalyzerZig:
		return "zig", "native-token-structural", "r27-p7-v1"
	case AnalyzerNim:
		return "nim", "native-line-structural", "r27-p7-v1"
	case AnalyzerSolidity:
		return "solidity", "native-token-structural", "r27-p7-v1"
	case AnalyzerApex:
		return "apex", "native-token-structural", "r27-p7-v1"
	case AnalyzerAL:
		return "al", "native-token-structural", "r27-p7-v1"
	case AnalyzerArduino:
		return "arduino", "native-adapter-structural", "r27-p7-v1"
	case AnalyzerPerl:
		return "perl", "native-line-structural", "r27-p8-v1"
	case AnalyzerLua:
		return "lua", "native-line-structural", "r27-p8-v1"
	case AnalyzerLuau:
		return "luau", "native-line-structural", "r27-p8-v1"
	case AnalyzerElixir:
		return "elixir", "native-line-structural", "r27-p8-v1"
	case AnalyzerErlang:
		return "erlang", "native-clause-structural", "r27-p8-v1"
	case AnalyzerGleam:
		return "gleam", "native-token-structural", "r27-p8-v1"
	case AnalyzerGroovy:
		return "groovy", "native-token-structural", "r27-p8-v1"
	case AnalyzerShell:
		return "shell", "native-token-structural", "r27-p8-v1"
	case AnalyzerBash:
		return "bash", "native-token-structural", "r27-p8-v1"
	case AnalyzerTcl:
		return "tcl", "native-command-structural", "r27-p8-v1"
	case AnalyzerAutoHotkey:
		return "autohotkey", "native-token-structural", "r27-p8-v1"
	case AnalyzerFortran:
		return "fortran", "native-fixed-free-structural", "r27-p9-v1"
	case AnalyzerCOBOL:
		return "cobol", "native-fixed-line-structural", "r27-p9-v1"
	case AnalyzerAda:
		return "ada", "native-line-structural", "r27-p9-v1"
	case AnalyzerMATLAB:
		return "matlab", "native-line-structural", "r27-p9-v1"
	case AnalyzerOctave:
		return "octave", "native-line-structural", "r27-p9-v1"
	case AnalyzerJulia:
		return "julia", "native-line-structural", "r27-p9-v1"
	case AnalyzerR:
		return "r", "native-line-structural", "r27-p9-v1"
	case AnalyzerHaskell:
		return "haskell", "native-line-structural", "r27-p9-v1"
	case AnalyzerOCaml:
		return "ocaml", "native-line-structural", "r27-p9-v1"
	case AnalyzerCommonLisp:
		return "common-lisp", "native-sexpr-structural", "r27-p9-v1"
	case AnalyzerClojure:
		return "clojure", "native-sexpr-structural", "r27-p9-v1"
	case AnalyzerEmacsLisp:
		return "emacs-lisp", "native-sexpr-structural", "r27-p9-v1"
	case AnalyzerSQL, AnalyzerPLSQL:
		return "sql-dialect", "native-line-structural", "r27-p10-v1"
	case AnalyzerGraphQL, AnalyzerProto:
		return "schema-block", "native-block-structural", "r27-p10-v1"
	case AnalyzerTerraform:
		return "hcl", "native-block-structural", "r27-p10-v1"
	case AnalyzerNix:
		return "nix", "native-line-structural", "r27-p10-v1"
	case AnalyzerVHDL:
		return "vhdl", "native-line-structural", "r27-p10-v1"
	case AnalyzerVerilog, AnalyzerSystemVerilog:
		return "hdl", "native-block-structural", "r27-p10-v1"
	case AnalyzerAssembly:
		return "assembly", "native-line-structural", "r27-p10-v1"
	case AnalyzerHTML, AnalyzerXML:
		return "markup", "native-document-structural", "r27-p10-v1"
	case AnalyzerCSS, AnalyzerSCSS, AnalyzerSass, AnalyzerLess:
		return "style", "native-selector-structural", "r27-p10-v1"
	case AnalyzerJSON:
		return "json", "native-document-structural", "r27-p10-v1"
	case AnalyzerYAML, AnalyzerTOML, AnalyzerMarkdown, AnalyzerOpenAPI, AnalyzerAnsibleYAML:
		return "document-line", "native-line-structural", "r27-p10-v1"
	case AnalyzerVue, AnalyzerSvelte, AnalyzerAstro, AnalyzerPHPHTML, AnalyzerJSP, AnalyzerJinja, AnalyzerTwig, AnalyzerBlade, AnalyzerEJS:
		return "composite-template", "native-masked-composite", "r27-p11-v1"
	default:
		return "unimplemented", "unimplemented", "none"
	}
}

func detectionEvidenceForDescriptor(descriptor LanguageDescriptor) []EvidenceKind {
	values := []EvidenceKind{EvidenceExplicit}
	if len(descriptor.ExactBasenames) > 0 {
		values = append(values, EvidenceExactBasename)
	}
	if len(descriptor.CompoundSuffixes) > 0 {
		values = append(values, EvidenceCompoundSuffix)
	}
	if len(descriptor.Extensions)+len(descriptor.AmbiguousExtensions) > 0 {
		values = append(values, EvidenceExtension)
	}
	if len(descriptor.ShebangInterpreters) > 0 {
		values = append(values, EvidenceShebang)
	}
	// Modelines can name every registered canonical ID/alias.
	values = append(values, EvidenceDirective)
	switch descriptor.ID {
	case "go", "csharp", "vbnet", "python", "classic-asp", "c", "cpp", "java", "kotlin", "php", "ruby", "swift", "delphi", "vbscript", "fsharp", "cil", "powershell", "freebasic", "purebasic", "aspnet-webforms", "razor", "blazor", "xaml", "mql4", "mql5", "objective-c", "objective-cpp", "dart", "d", "zig", "nim", "solidity", "apex", "al", "arduino", "perl", "luau", "elixir", "erlang", "groovy", "tcl", "autohotkey", "fortran", "cobol", "ada", "matlab", "octave", "julia", "r", "haskell", "ocaml", "common-lisp", "clojure", "emacs-lisp", "graphql", "proto", "terraform", "vhdl", "verilog", "systemverilog", "plsql", "openapi", "ansible-yaml", "php-html":
		values = append(values, EvidenceContentMarker)
	}
	values = append(values, EvidenceProjectHint)
	if descriptor.Capabilities.SourceAnalysis {
		values = append(values, EvidenceAnalyzerProbe)
	}
	return values
}

func validateLanguageCapabilityMetadata(descriptor LanguageDescriptor) error {
	if descriptor.Family == "" {
		return fmt.Errorf("language %s has no R27 family classification", descriptor.ID)
	}
	if len(descriptor.DetectionEvidence) == 0 || descriptor.ScannerProfile == "" || descriptor.CompositeBehavior == "" || descriptor.EncodingCoverage == "" || descriptor.AnalyzerStrategy == "" || descriptor.AnalyzerVersion == "" {
		return fmt.Errorf("language %s has incomplete R27 capability metadata", descriptor.ID)
	}
	seenEvidence := make(map[EvidenceKind]struct{}, len(descriptor.DetectionEvidence))
	for _, evidence := range descriptor.DetectionEvidence {
		if evidence == "" {
			return fmt.Errorf("language %s has empty detection evidence", descriptor.ID)
		}
		if _, duplicate := seenEvidence[evidence]; duplicate {
			return fmt.Errorf("language %s has duplicate detection evidence %q", descriptor.ID, evidence)
		}
		seenEvidence[evidence] = struct{}{}
	}
	capabilities := descriptor.Capabilities
	if capabilities.SourceAnalysis {
		if !capabilities.Declarations || !capabilities.Ranges || descriptor.AnalyzerStrategy == "unimplemented" || descriptor.ScannerProfile == "unimplemented" {
			return fmt.Errorf("language %s source analysis requires explicit declaration/range/provider capability metadata", descriptor.ID)
		}
	} else if capabilities.Declarations || capabilities.Hierarchy || capabilities.Signatures || capabilities.Ranges || capabilities.Dependencies || capabilities.InheritanceRelations || capabilities.SyntacticCalls || capabilities.ScopeResolvedReferences || capabilities.ProjectResolvedReferences || capabilities.ProjectResolvedDefinitions || capabilities.Implementations || capabilities.Overrides || capabilities.SemanticRelations || capabilities.IncrementalIndex {
		return fmt.Errorf("language %s advertises source capabilities without source analysis", descriptor.ID)
	}
	if capabilities.Composite && descriptor.CompositeBehavior == "none" {
		return fmt.Errorf("language %s composite capability requires composite behavior metadata", descriptor.ID)
	}
	if len(descriptor.KnownLimitations) == 0 {
		return fmt.Errorf("language %s must state known limitations explicitly", descriptor.ID)
	}
	return nil
}

func languageFamily(id string) string {
	families := map[string]string{
		"c": "mainstream-system", "cpp": "mainstream-system", "objective-c": "mainstream-system", "objective-cpp": "mainstream-system", "csharp": "dotnet", "java": "mainstream-system", "kotlin": "mainstream-system", "scala": "mainstream-system", "go": "mainstream-system", "rust": "mainstream-system", "swift": "mainstream-system", "dart": "mainstream-system", "d": "mainstream-system", "zig": "mainstream-system", "nim": "mainstream-system",
		"javascript": "web-dynamic", "typescript": "web-dynamic", "flow": "web-dynamic", "python": "web-dynamic", "php": "web-dynamic", "ruby": "web-dynamic", "perl": "web-dynamic", "lua": "web-dynamic", "luau": "web-dynamic", "elixir": "web-dynamic", "erlang": "web-dynamic", "gleam": "web-dynamic", "groovy": "web-dynamic",
		"vbnet": "dotnet", "fsharp": "dotnet", "cpp-cli": "dotnet", "jscript-net": "dotnet", "cil": "dotnet", "powershell": "dotnet", "xaml": "dotnet",
		"vb6": "basic", "vba": "basic", "vbscript": "basic", "qbasic": "basic", "classic-basic": "basic", "freebasic": "basic", "purebasic": "basic",
		"fortran": "scientific-legacy", "cobol": "scientific-legacy", "ada": "scientific-legacy", "pascal": "scientific-legacy", "delphi": "scientific-legacy", "matlab": "scientific-legacy", "octave": "scientific-legacy", "julia": "scientific-legacy", "r": "scientific-legacy",
		"haskell": "functional-lisp", "ocaml": "functional-lisp", "common-lisp": "functional-lisp", "clojure": "functional-lisp", "emacs-lisp": "functional-lisp",
		"shell": "shell-automation", "bash": "shell-automation", "tcl": "shell-automation", "autohotkey": "shell-automation",
		"mql4": "trading", "mql5": "trading",
		"assembly": "hardware-low-level", "vhdl": "hardware-low-level", "verilog": "hardware-low-level", "systemverilog": "hardware-low-level", "arduino": "hardware-low-level",
		"sql": "data-infra-dsl", "plsql": "data-infra-dsl", "graphql": "data-infra-dsl", "terraform": "data-infra-dsl", "nix": "data-infra-dsl", "proto": "data-infra-dsl", "solidity": "data-infra-dsl", "apex": "data-infra-dsl", "al": "data-infra-dsl",
		"html": "document-config", "xml": "document-config", "css": "document-config", "scss": "document-config", "sass": "document-config", "less": "document-config", "json": "document-config", "yaml": "document-config", "toml": "document-config", "markdown": "document-config", "openapi": "document-config", "ansible-yaml": "document-config",
		"classic-asp": "composite-template", "aspnet-webforms": "composite-template", "razor": "composite-template", "blazor": "composite-template", "vue": "composite-template", "svelte": "composite-template", "astro": "composite-template", "php-html": "composite-template", "jsp": "composite-template", "jinja": "composite-template", "twig": "composite-template", "blade": "composite-template", "ejs": "composite-template",
		"dockerfile": "build-config", "make": "build-config",
	}
	if family := families[id]; family != "" {
		return family
	}
	return "custom"
}
