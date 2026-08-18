package sourceintelligence

import (
	"context"
	"regexp"
	"strings"
)

type VHDLAnalyzer struct{}
type VerilogAnalyzer struct{}
type SystemVerilogAnalyzer struct{}
type AssemblyAnalyzer struct{}

func (VHDLAnalyzer) ID() AnalyzerID            { return AnalyzerVHDL }
func (VHDLAnalyzer) Language() string          { return "vhdl" }
func (VerilogAnalyzer) ID() AnalyzerID         { return AnalyzerVerilog }
func (VerilogAnalyzer) Language() string       { return "verilog" }
func (SystemVerilogAnalyzer) ID() AnalyzerID   { return AnalyzerSystemVerilog }
func (SystemVerilogAnalyzer) Language() string { return "systemverilog" }
func (AssemblyAnalyzer) ID() AnalyzerID        { return AnalyzerAssembly }
func (AssemblyAnalyzer) Language() string      { return "assembly" }

var (
	phase10VHDLEntity       = regexp.MustCompile(`(?im)^[ \t]*entity[ \t]+([A-Za-z_][A-Za-z0-9_]*)[ \t]+is\b`)
	phase10VHDLArchitecture = regexp.MustCompile(`(?im)^[ \t]*architecture[ \t]+([A-Za-z_][A-Za-z0-9_]*)[ \t]+of[ \t]+([A-Za-z_][A-Za-z0-9_]*)[ \t]+is\b`)
	phase10VHDLSignal       = regexp.MustCompile(`(?im)^[ \t]*signal[ \t]+([A-Za-z_][A-Za-z0-9_]*)[ \t]*:`)
	phase10VHDLUse          = regexp.MustCompile(`(?im)^[ \t]*use[ \t]+([A-Za-z_][A-Za-z0-9_.]*)[ \t]*;`)
)

func (VHDLAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newPhase10Builder(ctx, document, options, "vhdl", AnalyzerVHDL)
	if err != nil {
		return AnalyzerResult{}, err
	}
	dependencies := []StructuralDependency{}
	entities := map[string]SymbolParent{}
	source := phase10MaskComments(document.Text, []string{"--"}, "", "")
	for _, use := range phase10VHDLUse.FindAllStringSubmatchIndex(source, -1) {
		value := document.Text[use[2]:use[3]]
		phase10AddDependency(document, &dependencies, StructuralDependencyImport, value, use[2], use[3])
	}
	for _, match := range phase10VHDLEntity.FindAllStringSubmatchIndex(source, -1) {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		name := document.Text[match[2]:match[3]]
		end := phase10VHDLEnd(source, match[1], "entity", name)
		if end < 0 {
			phase10Diagnostic(builder, "vhdl-unclosed-entity", "VHDL entity is not structurally closed", match[0], match[1])
			end = match[1]
		}
		symbol, ok := phase10AddSymbol(builder, SymbolKindEntity, "entity", name, nil, OffsetRange{Start: match[0], End: end}, OffsetRange{Start: match[2], End: match[3]})
		if ok {
			entities[strings.ToLower(name)] = SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
		}
	}
	for _, match := range phase10VHDLArchitecture.FindAllStringSubmatchIndex(source, -1) {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		name := document.Text[match[2]:match[3]]
		owner := document.Text[match[4]:match[5]]
		var parent *SymbolParent
		if value, ok := entities[strings.ToLower(owner)]; ok {
			v := value
			parent = &v
		}
		end := phase10VHDLEnd(source, match[1], "architecture", name)
		if end < 0 {
			phase10Diagnostic(builder, "vhdl-unclosed-architecture", "VHDL architecture is not structurally closed", match[0], match[1])
			end = len(document.Text)
		}
		symbol, ok := phase10AddSymbol(builder, SymbolKindImplementation, "architecture", name, parent, OffsetRange{Start: match[0], End: end}, OffsetRange{Start: match[2], End: match[3]})
		if !ok {
			continue
		}
		archParent := phase10Parent(symbol)
		bodyEnd := end
		for _, signal := range phase10VHDLSignal.FindAllStringSubmatchIndex(source[match[1]:bodyEnd], -1) {
			value := document.Text[match[1]+signal[2] : match[1]+signal[3]]
			phase10AddSymbol(builder, SymbolKindSignal, "signal", value, archParent,
				OffsetRange{Start: match[1] + signal[0], End: match[1] + signal[1]},
				OffsetRange{Start: match[1] + signal[2], End: match[1] + signal[3]})
		}
	}
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: dependencies}, nil
}

func phase10VHDLEnd(text string, start int, kind, name string) int {
	pattern := regexp.MustCompile(`(?im)^[ \t]*end(?:[ \t]+` + regexp.QuoteMeta(kind) + `)?(?:[ \t]+` + regexp.QuoteMeta(name) + `)?[ \t]*;`)
	if match := pattern.FindStringIndex(text[start:]); match != nil {
		return start + match[1]
	}
	return -1
}

var (
	phase10VerilogModule   = regexp.MustCompile(`(?im)^[ \t]*module[ \t]+([A-Za-z_][A-Za-z0-9_$]*)\b`)
	phase10SVPackage       = regexp.MustCompile(`(?im)^[ \t]*package[ \t]+([A-Za-z_][A-Za-z0-9_$]*)[ \t]*;`)
	phase10SVInterface     = regexp.MustCompile(`(?im)^[ \t]*interface[ \t]+([A-Za-z_][A-Za-z0-9_$]*)\b`)
	phase10HDLSignal       = regexp.MustCompile(`(?i)\b(?:wire|reg|logic|bit)[ \t]+(?:signed[ \t]+|unsigned[ \t]+)?(?:\[[^\]\r\n]+\][ \t]+)?([A-Za-z_][A-Za-z0-9_$]*)\b`)
	phase10SVTypedefStruct = regexp.MustCompile(`(?is)typedef[ \t]+struct(?:[ \t]+packed)?[ \t]*\{.*?\}[ \t]*([A-Za-z_][A-Za-z0-9_$]*)[ \t]*;`)
)

func (VerilogAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzePhase10HDL(ctx, document, options, false)
}

func (SystemVerilogAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzePhase10HDL(ctx, document, options, true)
}

func analyzePhase10HDL(ctx context.Context, document *SourceDocument, options AnalyzeOptions, systemVerilog bool) (AnalyzerResult, error) {
	language := "verilog"
	analyzer := AnalyzerVerilog
	if systemVerilog {
		language = "systemverilog"
		analyzer = AnalyzerSystemVerilog
	}
	builder, err := newPhase10Builder(ctx, document, options, language, analyzer)
	if err != nil {
		return AnalyzerResult{}, err
	}
	source := phase10MaskComments(document.Text, []string{"//"}, "/*", "*/")
	source = phase10MaskStrings(source, false, true, false)
	if systemVerilog {
		for _, match := range phase10SVPackage.FindAllStringSubmatchIndex(source, -1) {
			name := document.Text[match[2]:match[3]]
			end := phase10HDLEnd(source, match[1], "endpackage")
			if end < 0 {
				phase10Diagnostic(builder, "systemverilog-unclosed-package", "SystemVerilog package is not structurally closed", match[0], match[1])
				continue
			}
			symbol, ok := phase10AddSymbol(builder, SymbolKindPackage, "package", name, nil, OffsetRange{Start: match[0], End: end}, OffsetRange{Start: match[2], End: match[3]})
			if ok {
				parent := phase10Parent(symbol)
				body := source[match[1]:end]
				for _, typeMatch := range phase10SVTypedefStruct.FindAllStringSubmatchIndex(body, -1) {
					value := document.Text[match[1]+typeMatch[2] : match[1]+typeMatch[3]]
					phase10AddSymbol(builder, SymbolKindType, "typedef-struct", value, parent, OffsetRange{Start: match[1] + typeMatch[0], End: match[1] + typeMatch[1]}, OffsetRange{Start: match[1] + typeMatch[2], End: match[1] + typeMatch[3]})
				}
			}
		}
		for _, match := range phase10SVInterface.FindAllStringSubmatchIndex(source, -1) {
			name := document.Text[match[2]:match[3]]
			end := phase10HDLEnd(source, match[1], "endinterface")
			if end < 0 {
				phase10Diagnostic(builder, "systemverilog-unclosed-interface", "SystemVerilog interface is not structurally closed", match[0], match[1])
				continue
			}
			symbol, ok := phase10AddSymbol(builder, SymbolKindInterface, "interface", name, nil, OffsetRange{Start: match[0], End: end}, OffsetRange{Start: match[2], End: match[3]})
			if ok {
				phase10HDLSignals(builder, document, source, match[1], end, phase10Parent(symbol))
			}
		}
	}
	for _, match := range phase10VerilogModule.FindAllStringSubmatchIndex(source, -1) {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		name := document.Text[match[2]:match[3]]
		end := phase10HDLEnd(source, match[1], "endmodule")
		if end < 0 {
			phase10Diagnostic(builder, language+"-unclosed-module", language+" module is not structurally closed", match[0], match[1])
			continue
		}
		symbol, ok := phase10AddSymbol(builder, SymbolKindModule, "module", name, nil, OffsetRange{Start: match[0], End: end}, OffsetRange{Start: match[2], End: match[3]})
		if ok {
			phase10HDLSignals(builder, document, source, match[1], end, phase10Parent(symbol))
		}
	}
	return AnalyzerResult{Analysis: builder.Result()}, nil
}

func phase10HDLEnd(text string, start int, terminator string) int {
	pattern := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(terminator) + `\b`)
	for _, line := range phase10Lines(text[start:]) {
		code := phase10StripLineComment(line.text, "//")
		if match := pattern.FindStringIndex(code); match != nil {
			return start + line.start + match[1]
		}
	}
	return -1
}

func phase10HDLSignals(builder *SymbolBuilder, document *SourceDocument, source string, start, end int, parent *SymbolParent) {
	if end <= start || end > len(document.Text) || len(source) != len(document.Text) {
		return
	}
	body := source[start:end]
	for _, match := range phase10HDLSignal.FindAllStringSubmatchIndex(body, -1) {
		name := document.Text[start+match[2] : start+match[3]]
		phase10AddSymbol(builder, SymbolKindSignal, "signal", name, parent,
			OffsetRange{Start: start + match[0], End: start + match[1]},
			OffsetRange{Start: start + match[2], End: start + match[3]})
	}
}

var (
	phase10AssemblyLabel    = regexp.MustCompile(`(?m)^[ \t]*([0-9]+|[A-Za-z_.$][A-Za-z0-9_.$@]*):`)
	phase10AssemblyMASMProc = regexp.MustCompile(`(?im)^[ \t]*([A-Za-z_.$][A-Za-z0-9_.$@]*)[ \t]+PROC\b`)
)

func (AssemblyAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newPhase10Builder(ctx, document, options, "assembly", AnalyzerAssembly)
	if err != nil {
		return AnalyzerResult{}, err
	}
	for _, match := range phase10AssemblyLabel.FindAllStringSubmatchIndex(document.Text, -1) {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		name := document.Text[match[2]:match[3]]
		phase10AddSymbol(builder, SymbolKindLabel, "label", name, nil, OffsetRange{Start: match[0], End: match[1]}, OffsetRange{Start: match[2], End: match[3]})
	}
	for _, match := range phase10AssemblyMASMProc.FindAllStringSubmatchIndex(document.Text, -1) {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		name := document.Text[match[2]:match[3]]
		phase10AddSymbol(builder, SymbolKindLabel, "masm-proc", name, nil, OffsetRange{Start: match[0], End: match[1]}, OffsetRange{Start: match[2], End: match[3]})
	}
	return AnalyzerResult{Analysis: builder.Result()}, nil
}
