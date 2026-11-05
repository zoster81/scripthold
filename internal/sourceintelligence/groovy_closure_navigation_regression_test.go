package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// A class-scoped Groovy closure initializer is a field value, not a scope
// containing declared methods. Real script helper classes use this form.
func TestGroovyClosureFieldsDoNotConsumeFollowingMethods(t *testing.T) {
	const input = `class Helper {
  protected loadInterceptor = { args ->
    String name = args
    def normalized = Paths.get(name)
    return normalized
  }
  protected libraryResourceInterceptor = { Object args ->
    try {
      return IOUtils.toString(args)
    } finally {
      IOUtils.closeQuietly(args)
    }
  }
  void registerAllowedMethod(String name, Closure closure = null) {
    allowedMethodCallbacks.put(method(name), closure)
  }
  void registerAllowedMethod(String name, List<Class> args, Closure closure = null) {
    allowedMethodCallbacks.put(method(name, args.toArray(new Class[args.size()])), closure)
  }
}
`
	for _, eol := range []struct{ name, sep string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
		t.Run(eol.name, func(t *testing.T) {
			doc := sourceDocumentForScanner(strings.ReplaceAll(input, "\n", eol.sep))
			r, err := (GroovyAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 100))
			if err != nil {
				t.Fatal(err)
			}
			wantMethods := map[int]bool{14: false, 17: false}
			unwanted := map[string]bool{"get": true, "toString": true, "closeQuietly": true, "put": true}
			for _, sym := range r.Analysis.Symbols {
				if sym.Kind == SymbolKindMethod {
					if sym.Name == "registerAllowedMethod" {
						if _, ok := wantMethods[sym.NameRange.Start.Line]; ok {
							wantMethods[sym.NameRange.Start.Line] = true
						}
					}
					if unwanted[sym.Name] {
						t.Errorf("initializer call indexed as method %s at line %d", sym.Name, sym.NameRange.Start.Line)
					}
				}
			}
			for line, found := range wantMethods {
				if !found {
					t.Errorf("real registerAllowedMethod at line %d missing", line)
				}
			}
		})
	}
}
