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
	vhdlEntity         = regexp.MustCompile(`(?im)^[ \t]*entity[ \t]+([A-Za-z_][A-Za-z0-9_]*)[ \t]+is\b`)
	vhdlArchitecture   = regexp.MustCompile(`(?im)^[ \t]*architecture[ \t]+([A-Za-z_][A-Za-z0-9_]*)[ \t]+of[ \t]+([A-Za-z_][A-Za-z0-9_]*)[ \t]+is\b`)
	vhdlSignal         = regexp.MustCompile(`(?im)^[ \t]*signal[ \t]+([A-Za-z_][A-Za-z0-9_]*)[ \t]*:`)
	vhdlSignalBoundary = regexp.MustCompile(`(?i)^[ \t]*(?:begin|end|signal|architecture|entity|component|process|function|procedure|type|constant|variable)\b`)
	vhdlRangeType      = regexp.MustCompile(`(?im)^[ \t]*type[ \t]+([A-Za-z_][A-Za-z0-9_]*)[ \t]+is[ \t]+range[ \t]+[^;\r\n]+;`)
	vhdlBegin          = regexp.MustCompile(`(?im)^[ \t]*begin\b`)
	vhdlSubprogram     = regexp.MustCompile(`(?im)^[ \t]*(?:(?:pure|impure)[ \t]+)?(?:function|procedure)[ \t]+[A-Za-z_]`)
	vhdlComponent      = regexp.MustCompile(`(?im)^[ \t]*component[ \t]+([A-Za-z_][A-Za-z0-9_]*)\b`)
	vhdlUse            = regexp.MustCompile(`(?im)^[ \t]*use[ \t]+([A-Za-z_][A-Za-z0-9_.]*)[ \t]*;`)
	vhdlGenericOpen    = regexp.MustCompile(`(?im)^[ \t]*generic[ \t\r\n]*\(`)
	vhdlGenericWord    = regexp.MustCompile(`(?im)^[ \t]*generic\b`)
	vhdlGenericDecl    = regexp.MustCompile(`(?is)^\s*(?:constant\s+)?([A-Za-z_][A-Za-z0-9_]*(?:\s*,\s*[A-Za-z_][A-Za-z0-9_]*)*)\s*:\s*(?:in\s+)?[A-Za-z_][A-Za-z0-9_]*\b`)
	vhdlGenericIdent   = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)
)

func (VHDLAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newStructuralAnalyzerBuilder(ctx, document, options, "vhdl", AnalyzerVHDL)
	if err != nil {
		return AnalyzerResult{}, err
	}
	dependencies := structuralDependencyAccumulator{items: []StructuralDependency{}}
	entities := map[string]SymbolParent{}
	source := maskSourceComments(document.Text, []string{"--"}, "", "")
	for _, use := range vhdlUse.FindAllStringSubmatchIndex(source, -1) {
		value := document.Text[use[2]:use[3]]
		dependencies.add(document, StructuralDependencyImport, value, use[2], use[3])
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
			if end > match[1] {
				vhdlEntityGenerics(builder, document, source, match[1], end, &symbol)
			}
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
		archParent := &symbol
		bodyEnd := end
		vhdlArchitectureRangeTypes(builder, document, source, match[1], bodyEnd, archParent)
		for _, component := range vhdlComponent.FindAllStringSubmatchIndex(source[match[1]:bodyEnd], -1) {
			if err := ctx.Err(); err != nil {
				return AnalyzerResult{}, err
			}
			componentStart := match[1] + component[0]
			nameStart := match[1] + component[2]
			nameEnd := match[1] + component[3]
			name := document.Text[nameStart:nameEnd]
			componentEnd := vhdlEnd(source, match[1]+component[1], "component", name)
			if componentEnd < 0 || componentEnd > bodyEnd {
				addDocumentDataHardwareDiagnostic(builder, "vhdl-unclosed-component", "VHDL component declaration is not structurally closed", componentStart, nameEnd)
				continue
			}
			addDocumentDataHardwareSymbol(builder, SymbolKindType, "component", name, archParent,
				OffsetRange{Start: componentStart, End: componentEnd}, OffsetRange{Start: nameStart, End: nameEnd})
		}
		for _, signal := range vhdlSignal.FindAllStringSubmatchIndex(source[match[1]:bodyEnd], -1) {
			value := document.Text[match[1]+signal[2] : match[1]+signal[3]]
			declarationEnd, complete := vhdlSignalDeclarationEnd(source, match[1]+signal[1], bodyEnd)
			if !complete {
				addDocumentDataHardwareDiagnostic(builder, "vhdl-unclosed-signal",
					"VHDL signal declaration has no bounded, unambiguous terminating semicolon",
					match[1]+signal[0], match[1]+signal[1])
				declarationEnd = match[1] + signal[1] // Preserve the known prefix only.
			}
			addDocumentDataHardwareSymbol(builder, SymbolKindSignal, "signal", value, archParent,
				OffsetRange{Start: match[1] + signal[0], End: declarationEnd},
				OffsetRange{Start: match[1] + signal[2], End: match[1] + signal[3]})
		}
	}
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: dependencies.items}, nil
}

// vhdlSignalDeclarationEnd finds the authored semicolon of a VHDL signal
// declaration. Source comments have already been masked without changing
// offsets. Quoted strings and character literals may themselves contain ';'.
// Unbounded or structurally ambiguous input keeps only the recognized prefix.
func vhdlSignalDeclarationEnd(source string, afterColon, scopeEnd int) (int, bool) {
	const maxBytes = 64 << 10
	const maxLines = 1024
	if afterColon < 0 || afterColon >= scopeEnd || scopeEnd > len(source) {
		return 0, false
	}
	limit := min(scopeEnd, afterColon+maxBytes)
	quoted, lines := false, 0
	for i := afterColon; i < limit; i++ {
		switch source[i] {
		case '"':
			if quoted && i+1 < limit && source[i+1] == '"' {
				i++ // VHDL doubled quote inside a string.
			} else {
				quoted = !quoted
			}
		case '\'':
			if !quoted && i+2 < limit && source[i+2] == '\'' {
				i += 2 // A single-character literal, including ';'.
			}
		case ';':
			if !quoted {
				return i + 1, true
			}
		case '\n':
			if quoted {
				return 0, false // Multiline string content is not VHDL source.
			}
			lines++
			if lines > maxLines {
				return 0, false
			}
			next := i + 1
			if next < limit && vhdlSignalBoundary.MatchString(source[next:min(limit, next+256)]) {
				return 0, false // Do not absorb a following declaration or begin.
			}
		}
	}
	return 0, false
}

// vhdlArchitectureRangeTypes recognizes simple, complete single-line range
// type declarations in an architecture's declarative prefix. Stop at the
// first begin or nested subprogram: a process/function-local type cannot
// truthfully be assigned to the architecture by a flat text matcher.
func vhdlArchitectureRangeTypes(builder *SymbolBuilder, document *SourceDocument, source string, start, end int, parent *SymbolParent) {
	if start >= end || end > len(source) {
		return
	}
	body := source[start:end]
	begin := vhdlBegin.FindStringIndex(body)
	if begin == nil {
		return
	}
	declarationEnd := begin[0]
	if nested := vhdlSubprogram.FindStringIndex(body[:declarationEnd]); nested != nil {
		declarationEnd = nested[0]
	}
	for _, match := range vhdlRangeType.FindAllStringSubmatchIndex(body[:declarationEnd], -1) {
		nameStart, nameEnd := start+match[2], start+match[3]
		addDocumentDataHardwareSymbol(builder, SymbolKindType, "range-type",
			document.Text[nameStart:nameEnd], parent,
			OffsetRange{Start: start + match[0], End: start + match[1]},
			OffsetRange{Start: nameStart, End: nameEnd})
	}
}

// vhdlEntityGenerics indexes explicit scalar interface constants only inside
// an entity declaration. Generic maps and component generics have different
// owners and must not be attributed to this entity.
func vhdlEntityGenerics(builder *SymbolBuilder, doc *SourceDocument, source string, start, end int, parent *SymbolParent) {
	header := source[start:end]
	open := vhdlGenericOpen.FindStringIndex(header)
	if open == nil {
		if vhdlGenericWord.MatchString(header) {
			addDocumentDataHardwareDiagnostic(builder, "vhdl-unrecognized-generic", "Unrecognized VHDL generic clause", start, end)
		}
		return
	}
	at := start + open[1] - 1 // opening parenthesis
	segmentStart, depth := at+1, 0
	addSegment := func(stop int) {
		segment := source[segmentStart:stop]
		if strings.TrimSpace(segment) == "" {
			return
		}
		decl := vhdlGenericDecl.FindStringSubmatchIndex(segment)
		if decl == nil {
			addDocumentDataHardwareDiagnostic(builder, "vhdl-unrecognized-generic", "VHDL generic declaration is not supported", segmentStart, stop)
			return
		}
		// A conservative subtype prefix must be followed by a constrained
		// subtype, range, initialization or the end of this declaration.
		tail := strings.TrimSpace(segment[decl[1]:])
		if tail != "" && !strings.HasPrefix(tail, "(") && !strings.HasPrefix(tail, ":=") &&
			!strings.HasPrefix(strings.ToLower(tail), "range ") {
			addDocumentDataHardwareDiagnostic(builder, "vhdl-unrecognized-generic", "VHDL generic subtype is not supported", segmentStart, stop)
			return
		}
		nameList := segment[decl[2]:decl[3]]
		for _, match := range vhdlGenericIdent.FindAllStringIndex(nameList, -1) {
			nameStart := segmentStart + decl[2] + match[0]
			nameEnd := segmentStart + decl[2] + match[1]
			addDocumentDataHardwareSymbol(builder, SymbolKindConstant, "generic", doc.Text[nameStart:nameEnd], parent,
				OffsetRange{Start: nameStart, End: stop},
				OffsetRange{Start: nameStart, End: nameEnd})
		}
	}
	for cursor := at; cursor < end; cursor++ {
		switch source[cursor] {
		case '"':
			for cursor++; cursor < end; cursor++ {
				if source[cursor] == '"' {
					if cursor+1 < end && source[cursor+1] == '"' {
						cursor++
						continue
					}
					break
				}
			}
		case '\'':
			if cursor+2 < end && source[cursor+2] == '\'' {
				cursor += 2
			}
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				addSegment(cursor)
				if !strings.HasPrefix(strings.TrimSpace(source[cursor+1:end]), ";") {
					addDocumentDataHardwareDiagnostic(builder, "vhdl-unrecognized-generic", "VHDL generic clause must end in a semicolon", at, cursor+1)
				}
				return
			}
		case ';':
			if depth == 1 {
				addSegment(cursor)
				segmentStart = cursor + 1
			}
		}
	}
	addDocumentDataHardwareDiagnostic(builder, "vhdl-unclosed-generic", "VHDL generic clause is not closed", at, end)
}

func vhdlEnd(text string, start int, kind, name string) int {
	if kind == "architecture" {
		// Prefer a named architecture end over a bare end for nested functions.
		// Never borrow a terminator from a subsequent design unit.
		limit := len(text)
		nextUnit := regexp.MustCompile(`(?im)^[ \t]*(?:entity|architecture|package|configuration)[ \t]+[A-Za-z_][A-Za-z0-9_]*\b`)
		if match := nextUnit.FindStringIndex(text[start:]); match != nil {
			limit = start + match[0]
		}
		body := text[start:limit]
		named := regexp.MustCompile(`(?im)^[ \t]*end(?:[ \t]+architecture(?:[ \t]+` + regexp.QuoteMeta(name) + `)?|[ \t]+` + regexp.QuoteMeta(name) + `)[ \t]*;`)
		if match := named.FindStringIndex(body); match != nil {
			return start + match[1]
		}
		// Without an explicit architecture name, the final bare end is the
		// outer closure rather than an earlier subprogram's bare end.
		bare := regexp.MustCompile(`(?im)^[ \t]*end[ \t]*;`)
		lastEnd := -1
		for cursor := 0; cursor < len(body); {
			match := bare.FindStringIndex(body[cursor:])
			if match == nil {
				break
			}
			lastEnd = cursor + match[1]
			cursor = lastEnd
		}
		if lastEnd >= 0 && strings.TrimSpace(body[lastEnd:]) == "" &&
			vhdlArchitectureBareEndProven(body, lastEnd) {
			return start + lastEnd
		}
		return -1
	}
	pattern := regexp.MustCompile(`(?im)^[ \t]*end(?:[ \t]+` + regexp.QuoteMeta(kind) + `)?(?:[ \t]+` + regexp.QuoteMeta(name) + `)?[ \t]*;`)
	if match := pattern.FindStringIndex(text[start:]); match != nil {
		return start + match[1]
	}
	return -1
}

// A bare end following a nested subprogram is not proof of an outer
// architecture closure until a separate architecture begin follows the
// subprogram's closing end. Unrecognized forms remain incomplete.
func vhdlArchitectureBareEndProven(body string, lastEnd int) bool {
	begin := regexp.MustCompile(`(?im)^[ \t]*begin\b`)
	subprogram := regexp.MustCompile(`(?im)^[ \t]*(?:(?:pure|impure)[ \t]+)?(?:function|procedure)[ \t]+([A-Za-z_][A-Za-z0-9_]*)\b`)
	isKeyword := regexp.MustCompile(`(?i)\bis\b`)
	// Scan headers forward once. Looking for a begin after every header
	// would rescan large malformed files quadratically.
	lastHeaderStart, lastHeaderEnd := -1, -1
	lastBodyName := ""
	for offset := 0; offset < lastEnd; {
		match := subprogram.FindStringSubmatchIndex(body[offset:lastEnd])
		if match == nil {
			break
		}
		lastHeaderStart = offset + match[0]
		lastHeaderEnd = offset + match[1]
		lastBodyName = body[offset+match[2] : offset+match[3]]
		offset = lastHeaderEnd
	}
	if lastHeaderStart < 0 {
		return begin.FindStringIndex(body[:lastEnd]) != nil
	}
	opening := begin.FindStringIndex(body[lastHeaderEnd:lastEnd])
	if opening == nil {
		return false
	}
	if !isKeyword.MatchString(body[lastHeaderStart : lastHeaderEnd+opening[0]]) {
		// A subprogram prototype without a body does not own a bare end.
		return begin.FindStringIndex(body[:lastEnd]) != nil
	}
	lastBodyStart := lastHeaderEnd + opening[1]
	closing := regexp.MustCompile(`(?im)^[ \t]*end(?:[ \t]+(?:function|procedure)(?:[ \t]+` +
		regexp.QuoteMeta(lastBodyName) + `)?|[ \t]+` + regexp.QuoteMeta(lastBodyName) + `)?[ \t]*;`)
	end := closing.FindStringIndex(body[lastBodyStart:lastEnd])
	if end == nil {
		return false
	}
	return begin.FindStringIndex(body[lastBodyStart+end[1]:lastEnd]) != nil
}

var (
	verilogModule = regexp.MustCompile(`(?im)^[ \t]*module[ \t]+([A-Za-z_][A-Za-z0-9_$]*)\b`)
	svPackage     = regexp.MustCompile(`(?im)^[ \t]*package[ \t]+([A-Za-z_][A-Za-z0-9_$]*)[ \t]*;`)
	svInterface   = regexp.MustCompile(`(?im)^[ \t]*interface[ \t]+([A-Za-z_][A-Za-z0-9_$]*)\b`)
	hdlSignal     = regexp.MustCompile(`(?i)\b(?:wire|reg|logic|bit)[ \t]+(?:signed[ \t]+|unsigned[ \t]+)?(?:\[[^\]\r\n]+\][ \t]+)?([A-Za-z_][A-Za-z0-9_$]*)\b`)
	// Only declarations introduced by parameter/localparam (and their bounded
	// header continuations) are indexed, never instance overrides or assignments.
	hdlParameterDeclaration  = regexp.MustCompile(`^[ \t]*(parameter|localparam)[ \t]+(?:(?:[A-Za-z_][A-Za-z0-9_$]*|\[[^\]\r\n]*\])[ \t]+)*([A-Za-z_][A-Za-z0-9_$]*)[ \t]*=`)
	hdlParameterContinuation = regexp.MustCompile(`^[ \t]*([A-Za-z_][A-Za-z0-9_$]*)[ \t]*=`)
	svTypedefStruct          = regexp.MustCompile(`(?is)typedef[ \t]+struct(?:[ \t]+packed)?[ \t]*\{.*?\}[ \t]*([A-Za-z_][A-Za-z0-9_$]*)[ \t]*;`)
	svTypedefLogic           = regexp.MustCompile(`(?m)^[ \t]*typedef[ \t]+logic[ \t]+(?:(?:signed|unsigned)[ \t]+)?(?:\[[^\]\r\n]+\][ \t]*)*([A-Za-z_][A-Za-z0-9_$]*)[ \t]*;`)
	svTypedefEnum            = regexp.MustCompile(`(?ms)^[ \t]*typedef[ \t]+enum(?:[ \t]+(?:logic|bit|reg|int|integer|byte|shortint|longint)(?:[ \t]*\[[^\]\r\n]+\])?)?[ \t\r\n]*\{[^{}]*\}[ \t\r\n]*([A-Za-z_][A-Za-z0-9_$]*)[ \t]*;`)
	hdlEndModulePattern      = regexp.MustCompile(`(?i)\bendmodule\b`)
	hdlEndPackagePattern     = regexp.MustCompile(`(?i)\bendpackage\b`)
	hdlEndInterfacePattern   = regexp.MustCompile(`(?i)\bendinterface\b`)
	hdlPackageBlockBoundary  = regexp.MustCompile(`(?i)^[ \t]*(?:(?:pure|virtual|static|automatic|extern|local|protected)[ \t]+)*(class|function|task|endclass|endfunction|endtask)\b`)
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
	// Verilog apostrophes introduce numeric literals, not quoted strings.
	// Replace them only in the analysis copy before masking comments; when
	// no apostrophe exists, ReplaceAll preserves the original allocation.
	source := maskSourceComments(strings.ReplaceAll(document.Text, "'", " "), []string{"//"}, "/*", "*/")
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
				hdlSVStructTypes(builder, document, source, match[1], end, &symbol, true)
				hdlSVPackageCallables(builder, document, source, match[1], end, &symbol)
				hdlSignals(builder, document, source, match[1], end, &symbol, false)
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
				hdlSVStructTypes(builder, document, source, match[1], end, &symbol, false)
				hdlSVInterfaceModports(builder, document, source, match[1], end, &symbol)
				hdlSVInterfaceClocking(builder, document, source, match[1], end, &symbol)
				hdlSignals(builder, document, source, match[1], end, &symbol, true)
			}
		}
	}
	for search := 0; search < len(source); {
		match := verilogModule.FindStringSubmatchIndex(source[search:])
		if match == nil {
			break
		}
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		matchStart := search + match[0]
		matchEnd := search + match[1]
		nameStart := search + match[2]
		nameEnd := search + match[3]
		name := document.Text[nameStart:nameEnd]
		end := hdlEnd(source, matchEnd, "endmodule")
		if end < 0 {
			addDocumentDataHardwareDiagnostic(builder, language+"-unclosed-module", language+" module is not structurally closed", matchStart, matchEnd)
			search = matchEnd
			continue
		}
		symbol, ok := addDocumentDataHardwareSymbol(builder, SymbolKindModule, "module", name, nil, OffsetRange{Start: matchStart, End: end}, OffsetRange{Start: nameStart, End: nameEnd})
		if ok {
			if systemVerilog {
				hdlSVStructTypes(builder, document, source, matchEnd, end, &symbol, false)
				hdlSVModuleProperties(builder, document, source, matchEnd, end, &symbol)
				hdlSVModuleSequences(builder, document, source, matchEnd, end, &symbol)
				hdlSVModuleCovergroups(builder, document, source, matchEnd, end, &symbol)
			}
			hdlSignals(builder, document, source, matchEnd, end, &symbol, true)
		}
		search = matchEnd
	}
	if systemVerilog {
		hdlSVTopLevelCheckers(builder, document, source)
		hdlSVTopLevelPrograms(builder, document, source)
	}
	return AnalyzerResult{Analysis: builder.takeResult()}, nil
}

// Recognized packed structs, direct logic aliases and enums are named types,
// not independently declared interface/module signals.
func hdlSVStructTypes(builder *SymbolBuilder, document *SourceDocument, source string, start, end int, parent *SymbolParent, packageScope bool) {
	body := source[start:end]
	if packageScope {
		body = hdlMaskPackageNestedBlocks(body)
	}
	for _, definition := range []struct {
		pattern    *regexp.Regexp
		nativeKind string
	}{{svTypedefStruct, "typedef-struct"}, {svTypedefLogic, "typedef-logic"}, {svTypedefEnum, "typedef-enum"}} {
		for _, match := range definition.pattern.FindAllStringSubmatchIndex(body, -1) {
			nameStart, nameEnd := start+match[2], start+match[3]
			typ, added := addDocumentDataHardwareSymbol(builder, SymbolKindType, definition.nativeKind,
				document.Text[nameStart:nameEnd], parent,
				OffsetRange{Start: start + match[0], End: start + match[1]},
				OffsetRange{Start: nameStart, End: nameEnd})
			if added && definition.nativeKind == "typedef-enum" {
				hdlSVEnumMembers(builder, document, body, start, match[0], match[1], &typ)
			}
		}
	}
}

// hdlSVEnumMembers indexes only authored names inside a bounded simple enum.
// Verify the complete list before emitting members, so unsupported syntax does
// not produce an apparently complete partial enumeration.
func hdlSVEnumMembers(builder *SymbolBuilder, document *SourceDocument, body string, base, start, end int, parent *SymbolParent) {
	incomplete := func(at int) {
		addDocumentDataHardwareDiagnostic(builder, "systemverilog-unsupported-enum-members",
			"Unsupported or malformed SystemVerilog enum members require direct source inspection", base+at, base+at+1)
	}
	if end <= start || end-start > 16*1024 {
		incomplete(start)
		return
	}
	span := body[start:end]
	open, close := strings.IndexByte(span, '{'), strings.LastIndexByte(span, '}')
	if open < 0 || close <= open {
		incomplete(start)
		return
	}
	type member struct {
		name                   string
		begin, finish, nameEnd int
	}
	members := make([]member, 0, 16)
	seen := make(map[string]struct{})
	segmentStart, parens, brackets := open+1, 0, 0
	for i := open + 1; i <= close; i++ {
		if i < close {
			switch span[i] {
			case '(':
				parens++
			case ')':
				parens--
			case '[':
				brackets++
			case ']':
				brackets--
			}
			if parens < 0 || brackets < 0 {
				incomplete(start + i)
				return
			}
			if span[i] != ',' || parens != 0 || brackets != 0 {
				continue
			}
		}
		if parens != 0 || brackets != 0 {
			incomplete(start + i)
			return
		}
		segment := span[segmentStart:i]
		entry := strings.TrimSpace(segment)
		if entry == "" {
			if i == close {
				break // Empty body or trailing comma.
			}
			incomplete(start + i)
			return
		}
		if len(members) >= 128 {
			incomplete(start + segmentStart)
			return
		}
		leading := len(segment) - len(strings.TrimLeft(segment, " \t\r\n"))
		nameEnd := 0
		for nameEnd < len(entry) {
			c := entry[nameEnd]
			if nameEnd == 0 {
				if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_') {
					break
				}
			} else if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '$') {
				break
			}
			nameEnd++
		}
		if nameEnd == 0 {
			incomplete(start + segmentStart + leading)
			return
		}
		rest := strings.TrimSpace(entry[nameEnd:])
		if rest != "" && (!strings.HasPrefix(rest, "=") || strings.TrimSpace(rest[1:]) == "") {
			incomplete(start + segmentStart + leading + nameEnd)
			return
		}
		name := entry[:nameEnd]
		if _, exists := seen[name]; exists {
			incomplete(start + segmentStart + leading)
			return
		}
		seen[name] = struct{}{}
		begin := base + start + segmentStart + leading
		members = append(members, member{name: document.Text[begin : begin+nameEnd], begin: begin, finish: begin + len(entry), nameEnd: begin + nameEnd})
		segmentStart = i + 1
	}
	for _, m := range members {
		addDocumentDataHardwareSymbol(builder, SymbolKindConstant, "enum-value", m.name, parent,
			OffsetRange{Start: m.begin, End: m.finish}, OffsetRange{Start: m.begin, End: m.nameEnd})
	}
}

func hdlEnd(text string, start int, terminator string) int {
	var pattern *regexp.Regexp
	switch terminator {
	case "endmodule":
		pattern = hdlEndModulePattern
	case "endpackage":
		pattern = hdlEndPackagePattern
	case "endinterface":
		pattern = hdlEndInterfacePattern
	default:
		pattern = regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(terminator) + `\b`)
	}
	for _, line := range sourceTextLines(text[start:]) {
		code := stripLineComment(line.text, "//")
		if match := pattern.FindStringIndex(code); match != nil {
			return start + line.start + match[1]
		}
	}
	return -1
}

// A module/interface header can declare multiple parameters after a single
// "parameter" keyword. Scan only its #(...) list: scanning arbitrary commas
// would misclassify instance overrides and ordinary expressions as declarations.
func hdlHeaderParameters(builder *SymbolBuilder, document *SourceDocument, body string, base int, parent *SymbolParent) string {
	hash := 0
	for hash < len(body) && (body[hash] == ' ' || body[hash] == '\t' || body[hash] == '\r' || body[hash] == '\n') {
		hash++
	}
	if hash >= len(body) || body[hash] != '#' {
		return body
	}
	open := hash + 1
	for open < len(body) && (body[open] == ' ' || body[open] == '\t' || body[open] == '\r' || body[open] == '\n') {
		open++
	}
	if open >= len(body) || body[open] != '(' {
		return body
	}
	closing, depth := -1, 1
	for i := open + 1; i < len(body); i++ {
		switch body[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				closing = i
			}
		}
		if closing >= 0 {
			break
		}
	}
	if closing < 0 {
		return body
	}

	// Mask the parsed header before the ordinary line-based parameter/signal
	// scanner. Preserve CR/LF and byte offsets for exact source ranges.
	masked := []byte(body)
	for i := hash; i <= closing; i++ {
		if masked[i] != '\r' && masked[i] != '\n' {
			masked[i] = ' '
		}
	}

	segmentStart := open + 1
	paren, bracket, brace := 0, 0, 0
	inDeclaration := false
	kind := SymbolKindConstant
	declarationKind := "parameter"
	for i := segmentStart; i <= closing; i++ {
		if i != closing {
			switch body[i] {
			case '(':
				paren++
			case ')':
				paren--
			case '[':
				bracket++
			case ']':
				bracket--
			case '{':
				brace++
			case '}':
				brace--
			}
			if body[i] != ',' || paren != 0 || bracket != 0 || brace != 0 {
				continue
			}
		}

		segment := body[segmentStart:i]
		trimmed := strings.TrimLeft(segment, " \t\r\n")
		offset := base + segmentStart + len(segment) - len(trimmed)
		match := hdlParameterDeclaration.FindStringSubmatchIndex(trimmed)
		nameStart, nameEnd := -1, -1
		if match != nil {
			inDeclaration = true
			kind = SymbolKindConstant
			if fields := strings.Fields(trimmed[match[3]:match[4]]); len(fields) > 0 && fields[0] == "type" {
				kind = SymbolKindType
			}
			declarationKind = trimmed[match[2]:match[3]]
			nameStart, nameEnd = match[4], match[5]
		} else if inDeclaration {
			if continuation := hdlParameterContinuation.FindStringSubmatchIndex(trimmed); continuation != nil {
				nameStart, nameEnd = continuation[2], continuation[3]
			} else {
				inDeclaration = false
			}
		}
		if nameStart >= 0 {
			addDocumentDataHardwareSymbol(builder, kind, declarationKind,
				document.Text[offset+nameStart:offset+nameEnd], parent,
				OffsetRange{Start: offset, End: base + i},
				OffsetRange{Start: offset + nameStart, End: offset + nameEnd})
		}
		segmentStart = i + 1
	}
	return string(masked)
}

// A package parameter is not owned by the package when declared inside a
// nested class, function or task. Keep byte offsets and line breaks unchanged.
func hdlMaskPackageNestedBlocks(body string) string {
	var masked []byte
	depth := 0
	for _, line := range sourceTextLines(body) {
		match := hdlPackageBlockBoundary.FindStringSubmatch(line.text)
		kind := ""
		if len(match) > 1 {
			kind = strings.ToLower(match[1])
		}
		switch kind {
		case "class", "function", "task":
			depth++
		}
		if depth > 0 {
			if masked == nil {
				masked = []byte(body)
			}
			for i := line.start; i < line.end; i++ {
				if masked[i] != '\n' && masked[i] != '\r' {
					masked[i] = ' '
				}
			}
		}
		switch kind {
		case "endclass", "endfunction", "endtask":
			if depth > 0 {
				depth--
			}
		}
	}
	if masked != nil {
		return string(masked)
	}
	return body
}

func hdlSignals(builder *SymbolBuilder, document *SourceDocument, source string, start, end int, parent *SymbolParent, includeSignals bool) {
	if end <= start || end > len(document.Text) || len(source) != len(document.Text) {
		return
	}
	body := source[start:end]
	if !includeSignals {
		body = hdlMaskPackageNestedBlocks(body)
	}
	body = hdlHeaderParameters(builder, document, body, start, parent)
	// Named types are not module/interface signals; preserve line offsets.
	var maskedTypes []byte
	for _, pattern := range []*regexp.Regexp{svTypedefStruct, svTypedefLogic, svTypedefEnum} {
		for _, match := range pattern.FindAllStringIndex(body, -1) {
			if maskedTypes == nil {
				maskedTypes = []byte(body)
			}
			for i := match[0]; i < match[1]; i++ {
				if maskedTypes[i] != '\r' && maskedTypes[i] != '\n' {
					maskedTypes[i] = ' '
				}
			}
		}
	}
	if maskedTypes != nil {
		body = string(maskedTypes)
	}
	// A typed SystemVerilog parameter may contain "logic" or "bit". Mask
	// recognized parameter lines before scanning signals so that a parameter
	// is not also misreported as a signal.
	if strings.Contains(body, "parameter") || strings.Contains(body, "localparam") {
		var masked []byte
		for _, line := range sourceTextLines(body) {
			match := hdlParameterDeclaration.FindStringSubmatchIndex(line.text)
			if match == nil {
				continue
			}
			kind := SymbolKindConstant
			if fields := strings.Fields(line.text[match[3]:match[4]]); len(fields) > 0 && fields[0] == "type" {
				kind = SymbolKindType
			}
			nameStart, nameEnd := start+line.start+match[4], start+line.start+match[5]
			addDocumentDataHardwareSymbol(builder, kind, line.text[match[2]:match[3]],
				document.Text[nameStart:nameEnd], parent,
				OffsetRange{Start: start + line.start + match[0], End: start + line.end},
				OffsetRange{Start: nameStart, End: nameEnd})
			if masked == nil {
				masked = []byte(body)
			}
			for index := line.start; index < line.end; index++ {
				masked[index] = ' '
			}
		}
		if masked != nil {
			body = string(masked)
		}
	}
	if !includeSignals {
		return
	}
	for search := 0; search < len(body); {
		match := hdlSignal.FindStringSubmatchIndex(body[search:])
		if match == nil {
			break
		}
		matchStart := search + match[0]
		matchEnd := search + match[1]
		nameStart := search + match[2]
		nameEnd := search + match[3]
		name := document.Text[start+nameStart : start+nameEnd]
		addDocumentDataHardwareSymbol(builder, SymbolKindSignal, "signal", name, parent,
			OffsetRange{Start: start + matchStart, End: start + matchEnd},
			OffsetRange{Start: start + nameStart, End: start + nameEnd})
		search = matchEnd
	}
}

var (
	assemblyLabel        = regexp.MustCompile(`(?m)^[ \t]*([0-9]+|[A-Za-z_.$][A-Za-z0-9_.$@]*):`)
	assemblyMASMProc     = regexp.MustCompile(`(?im)^[ \t]*([A-Za-z_.$][A-Za-z0-9_.$@]*)[ \t]+PROC\b`)
	assemblyEQU          = regexp.MustCompile(`(?im)^[ \t]*([A-Za-z_.$][A-Za-z0-9_.$@]*)[ \t]+EQU[ \t]+[^ \t;\r\n][^;\r\n]*`)
	assemblyData         = regexp.MustCompile(`(?i)^[ \t]*([A-Za-z_.$][A-Za-z0-9_.$@]*)[ \t]+(DB|DW)[ \t]+[^ \t;\r\n][^;\r\n]*`)
	assemblyMotorolaData = regexp.MustCompile(`(?i)^[ \t]*([A-Za-z_.$][A-Za-z0-9_.$@]*)[ \t]+(FCB|FCC|FDB|RMB)[ \t]+([^ \t\r\n][^\r\n]*)`)
)

func (AssemblyAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newStructuralAnalyzerBuilder(ctx, document, options, "assembly", AnalyzerAssembly)
	if err != nil {
		return AnalyzerResult{}, err
	}
	if err := assemblyNamedDeclarations(ctx, builder, document.Text); err != nil {
		return AnalyzerResult{}, err
	}
	return AnalyzerResult{Analysis: builder.Result()}, nil
}

// Index labels, MASM procedures, EQU constants, and supported named data
// declarations outside recognized macro/structure definitions, without expansion.
func assemblyNamedDeclarations(ctx context.Context, builder *SymbolBuilder, text string) error {
	var structureDepth, macroDepth int
	for start := 0; start < len(text); {
		if err := ctx.Err(); err != nil {
			return err
		}
		end := len(text)
		if index := strings.IndexByte(text[start:], '\n'); index >= 0 {
			end = start + index
		}
		line := text[start:end]
		words := strings.Fields(strings.TrimSpace(line))
		if len(words) > 0 && !strings.HasPrefix(words[0], ";") {
			upper := strings.ToUpper(words[0])
			second := ""
			if len(words) > 1 {
				second = strings.ToUpper(words[1])
			}
			openMacro := upper == "%MACRO" || upper == "MACRO" || second == "MACRO"
			closeMacro := upper == "ENDM" || upper == "%ENDMACRO"
			openStruct := upper == "STRUC" || second == "STRUC" || second == "STRUCT"
			closeStruct := upper == "ENDSTRUC" || upper == "ENDS" || second == "ENDS"
			switch {
			case macroDepth > 0:
				if openMacro {
					macroDepth++
				} else if closeMacro {
					macroDepth--
				}
			case structureDepth > 0:
				if openStruct {
					structureDepth++
				} else if closeStruct {
					structureDepth--
				}
			case openMacro:
				macroDepth = 1
			case openStruct:
				structureDepth = 1
			default:
				if match := assemblyLabel.FindStringSubmatchIndex(line); match != nil {
					name := line[match[2]:match[3]]
					addDocumentDataHardwareSymbol(builder, SymbolKindLabel, "label", name, nil,
						OffsetRange{Start: start + match[0], End: start + match[1]},
						OffsetRange{Start: start + match[2], End: start + match[3]})
				} else if match := assemblyMASMProc.FindStringSubmatchIndex(line); match != nil {
					name := line[match[2]:match[3]]
					addDocumentDataHardwareSymbol(builder, SymbolKindLabel, "masm-proc", name, nil,
						OffsetRange{Start: start + match[0], End: start + match[1]},
						OffsetRange{Start: start + match[2], End: start + match[3]})
				} else if match := assemblyEQU.FindStringSubmatchIndex(line); match != nil {
					name := line[match[2]:match[3]]
					addDocumentDataHardwareSymbol(builder, SymbolKindConstant, "equ", name, nil,
						OffsetRange{Start: start + match[0], End: start + match[1]},
						OffsetRange{Start: start + match[2], End: start + match[3]})
				} else if match := assemblyData.FindStringSubmatchIndex(line); match != nil {
					name := line[match[2]:match[3]]
					addDocumentDataHardwareSymbol(builder, SymbolKindVariable, "data", name, nil,
						OffsetRange{Start: start + match[0], End: start + match[1]},
						OffsetRange{Start: start + match[2], End: start + match[3]})
				} else if match := assemblyMotorolaData.FindStringSubmatchIndex(line); match != nil &&
					assemblyMotorolaDataOperandValid(line[match[4]:match[5]], line[match[6]:match[7]]) {
					name := line[match[2]:match[3]]
					addDocumentDataHardwareSymbol(builder, SymbolKindVariable, "data", name, nil,
						OffsetRange{Start: start + match[0], End: start + match[1]},
						OffsetRange{Start: start + match[2], End: start + match[3]})
				}
			}
		}
		if end == len(text) {
			break
		}
		start = end + 1
	}
	if structureDepth > 0 || macroDepth > 0 {
		builder.MarkIncomplete()
	}
	return nil
}

// Validate only directive shape, not assembler expression semantics.
// FCC accepts printable ASCII delimiters and requires a matching terminator.
func assemblyMotorolaDataOperandValid(directive, operand string) bool {
	operand = strings.TrimSpace(operand)
	if operand == "" {
		return false
	}
	if !strings.EqualFold(directive, "FCC") {
		if operand[0] == ',' || operand[0] == ';' {
			return false
		}
		if strings.EqualFold(directive, "RMB") {
			if comment := strings.IndexByte(operand, ';'); comment >= 0 {
				operand = operand[:comment]
			}
			return !strings.Contains(operand, ",")
		}
		return true
	}
	delimiter := operand[0]
	if delimiter < '!' || delimiter > '~' {
		return false
	}
	for index := 1; index < len(operand); index++ {
		if operand[index] != delimiter {
			continue
		}
		rest := strings.TrimSpace(operand[index+1:])
		if rest == "" || strings.HasPrefix(rest, ";") {
			return true
		}
	}
	return false
}
