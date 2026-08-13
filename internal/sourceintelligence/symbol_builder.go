package sourceintelligence

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

// SymbolEvidence is the strongest fact class an analyzer may claim for one symbol.
type SymbolEvidence string

const (
	SymbolEvidenceTextual         SymbolEvidence = "textual"
	SymbolEvidenceLexical         SymbolEvidence = "lexical"
	SymbolEvidenceStructural      SymbolEvidence = "structural"
	SymbolEvidenceScopeResolved   SymbolEvidence = "scope-resolved"
	SymbolEvidenceProjectResolved SymbolEvidence = "project-resolved"
	SymbolEvidenceSemantic        SymbolEvidence = "semantic"
)

var symbolEvidenceRank = map[SymbolEvidence]int{
	SymbolEvidenceTextual:         1,
	SymbolEvidenceLexical:         2,
	SymbolEvidenceStructural:      3,
	SymbolEvidenceScopeResolved:   4,
	SymbolEvidenceProjectResolved: 5,
	SymbolEvidenceSemantic:        6,
}

// SymbolKind is the normalized language-neutral declaration kind.
type SymbolKind string

const (
	SymbolKindPackage        SymbolKind = "package"
	SymbolKindNamespace      SymbolKind = "namespace"
	SymbolKindModule         SymbolKind = "module"
	SymbolKindClass          SymbolKind = "class"
	SymbolKindStruct         SymbolKind = "struct"
	SymbolKindInterface      SymbolKind = "interface"
	SymbolKindEnum           SymbolKind = "enum"
	SymbolKindRecord         SymbolKind = "record"
	SymbolKindType           SymbolKind = "type"
	SymbolKindAlias          SymbolKind = "alias"
	SymbolKindFunction       SymbolKind = "function"
	SymbolKindMethod         SymbolKind = "method"
	SymbolKindConstructor    SymbolKind = "constructor"
	SymbolKindDestructor     SymbolKind = "destructor"
	SymbolKindOperator       SymbolKind = "operator"
	SymbolKindProperty       SymbolKind = "property"
	SymbolKindEvent          SymbolKind = "event"
	SymbolKindField          SymbolKind = "field"
	SymbolKindConstant       SymbolKind = "constant"
	SymbolKindVariable       SymbolKind = "variable"
	SymbolKindDelegate       SymbolKind = "delegate"
	SymbolKindTrait          SymbolKind = "trait"
	SymbolKindImplementation SymbolKind = "implementation"
)

var normalizedSymbolKinds = map[SymbolKind]struct{}{
	SymbolKindPackage: {}, SymbolKindNamespace: {}, SymbolKindModule: {},
	SymbolKindClass: {}, SymbolKindStruct: {}, SymbolKindInterface: {}, SymbolKindEnum: {}, SymbolKindRecord: {},
	SymbolKindType: {}, SymbolKindAlias: {}, SymbolKindFunction: {}, SymbolKindMethod: {}, SymbolKindConstructor: {},
	SymbolKindDestructor: {}, SymbolKindOperator: {}, SymbolKindProperty: {}, SymbolKindEvent: {}, SymbolKindField: {},
	SymbolKindConstant: {}, SymbolKindVariable: {}, SymbolKindDelegate: {}, SymbolKindTrait: {}, SymbolKindImplementation: {},
}

// Visibility is a normalized declaration visibility when the source states one.
type Visibility string

const (
	VisibilityPublic    Visibility = "public"
	VisibilityPrivate   Visibility = "private"
	VisibilityProtected Visibility = "protected"
	VisibilityInternal  Visibility = "internal"
	VisibilityFriend    Visibility = "friend"
	VisibilityPackage   Visibility = "package"
)

var normalizedVisibilities = map[Visibility]struct{}{
	VisibilityPublic: {}, VisibilityPrivate: {}, VisibilityProtected: {},
	VisibilityInternal: {}, VisibilityFriend: {}, VisibilityPackage: {},
}

// OffsetRange is an internal half-open UTF-8 byte range over SourceDocument.Text.
type OffsetRange struct {
	Start int
	End   int
}

// SymbolParent allows analyzers to express ownership that is not the current
// lexical scope, such as a Go method receiver.
type SymbolParent struct {
	ID            string
	QualifiedName string
}

// SymbolSpec is the analyzer-facing language-neutral declaration input.
type SymbolSpec struct {
	Kind          SymbolKind
	NativeKind    string
	Name          string
	QualifiedName string
	Parent        *SymbolParent
	RegionID      string
	Declaration   OffsetRange
	NameRange     OffsetRange
	Signature     *OffsetRange
	Body          *OffsetRange
	Visibility    Visibility
	Modifiers     []string
	Evidence      SymbolEvidence
	Disambiguator string
}

// NormalizedSymbol is the shared R25 symbol IR. Raw parser/AST types never escape
// through this record. Internal UTF-8 offsets are intentionally unexported.
type NormalizedSymbol struct {
	ID                  string         `json:"id"`
	Path                string         `json:"path"`
	Language            string         `json:"language"`
	Kind                SymbolKind     `json:"kind"`
	NativeKind          string         `json:"nativeKind"`
	Name                string         `json:"name"`
	QualifiedName       string         `json:"qualifiedName,omitempty"`
	ParentID            string         `json:"parentId,omitempty"`
	ParentQualifiedName string         `json:"parentQualifiedName,omitempty"`
	RegionID            string         `json:"regionId,omitempty"`
	DeclarationRange    Range          `json:"declarationRange"`
	NameRange           Range          `json:"nameRange"`
	SignatureRange      *Range         `json:"signatureRange,omitempty"`
	BodyRange           *Range         `json:"bodyRange,omitempty"`
	Signature           string         `json:"signature,omitempty"`
	Visibility          Visibility     `json:"visibility,omitempty"`
	Modifiers           []string       `json:"modifiers,omitempty"`
	Evidence            SymbolEvidence `json:"evidence"`
	Analyzer            string         `json:"analyzer"`

	declarationOffsets OffsetRange
	nameOffsets        OffsetRange
	signatureOffsets   *OffsetRange
	bodyOffsets        *OffsetRange
	signatureTruncated bool
}

// SourceOffsets exposes decoded-text UTF-8 offsets only to in-module internal
// orchestration. They are never serialized through the MCP/public contract.
func (symbol NormalizedSymbol) SourceOffsets() (declaration, name OffsetRange, signature, body *OffsetRange) {
	return symbol.declarationOffsets, symbol.nameOffsets, cloneOffsetRange(symbol.signatureOffsets), cloneOffsetRange(symbol.bodyOffsets)
}

func (symbol NormalizedSymbol) sourceOffsets() (declaration OffsetRange, signature, body *OffsetRange) {
	declaration, _, signature, body = symbol.SourceOffsets()
	return declaration, signature, body
}

// DiagnosticSeverity is the normalized diagnostic severity.
type DiagnosticSeverity string

const (
	DiagnosticInfo    DiagnosticSeverity = "info"
	DiagnosticWarning DiagnosticSeverity = "warning"
	DiagnosticError   DiagnosticSeverity = "error"
)

// DiagnosticSpec is the analyzer-facing bounded diagnostic input.
type DiagnosticSpec struct {
	Code            string
	Message         string
	Severity        DiagnosticSeverity
	Range           *OffsetRange
	AffectsCoverage bool
}

// AnalysisDiagnostic is the normalized diagnostic output.
type AnalysisDiagnostic struct {
	Code     string             `json:"code"`
	Message  string             `json:"message"`
	Severity DiagnosticSeverity `json:"severity"`
	Range    *Range             `json:"range,omitempty"`
}

// SymbolBuilderLimits are per-file retained-output bounds supplied by orchestration.
type SymbolBuilderLimits struct {
	MaxSymbols        int
	MaxSignatureBytes int
	MaxDiagnostics    int
}

// SymbolBuilderOptions define one analyzer's truthful capability ceiling.
type SymbolBuilderOptions struct {
	Context            context.Context
	Language           string
	Analyzer           string
	RegionID           string
	QualifierSeparator string
	IncludeSignatures  bool
	MaxEvidence        SymbolEvidence
	Limits             SymbolBuilderLimits
}

// AnalysisResult is the common bounded per-document analyzer result.
type AnalysisResult struct {
	Symbols              []NormalizedSymbol   `json:"symbols"`
	Diagnostics          []AnalysisDiagnostic `json:"diagnostics,omitempty"`
	CoverageComplete     bool                 `json:"coverageComplete"`
	Truncated            bool                 `json:"truncated,omitempty"`
	DiagnosticsTruncated bool                 `json:"diagnosticsTruncated,omitempty"`
}

// SymbolBuilder centralizes ranges, hierarchy, evidence, IDs, limits, diagnostics,
// ordering, and coverage so language analyzers cannot silently diverge.
type SymbolBuilder struct {
	document      *SourceDocument
	options       SymbolBuilderOptions
	scopes        *ScopeStack
	result        AnalysisResult
	validationErr error
	seenIDs       map[string]struct{}
}

// NewSymbolBuilder creates a builder. Invalid options are retained as a stable
// error returned by mutation methods so analyzers cannot accidentally ignore them.
func NewSymbolBuilder(document *SourceDocument, options SymbolBuilderOptions) *SymbolBuilder {
	if options.Context == nil {
		options.Context = context.Background()
	}
	if options.QualifierSeparator == "" {
		options.QualifierSeparator = "."
	}
	builder := &SymbolBuilder{
		document: document,
		options:  options,
		scopes:   NewScopeStack(),
		result: AnalysisResult{
			CoverageComplete: true,
		},
		seenIDs: make(map[string]struct{}),
	}
	builder.validationErr = validateSymbolBuilderOptions(document, options)
	if builder.validationErr != nil {
		builder.result.CoverageComplete = false
	}
	return builder
}

func validateSymbolBuilderOptions(document *SourceDocument, options SymbolBuilderOptions) error {
	if document == nil {
		return operation.New(operation.KindInvalidInput, "source document is required")
	}
	if strings.TrimSpace(options.Language) == "" {
		return operation.New(operation.KindInvalidInput, "symbol builder language is required")
	}
	if strings.TrimSpace(options.Analyzer) == "" {
		return operation.New(operation.KindInvalidInput, "symbol builder analyzer is required")
	}
	if _, ok := symbolEvidenceRank[options.MaxEvidence]; !ok {
		return operation.New(operation.KindInvalidInput, "symbol builder maximum evidence is invalid")
	}
	if options.Limits.MaxSymbols <= 0 || options.Limits.MaxSignatureBytes <= 0 || options.Limits.MaxDiagnostics <= 0 {
		return operation.New(operation.KindInvalidInput, "symbol builder limits must be positive")
	}
	return nil
}

func (builder *SymbolBuilder) Scopes() *ScopeStack {
	if builder == nil {
		return nil
	}
	return builder.scopes
}

// Add normalizes and appends one declaration while enforcing all common R25
// evidence, range, identity, hierarchy, and retention rules.
func (builder *SymbolBuilder) Add(spec SymbolSpec) (NormalizedSymbol, error) {
	if builder == nil {
		return NormalizedSymbol{}, operation.New(operation.KindInvalidInput, "symbol builder is nil")
	}
	if err := builder.checkReady(); err != nil {
		return NormalizedSymbol{}, err
	}
	if len(builder.result.Symbols) >= builder.options.Limits.MaxSymbols {
		builder.result.Truncated = true
		builder.recordDiagnostic(AnalysisDiagnostic{
			Code: "symbol-limit", Message: "symbol retention limit reached", Severity: DiagnosticWarning,
		}, true)
		return NormalizedSymbol{}, operation.Wrap(
			operation.KindLimit,
			"build_source_symbol",
			builder.document.Path,
			fmt.Errorf("symbol count exceeds limit %d", builder.options.Limits.MaxSymbols),
		)
	}

	normalized, err := builder.normalizeSymbol(spec)
	if err != nil {
		return NormalizedSymbol{}, err
	}
	if _, exists := builder.seenIDs[normalized.ID]; exists {
		return NormalizedSymbol{}, operation.Wrap(
			operation.KindInvalidInput,
			"build_source_symbol",
			builder.document.Path,
			fmt.Errorf("duplicate symbol identity %s", normalized.ID),
		)
	}
	builder.seenIDs[normalized.ID] = struct{}{}
	builder.result.Symbols = append(builder.result.Symbols, normalized)
	if normalized.signatureTruncated {
		builder.result.Truncated = true
		builder.recordDiagnostic(AnalysisDiagnostic{
			Code: "signature-limit", Message: "signature text exceeds retention limit", Severity: DiagnosticWarning,
			Range: cloneRange(normalized.SignatureRange),
		}, true)
	}
	return cloneNormalizedSymbol(normalized), nil
}

func (builder *SymbolBuilder) normalizeSymbol(spec SymbolSpec) (NormalizedSymbol, error) {
	if _, ok := normalizedSymbolKinds[spec.Kind]; !ok {
		return NormalizedSymbol{}, builder.invalidSpec("unknown normalized symbol kind %q", spec.Kind)
	}
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return NormalizedSymbol{}, builder.invalidSpec("symbol name is required")
	}
	nativeKind := strings.TrimSpace(spec.NativeKind)
	if nativeKind == "" {
		nativeKind = string(spec.Kind)
	}
	if spec.Visibility != "" {
		if _, ok := normalizedVisibilities[spec.Visibility]; !ok {
			return NormalizedSymbol{}, builder.invalidSpec("unknown normalized visibility %q", spec.Visibility)
		}
	}

	evidence := spec.Evidence
	if evidence == "" {
		evidence = SymbolEvidenceStructural
	}
	rank, ok := symbolEvidenceRank[evidence]
	if !ok {
		return NormalizedSymbol{}, builder.invalidSpec("unknown symbol evidence %q", evidence)
	}
	if rank > symbolEvidenceRank[builder.options.MaxEvidence] {
		return NormalizedSymbol{}, builder.invalidSpec("symbol evidence %q exceeds analyzer capability %q", evidence, builder.options.MaxEvidence)
	}

	if err := builder.validateRequiredRanges(spec.Declaration, spec.NameRange); err != nil {
		return NormalizedSymbol{}, err
	}
	declarationRange, err := builder.document.RangeFromUTF8Offsets(spec.Declaration.Start, spec.Declaration.End)
	if err != nil {
		return NormalizedSymbol{}, builder.invalidSpec("invalid declaration range: %v", err)
	}
	nameRange, err := builder.document.RangeFromUTF8Offsets(spec.NameRange.Start, spec.NameRange.End)
	if err != nil {
		return NormalizedSymbol{}, builder.invalidSpec("invalid name range: %v", err)
	}

	parent := SymbolParent{}
	if spec.Parent != nil {
		parent = *spec.Parent
	} else if current, ok := builder.scopes.Current(); ok {
		parent.ID = current.SymbolID
		parent.QualifiedName = current.QualifiedName
	}
	qualifiedName := strings.TrimSpace(spec.QualifiedName)
	if qualifiedName == "" {
		qualifiedName = name
		if parent.QualifiedName != "" {
			qualifiedName = parent.QualifiedName + builder.options.QualifierSeparator + name
		}
	}

	regionID := strings.TrimSpace(spec.RegionID)
	if regionID == "" {
		regionID = strings.TrimSpace(builder.options.RegionID)
	}
	normalized := NormalizedSymbol{
		Path:                builder.document.Path,
		Language:            builder.options.Language,
		Kind:                spec.Kind,
		NativeKind:          nativeKind,
		Name:                name,
		QualifiedName:       qualifiedName,
		ParentID:            strings.TrimSpace(parent.ID),
		ParentQualifiedName: strings.TrimSpace(parent.QualifiedName),
		RegionID:            regionID,
		DeclarationRange:    declarationRange,
		NameRange:           nameRange,
		Visibility:          spec.Visibility,
		Modifiers:           normalizeModifiers(spec.Modifiers),
		Evidence:            evidence,
		Analyzer:            builder.options.Analyzer,
		declarationOffsets:  spec.Declaration,
		nameOffsets:         spec.NameRange,
	}
	if spec.Signature != nil {
		normalized.signatureOffsets = cloneOffsetRange(spec.Signature)
		publicRange, rangeErr := builder.normalizeOptionalRange("signature", *spec.Signature, spec.Declaration)
		if rangeErr != nil {
			return NormalizedSymbol{}, rangeErr
		}
		normalized.SignatureRange = &publicRange
		if builder.options.IncludeSignatures {
			if spec.Signature.End-spec.Signature.Start > builder.options.Limits.MaxSignatureBytes {
				normalized.signatureTruncated = true
			} else {
				text, _, sliceErr := builder.document.SliceUTF8Offsets(spec.Signature.Start, spec.Signature.End, builder.options.Limits.MaxSignatureBytes)
				if sliceErr != nil {
					return NormalizedSymbol{}, builder.invalidSpec("invalid signature slice: %v", sliceErr)
				}
				normalized.Signature = text
			}
		}
	}
	if spec.Body != nil {
		normalized.bodyOffsets = cloneOffsetRange(spec.Body)
		publicRange, rangeErr := builder.normalizeOptionalRange("body", *spec.Body, spec.Declaration)
		if rangeErr != nil {
			return NormalizedSymbol{}, rangeErr
		}
		normalized.BodyRange = &publicRange
	}

	normalized.ID = deterministicSymbolID(builder.document.Path, builder.options.Language, normalized, spec.Disambiguator)
	return normalized, nil
}

func (builder *SymbolBuilder) validateRequiredRanges(declaration, name OffsetRange) error {
	if !validNonEmptyOffsetRange(declaration, len(builder.document.Text)) {
		return builder.invalidSpec("declaration range [%d,%d) is invalid", declaration.Start, declaration.End)
	}
	if !validNonEmptyOffsetRange(name, len(builder.document.Text)) {
		return builder.invalidSpec("name range [%d,%d) is invalid", name.Start, name.End)
	}
	if name.Start < declaration.Start || name.End > declaration.End {
		return builder.invalidSpec("name range [%d,%d) is outside declaration range [%d,%d)", name.Start, name.End, declaration.Start, declaration.End)
	}
	return nil
}

func (builder *SymbolBuilder) normalizeOptionalRange(label string, value, declaration OffsetRange) (Range, error) {
	if !validOffsetRange(value, len(builder.document.Text)) {
		return Range{}, builder.invalidSpec("%s range [%d,%d) is invalid", label, value.Start, value.End)
	}
	if value.Start < declaration.Start || value.End > declaration.End {
		return Range{}, builder.invalidSpec("%s range [%d,%d) is outside declaration range [%d,%d)", label, value.Start, value.End, declaration.Start, declaration.End)
	}
	publicRange, err := builder.document.RangeFromUTF8Offsets(value.Start, value.End)
	if err != nil {
		return Range{}, builder.invalidSpec("invalid %s range: %v", label, err)
	}
	return publicRange, nil
}

func validNonEmptyOffsetRange(value OffsetRange, textLength int) bool {
	return value.Start >= 0 && value.End > value.Start && value.End <= textLength
}

func validOffsetRange(value OffsetRange, textLength int) bool {
	return value.Start >= 0 && value.End >= value.Start && value.End <= textLength
}

func normalizeModifiers(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	sort.Strings(result)
	return result
}

func deterministicSymbolID(path, language string, symbol NormalizedSymbol, disambiguator string) string {
	hash := sha256.New()
	parts := []string{
		path,
		language,
		string(symbol.Kind),
		symbol.NativeKind,
		symbol.Name,
		symbol.QualifiedName,
		symbol.ParentID,
		symbol.ParentQualifiedName,
		symbol.RegionID,
		fmt.Sprintf("%d", symbol.declarationOffsets.Start),
		fmt.Sprintf("%d", symbol.declarationOffsets.End),
		fmt.Sprintf("%d", symbol.nameOffsets.Start),
		fmt.Sprintf("%d", symbol.nameOffsets.End),
		strings.TrimSpace(disambiguator),
	}
	var length [8]byte
	for _, part := range parts {
		binary.BigEndian.PutUint64(length[:], uint64(len(part)))
		_, _ = hash.Write(length[:])
		_, _ = hash.Write([]byte(part))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// AddDiagnostic appends one bounded normalized diagnostic. Coverage is lowered
// even when an affecting diagnostic itself falls beyond the retention cap.
func (builder *SymbolBuilder) AddDiagnostic(spec DiagnosticSpec) error {
	if builder == nil {
		return operation.New(operation.KindInvalidInput, "symbol builder is nil")
	}
	if err := builder.checkReady(); err != nil {
		return err
	}
	diagnostic, err := builder.normalizeDiagnostic(spec)
	if err != nil {
		return err
	}
	builder.recordDiagnostic(diagnostic, spec.AffectsCoverage)
	return nil
}

func (builder *SymbolBuilder) normalizeDiagnostic(spec DiagnosticSpec) (AnalysisDiagnostic, error) {
	severity := spec.Severity
	if severity == "" {
		severity = DiagnosticWarning
	}
	if severity != DiagnosticInfo && severity != DiagnosticWarning && severity != DiagnosticError {
		return AnalysisDiagnostic{}, builder.invalidSpec("unknown diagnostic severity %q", severity)
	}
	if strings.TrimSpace(spec.Code) == "" || strings.TrimSpace(spec.Message) == "" {
		return AnalysisDiagnostic{}, builder.invalidSpec("diagnostic code and message are required")
	}
	diagnostic := AnalysisDiagnostic{Code: strings.TrimSpace(spec.Code), Message: strings.TrimSpace(spec.Message), Severity: severity}
	if spec.Range != nil {
		if !validOffsetRange(*spec.Range, len(builder.document.Text)) {
			return AnalysisDiagnostic{}, builder.invalidSpec("diagnostic range [%d,%d) is invalid", spec.Range.Start, spec.Range.End)
		}
		publicRange, err := builder.document.RangeFromUTF8Offsets(spec.Range.Start, spec.Range.End)
		if err != nil {
			return AnalysisDiagnostic{}, builder.invalidSpec("invalid diagnostic range: %v", err)
		}
		diagnostic.Range = &publicRange
	}
	return diagnostic, nil
}

func (builder *SymbolBuilder) recordDiagnostic(diagnostic AnalysisDiagnostic, affectsCoverage bool) {
	if affectsCoverage {
		builder.result.CoverageComplete = false
	}
	if len(builder.result.Diagnostics) >= builder.options.Limits.MaxDiagnostics {
		builder.result.DiagnosticsTruncated = true
		return
	}
	diagnostic.Range = cloneRange(diagnostic.Range)
	builder.result.Diagnostics = append(builder.result.Diagnostics, diagnostic)
}

// MarkIncomplete records known analysis incompleteness without fabricating a
// diagnostic. Callers should normally add a concrete diagnostic when possible.
func (builder *SymbolBuilder) MarkIncomplete() {
	if builder != nil {
		builder.result.CoverageComplete = false
	}
}

// MarkTruncated records a bounded omission performed by analyzer/orchestration.
func (builder *SymbolBuilder) MarkTruncated() {
	if builder != nil {
		builder.result.Truncated = true
		builder.result.CoverageComplete = false
	}
}

// Result returns a defensive, deterministic source-ordered snapshot.
func (builder *SymbolBuilder) Result() AnalysisResult {
	if builder == nil {
		return AnalysisResult{}
	}
	result := builder.result
	result.Symbols = make([]NormalizedSymbol, len(builder.result.Symbols))
	for index, symbol := range builder.result.Symbols {
		result.Symbols[index] = cloneNormalizedSymbol(symbol)
	}
	sort.Slice(result.Symbols, func(i, j int) bool {
		left := result.Symbols[i]
		right := result.Symbols[j]
		if left.declarationOffsets.Start != right.declarationOffsets.Start {
			return left.declarationOffsets.Start < right.declarationOffsets.Start
		}
		if left.declarationOffsets.End != right.declarationOffsets.End {
			return left.declarationOffsets.End < right.declarationOffsets.End
		}
		return left.ID < right.ID
	})
	result.Diagnostics = append([]AnalysisDiagnostic(nil), builder.result.Diagnostics...)
	for index := range result.Diagnostics {
		if result.Diagnostics[index].Range != nil {
			rangeCopy := *result.Diagnostics[index].Range
			result.Diagnostics[index].Range = &rangeCopy
		}
	}
	return result
}

func (builder *SymbolBuilder) checkReady() error {
	if builder.validationErr != nil {
		return builder.validationErr
	}
	if err := builder.options.Context.Err(); err != nil {
		return operation.Wrap(operation.KindCancelled, "build_source_symbol", builder.document.Path, err)
	}
	return nil
}

func (builder *SymbolBuilder) invalidSpec(format string, args ...any) error {
	return operation.Wrap(operation.KindInvalidInput, "build_source_symbol", builder.document.Path, fmt.Errorf(format, args...))
}

func cloneNormalizedSymbol(symbol NormalizedSymbol) NormalizedSymbol {
	symbol.Modifiers = append([]string(nil), symbol.Modifiers...)
	if symbol.SignatureRange != nil {
		value := *symbol.SignatureRange
		symbol.SignatureRange = &value
	}
	if symbol.BodyRange != nil {
		value := *symbol.BodyRange
		symbol.BodyRange = &value
	}
	symbol.signatureOffsets = cloneOffsetRange(symbol.signatureOffsets)
	symbol.bodyOffsets = cloneOffsetRange(symbol.bodyOffsets)
	return symbol
}

func cloneRange(value *Range) *Range {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneOffsetRange(value *OffsetRange) *OffsetRange {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
