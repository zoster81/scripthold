package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// Quoted ambient module specifiers are real namespace identities. Module
// references such as module.exports must not become fake module declarations.
func TestQuotedAmbientModuleOriginalSourceRegression(t *testing.T) {
	for _, language := range []string{"flow", "typescript"} {
		for _, quote := range []string{"'", "\""} {
			for _, eol := range []string{"\n", "\r\n"} {
				label := language + "-" + map[string]string{"'": "single", "\"": "double"}[quote] +
					map[string]string{"\n": "-lf", "\r\n": "-crlf"}[eol]
				t.Run(label, func(t *testing.T) {
					lines := []string{
						"// declare module 'decoy' { class Ghost {} }",
						"declare module " + quote + "eventemitter3" + quote + " {",
						"  declare type ListenerFn = (...args: any[]) => void;",
						"  declare class EventEmitter {",
						"    on(event: string, handler: ListenerFn): void;",
						"  }",
						"}",
						"const shared = module.exports;",
					}
					source := strings.Join(lines, eol) + eol
					document := testSourceDocument("ambient.js.flow", source)
					var analyzer SourceAnalyzer
					switch language {
					case "flow":
						analyzer = FlowAnalyzer{}
					default:
						analyzer = TypeScriptAnalyzer{}
					}
					result, err := analyzer.Analyze(context.Background(), document, testAnalyzeOptions(true, 64))
					if err != nil {
						t.Fatal(err)
					}
					if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
						t.Fatalf("bounded declared scope incomplete: %+v", result.Analysis)
					}
					want := map[string]int{
						"eventemitter3":                 2,
						"eventemitter3.ListenerFn":      3,
						"eventemitter3.EventEmitter":    4,
						"eventemitter3.EventEmitter.on": 5,
					}
					for _, sym := range result.Analysis.Symbols {
						if sym.Name == "Ghost" || sym.Name == "decoy" || sym.Name == "exports" ||
							(sym.NativeKind == "module" && sym.Name != "eventemitter3") {
							t.Errorf("comment or runtime reference fabricated as declaration: %+v", sym)
						}
						if line, ok := want[sym.QualifiedName]; ok {
							if sym.NameRange.Start.Line != line {
								t.Errorf("%s at line %d; expected %d", sym.QualifiedName, sym.NameRange.Start.Line, line)
							}
							delete(want, sym.QualifiedName)
						}
					}
					if len(want) != 0 {
						t.Fatalf("declared module/name/owner/method omitted: %v; got=%+v", want, result.Analysis.Symbols)
					}
				})
			}
		}
	}
}
