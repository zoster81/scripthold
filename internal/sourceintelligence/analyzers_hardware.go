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
	vhdlEntity       = regexp.MustCompile(`(?im)^[ \t]*entity[ \t]+([A-Za-z_][A-Za-z0-9_]*)[ \t]+is\b`)
	vhdlArchitecture = regexp.MustCompile(`(?im)^[ \t]*architecture[ \t]+([A-Za-z_][A-Za-z0-9_]*)[ \t]+of[ \t]+([A-Za-z_][A-Za-z0-9_]*)[ \t]+is\b`)
	vhdlSignal       = regexp.MustCompile(`(?im)^[ \t]*signal[ \t]+([A-Za-z_][A-Za-z0-9_]*)[ \t]*:`)
	vhdlUse          = regexp.MustCompile(`(?im)^[ \t]*use[ \t]+([A-Za-z_][A-Za-z0-9_.]*)[ \t]*;`)
)

func (VHDLAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newStructuralAnalyzerBuilder(ctx, document, options, "vhdl", AnalyzerVHDL)
	if err != nil {
		return AnalyzerResult{}, err
	}
	dependencies := []StructuralDependency{}
	entities := map[string]SymbolParent{}
	source := maskSourceComments(document.Text, []string{"--"}, "", "")
	for _, use := range vhdlUse.FindAllStringSubmatchIndex(source, -1) {
		value := document.Text[use[2]:use[3]]
		addStructuralDependency(document, &dependencies, StructuralDependencyImport, value, use[2], use[3])
	}
	for _, match := range vhdlEntity.FindAllStringSubmatchIndex(source, -1) {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		name := document.Text[match[2]:match[3]]
		end := vhdlEnd(source, match[1], "entity", name)
		if end < 0 {
			addDocumentDataHardwareDiagnostic(builder, "vhdl-unclosed-entity", "VHDL entity is not structurally closed", match[0], match[1])
			end = match[1]
		}
		symbol, ok := addDocumentDataHardwareSymbol(builder, SymbolKindEntity, "entity", name, nil, OffsetRange{Start: match[0], End: end}, OffsetRange{Start: match[2], End: match[3]})
		if ok {
			entities[strings.ToLower(name)] = SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
		}
	}
	for _, match := range vhdlArchitecture.FindAllStringSubmatchIndex(source, -1) {
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
		end := vhdlEnd(source, match[1], "architecture", name)
		if end < 0 {
			addDocumentDataHardwareDiagnostic(builder, "vhdl-unclosed-architecture", "VHDL architecture is not structurally closed", match[0], match[1])
			end = len(document.Text)
		}
		symbol, ok := addDocumentDataHardwareSymbol(builder, SymbolKindImplementation, "architecture", name, parent, OffsetRange{Start: match[0], End: end}, OffsetRange{Start: match[2], End: match[3]})
		if !ok {
			continue
		}
		archParent := parentFromNormalizedSymbol(symbol)
		bodyEnd := end
		for _, signal := range vhdlSignal.FindAllStringSubmatchIndex(source[match[1]:bodyEnd], -1) {
			value := document.Text[match[1]+signal[2] : match[1]+signal[3]]
			addDocumentDataHardwareSymbol(builder, SymbolKindSignal, "signal", value, archParent,
				OffsetRange{Start: match[1] + signal[0], End: match[1] + signal[1]},
				OffsetRange{Start: match[1] + signal[2], End: match[1] + signal[3]})
		}
	}
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: dependencies}, nil
}

func vhdlEnd(text string, start int, kind, name string) int {
	pattern := regexp.MustCompile(`(?im)^[ \t]*end(?:[ \t]+` + regexp.QuoteMeta(kind) + `)?(?:[ \t]+` + regexp.QuoteMeta(name) + `)?[ \t]*;`)
	if match := pattern.FindStringIndex(text[start:]); match != nil {
		return start + match[1]
	}
	return -1
}

var (
	verilogModule   = regexp.MustCompile(`(?im)^[ \t]*module[ \t]+([A-Za-z_][A-Za-z0-9_$]*)\b`)
	svPackage       = regexp.MustCompile(`(?im)^[ \t]*package[ \t]+([A-Za-z_][A-Za-z0-9_$]*)[ \t]*;`)
	svInterface     = regexp.MustCompile(`(?im)^[ \t]*interface[ \t]+([A-Za-z_][A-Za-z0-9_$]*)\b`)
	hdlSignal       = regexp.MustCompile(`(?i)\b(?:wire|reg|logic|bit)[ \t]+(?:signed[ \t]+|unsigned[ \t]+)?(?:\[[^\]\r\n]+\][ \t]+)?([A-Za-z_][A-Za-z0-9_$]*)\b`)
	svTypedefStruct = regexp.MustCompile(`(?is)typedef[ \t]+struct(?:[ \t]+packed)?[ \t]*\{.*?\}[ \t]*([A-Za-z_][A-Za-z0-9_$]*)[ \t]*;`)
)

func (VerilogAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeHDLSource(ctx, document, options, false)
}

func (SystemVerilogAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeHDLSource(ctx, document, options, true)
}

func analyzeHDLSource(ctx context.Context, document *SourceDocument, options AnalyzeOptions, systemVerilog bool) (AnalyzerResult, error) {
	language := "verilog"
	analyzer := AnalyzerVerilog
	if systemVerilog {
		language = "systemverilog"
		analyzer = AnalyzerSystemVerilog
	}
	builder, err := newStructuralAnalyzerBuilder(ctx, document, options, language, analyzer)
	if err != nil {
		return AnalyzerResult{}, err
	}
	source := maskSourceComments(document.Text, []string{"//"}, "/*", "*/")
	source = maskSourceStrings(source, false, true, false)
	if systemVerilog {
		for _, match := range svPackage.FindAllStringSubmatchIndex(source, -1) {
			name := document.Text[match[2]:match[3]]
			end := hdlEnd(source, match[1], "endpackage")
			if end < 0 {
				addDocumentDataHardwareDiagnostic(builder, "systemverilog-unclosed-package", "SystemVerilog package is not structurally closed", match[0], match[1])
				continue
			}
			symbol, ok := addDocumentDataHardwareSymbol(builder, SymbolKindPackage, "package", name, nil, OffsetRange{Start: match[0], End: end}, OffsetRange{Start: match[2], End: match[3]})
			if ok {
				parent := parentFromNormalizedSymbol(symbol)
				body := source[match[1]:end]
				for _, typeMatch := range svTypedefStruct.FindAllStringSubmatchIndex(body, -1) {
					value := document.Text[match[1]+typeMatch[2] : match[1]+typeMatch[3]]
					addDocumentDataHardwareSymbol(builder, SymbolKindType, "typedef-struct", value, parent, OffsetRange{Start: match[1] + typeMatch[0], End: match[1] + typeMatch[1]}, OffsetRange{Start: match[1] + typeMatch[2], End: match[1] + typeMatch[3]})
				}
			}
		}
		for _, match := range svInterface.FindAllStringSubmatchIndex(source, -1) {
			name := document.Text[match[2]:match[3]]
			end := hdlEnd(source, match[1], "endinterface")
			if end < 0 {
				addDocumentDataHardwareDiagnostic(builder, "systemverilog-unclosed-interface", "SystemVerilog interface is not structurally closed", match[0], match[1])
				continue
			}
			symbol, ok := addDocumentDataHardwareSymbol(builder, SymbolKindInterface, "interface", name, nil, OffsetRange{Start: match[0], End: end}, OffsetRange{Start: match[2], End: match[3]})
			if ok {
				hdlSignals(builder, document, source, match[1], end, parentFromNormalizedSymbol(symbol))
			}
		}
	}
	for _, match := range verilogModule.FindAllStringSubmatchIndex(source, -1) {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		name := document.Text[match[2]:match[3]]
		end := hdlEnd(source, match[1], "endmodule")
		if end < 0 {
			addDocumentDataHardwareDiagnostic(builder, language+"-unclosed-module", language+" module is not structurally closed", match[0], match[1])
			continue
		}
		symbol, ok := addDocumentDataHardwareSymbol(builder, SymbolKindModule, "module", name, nil, OffsetRange{Start: match[0], End: end}, OffsetRange{Start: match[2], End: match[3]})
		if ok {
			hdlSignals(builder, document, source, match[1], end, parentFromNormalizedSymbol(symbol))
		}
	}
	return AnalyzerResult{Analysis: builder.Result()}, nil
}

func hdlEnd(text string, start int, terminator string) int {
	pattern := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(terminator) + `\b`)
	for _, line := range sourceTextLines(text[start:]) {
		code := stripLineComment(line.text, "//")
		if match := pattern.FindStringIndex(code); match != nil {
			return start + line.start + match[1]
		}
	}
	return -1
}

func hdlSignals(builder *SymbolBuilder, document *SourceDocument, source string, start, end int, parent *SymbolParent) {
	if end <= start || end > len(document.Text) || len(source) != len(document.Text) {
		return
	}
	body := source[start:end]
	for _, match := range hdlSignal.FindAllStringSubmatchIndex(body, -1) {
		name := document.Text[start+match[2] : start+match[3]]
		addDocumentDataHardwareSymbol(builder, SymbolKindSignal, "signal", name, parent,
			OffsetRange{Start: start + match[0], End: start + match[1]},
			OffsetRange{Start: start + match[2], End: start + match[3]})
	}
}

var (
	assemblyLabel    = regexp.MustCompile(`(?m)^[ \t]*([0-9]+|[A-Za-z_.$][A-Za-z0-9_.$@]*):`)
	assemblyMASMProc = regexp.MustCompile(`(?im)^[ \t]*([A-Za-z_.$][A-Za-z0-9_.$@]*)[ \t]+PROC\b`)
)

func (AssemblyAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newStructuralAnalyzerBuilder(ctx, document, options, "assembly", AnalyzerAssembly)
	if err != nil {
		return AnalyzerResult{}, err
	}
	for _, match := range assemblyLabel.FindAllStringSubmatchIndex(document.Text, -1) {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		name := document.Text[match[2]:match[3]]
		addDocumentDataHardwareSymbol(builder, SymbolKindLabel, "label", name, nil, OffsetRange{Start: match[0], End: match[1]}, OffsetRange{Start: match[2], End: match[3]})
	}
	for _, match := range assemblyMASMProc.FindAllStringSubmatchIndex(document.Text, -1) {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		name := document.Text[match[2]:match[3]]
		addDocumentDataHardwareSymbol(builder, SymbolKindLabel, "masm-proc", name, nil, OffsetRange{Start: match[0], End: match[1]}, OffsetRange{Start: match[2], End: match[3]})
	}
	return AnalyzerResult{Analysis: builder.Result()}, nil
}
