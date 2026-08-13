package sourceintelligence

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/zoster81/scripthold/internal/operation"
)

const (
	DetectionContentProbeBytes = 64 * 1024
	DetectionMaxCandidates     = 16
	DetectionMaxEvidence       = 32
)

// DetectionState reports evidence quality without fabricated percentage confidence.
type DetectionState string

const (
	DetectionExact     DetectionState = "exact"
	DetectionProbable  DetectionState = "probable"
	DetectionAmbiguous DetectionState = "ambiguous"
	DetectionUnknown   DetectionState = "unknown"
)

// EvidenceKind is the ordered source of one language-routing fact.
type EvidenceKind string

const (
	EvidenceExplicit       EvidenceKind = "explicit"
	EvidenceExactBasename  EvidenceKind = "exact-basename"
	EvidenceCompoundSuffix EvidenceKind = "compound-suffix"
	EvidenceExtension      EvidenceKind = "extension"
	EvidenceShebang        EvidenceKind = "shebang"
	EvidenceDirective      EvidenceKind = "directive"
	EvidenceContentMarker  EvidenceKind = "content-marker"
	EvidenceProjectHint    EvidenceKind = "project-hint"
	EvidenceAnalyzerProbe  EvidenceKind = "analyzer-probe"
)

// LanguageEvidence is one bounded, deterministic detector observation.
type LanguageEvidence struct {
	Kind     EvidenceKind `json:"kind"`
	Language string       `json:"language,omitempty"`
	Detail   string       `json:"detail,omitempty"`
}

// LanguageCandidate is a ranked candidate without a percentage confidence claim.
type LanguageCandidate struct {
	Language          string       `json:"language"`
	StrongestEvidence EvidenceKind `json:"strongestEvidence"`
}

// DetectionResult is the bounded language-routing result.
type DetectionResult struct {
	State      DetectionState      `json:"state"`
	Language   string              `json:"language,omitempty"`
	Candidates []LanguageCandidate `json:"candidates,omitempty"`
	Evidence   []LanguageEvidence  `json:"evidence,omitempty"`
	Truncated  bool                `json:"truncated,omitempty"`
}

// LanguageProbe is an optional bounded structural probe used only after cheap
// evidence. Probe implementations are supplied by analyzer orchestration later.
type LanguageProbe struct {
	Language string
	Probe    func(context.Context, string) (bool, error)
}

// DetectionInput contains already-decoded text; encoding choice is deliberately
// absent so language routing cannot influence charset detection.
type DetectionInput struct {
	Path             string
	Text             string
	ExplicitLanguage string
	ProjectLanguages []string
	MaxProbes        int
	Probes           []LanguageProbe
}

type detectionCandidateState struct {
	language       string
	priority       int
	strongest      EvidenceKind
	insertionOrder int
	hasExtension   bool
	hasContent     bool
}

type detectionCollector struct {
	candidates              map[string]*detectionCandidateState
	evidence                []LanguageEvidence
	truncated               bool
	nextOrder               int
	extensionCandidateCount int
}

var (
	safeShebangInterpreter = regexp.MustCompile(`^[A-Za-z0-9_.+-]+$`)
	safeDirectiveValue     = regexp.MustCompile(`^[A-Za-z0-9_+#.+-]+$`)
	modelineLanguage       = regexp.MustCompile(`(?i)(?:\b(?:ft|filetype)\s*=\s*([a-z0-9_+#.-]+)|\bmode\s*:\s*([a-z0-9_+#.-]+))`)
	goContentMarker        = regexp.MustCompile(`(?m)^\s*package\s+[A-Za-z_][A-Za-z0-9_]*\s*(?:$|//)`)
	csharpContentMarker    = regexp.MustCompile(`(?m)^\s*(?:using\s+[A-Za-z_]|namespace\s+[A-Za-z_]|(?:(?:public|internal|private|protected|static|abstract|sealed|partial)\s+)*(?:class|struct|interface|record|enum)\s+[A-Za-z_])`)
	vbnetContentMarker     = regexp.MustCompile(`(?im)^\s*(?:Imports\s+[A-Za-z_]|(?:(?:Public|Private|Friend|Protected|Partial|MustInherit|NotInheritable)\s+)*(?:Class|Module|Structure|Interface|Enum)\s+[A-Za-z_]|End\s+(?:Class|Module|Structure|Interface|Enum)\b)`)
	pythonContentMarker    = regexp.MustCompile(`(?m)^\s*(?:(?:async\s+)?def\s+[A-Za-z_][A-Za-z0-9_]*\s*\(|class\s+[A-Za-z_][A-Za-z0-9_]*(?:\s*\(|\s*:)|(?:from|import)\s+[A-Za-z_])`)
)

const (
	priorityExplicit       = 100
	priorityExactBasename  = 95
	priorityCompoundSuffix = 90
	priorityShebang        = 80
	priorityDirective      = 80
	priorityExtension      = 70
	priorityContent        = 50
	priorityAnalyzerProbe  = 40
	priorityProjectHint    = 20
)

// DetectLanguage applies the frozen ordered evidence cascade to bounded decoded text.
func DetectLanguage(ctx context.Context, registry *LanguageRegistry, input DetectionInput) (DetectionResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return DetectionResult{}, operation.Wrap(operation.KindCancelled, "detect_source_language", input.Path, err)
	}
	if registry == nil {
		return DetectionResult{}, operation.New(operation.KindInvalidInput, "language registry is required")
	}

	collector := newDetectionCollector()
	if input.ExplicitLanguage != "" {
		descriptor, ok := registry.Resolve(input.ExplicitLanguage)
		if !ok {
			return DetectionResult{}, operation.Wrap(
				operation.KindInvalidInput,
				"detect_source_language",
				input.Path,
				fmt.Errorf("unknown explicit language %q", input.ExplicitLanguage),
			)
		}
		collector.add(descriptor.ID, EvidenceExplicit, normalizeLanguageName(input.ExplicitLanguage), priorityExplicit)
		return collector.finalize(), nil
	}

	base := filepath.Base(input.Path)
	if descriptor, ok := registry.ExactBasename(base); ok {
		collector.add(descriptor.ID, EvidenceExactBasename, base, priorityExactBasename)
	}
	if descriptor, suffix, ok := registry.CompoundSuffix(base); ok {
		collector.add(descriptor.ID, EvidenceCompoundSuffix, suffix, priorityCompoundSuffix)
	}
	if extension := filepath.Ext(base); extension != "" {
		for _, descriptor := range registry.ExtensionCandidates(extension) {
			collector.add(descriptor.ID, EvidenceExtension, strings.ToLower(extension), priorityExtension)
		}
	}

	probeText := boundedDetectionText(input.Text)
	if interpreter := parseShebangInterpreter(probeText); interpreter != "" {
		for _, descriptor := range registry.ShebangCandidates(interpreter) {
			collector.add(descriptor.ID, EvidenceShebang, interpreter, priorityShebang)
		}
	}
	addDirectiveEvidence(registry, collector, probeText)
	addContentMarkerEvidence(registry, collector, probeText)

	for _, hinted := range input.ProjectLanguages {
		if descriptor, ok := registry.Resolve(hinted); ok {
			collector.add(descriptor.ID, EvidenceProjectHint, normalizeLanguageName(hinted), priorityProjectHint)
		}
	}

	if input.MaxProbes > 0 && len(input.Probes) > 0 {
		limit := min(input.MaxProbes, len(input.Probes))
		for index := 0; index < limit; index++ {
			if err := ctx.Err(); err != nil {
				return DetectionResult{}, operation.Wrap(operation.KindCancelled, "detect_source_language", input.Path, err)
			}
			probe := input.Probes[index]
			if probe.Probe == nil {
				continue
			}
			descriptor, ok := registry.Resolve(probe.Language)
			if !ok {
				continue
			}
			matched, err := probe.Probe(ctx, probeText)
			if err != nil {
				if ctx.Err() != nil {
					return DetectionResult{}, operation.Wrap(operation.KindCancelled, "detect_source_language", input.Path, ctx.Err())
				}
				return DetectionResult{}, err
			}
			if matched {
				collector.add(descriptor.ID, EvidenceAnalyzerProbe, descriptor.ID, priorityAnalyzerProbe)
			}
		}
		if len(input.Probes) > limit {
			collector.truncated = true
		}
	}

	return collector.finalize(), nil
}

func newDetectionCollector() *detectionCollector {
	return &detectionCollector{candidates: make(map[string]*detectionCandidateState)}
}

func (collector *detectionCollector) add(language string, kind EvidenceKind, detail string, priority int) {
	if language == "" {
		return
	}
	candidate, exists := collector.candidates[language]
	if !exists {
		candidate = &detectionCandidateState{language: language, insertionOrder: collector.nextOrder}
		collector.nextOrder++
		collector.candidates[language] = candidate
	}
	if kind == EvidenceExtension && !candidate.hasExtension {
		candidate.hasExtension = true
		collector.extensionCandidateCount++
	}
	if kind == EvidenceContentMarker {
		candidate.hasContent = true
	}
	if priority > candidate.priority {
		candidate.priority = priority
		candidate.strongest = kind
	}
	if len(collector.evidence) < DetectionMaxEvidence {
		collector.evidence = append(collector.evidence, LanguageEvidence{Kind: kind, Language: language, Detail: detail})
	} else {
		collector.truncated = true
	}
}

func (collector *detectionCollector) finalize() DetectionResult {
	states := make([]detectionCandidateState, 0, len(collector.candidates))
	for _, candidate := range collector.candidates {
		states = append(states, *candidate)
	}
	sort.SliceStable(states, func(i, j int) bool {
		if states[i].priority != states[j].priority {
			return states[i].priority > states[j].priority
		}
		if states[i].insertionOrder != states[j].insertionOrder {
			return states[i].insertionOrder < states[j].insertionOrder
		}
		return states[i].language < states[j].language
	})

	result := DetectionResult{Evidence: append([]LanguageEvidence(nil), collector.evidence...), Truncated: collector.truncated}
	candidateLimit := min(len(states), DetectionMaxCandidates)
	result.Candidates = make([]LanguageCandidate, 0, candidateLimit)
	for index := 0; index < candidateLimit; index++ {
		result.Candidates = append(result.Candidates, LanguageCandidate{
			Language:          states[index].language,
			StrongestEvidence: states[index].strongest,
		})
	}
	if len(states) > candidateLimit {
		result.Truncated = true
	}

	if len(states) == 0 {
		result.State = DetectionUnknown
		return result
	}
	if states[0].priority >= priorityCompoundSuffix {
		result.State = DetectionExact
		result.Language = states[0].language
		return result
	}
	if len(states) == 1 {
		result.State = DetectionProbable
		result.Language = states[0].language
		return result
	}
	if collector.extensionCandidateCount == 1 &&
		states[0].priority == priorityExtension &&
		states[0].hasExtension &&
		states[0].hasContent &&
		states[1].priority <= priorityContent {
		result.State = DetectionProbable
		result.Language = states[0].language
		return result
	}
	if states[0].priority-states[1].priority >= 30 {
		result.State = DetectionProbable
		result.Language = states[0].language
		return result
	}
	result.State = DetectionAmbiguous
	return result
}

func boundedDetectionText(text string) string {
	if len(text) <= DetectionContentProbeBytes {
		return text
	}
	limit := DetectionContentProbeBytes
	for limit > 0 && !utf8.RuneStart(text[limit]) {
		limit--
	}
	return text[:limit]
}

func parseShebangInterpreter(text string) string {
	if !strings.HasPrefix(text, "#!") {
		return ""
	}
	line := text
	if index := strings.IndexAny(line, "\r\n"); index >= 0 {
		line = line[:index]
	}
	if len(line) > 512 {
		return ""
	}
	fields := strings.Fields(strings.TrimSpace(strings.TrimPrefix(line, "#!")))
	if len(fields) == 0 {
		return ""
	}
	interpreter := strings.ToLower(filepath.Base(fields[0]))
	if interpreter == "env" {
		if len(fields) < 2 || !safeShebangInterpreter.MatchString(fields[1]) {
			return ""
		}
		return strings.ToLower(fields[1])
	}
	if !safeShebangInterpreter.MatchString(interpreter) {
		return ""
	}
	return interpreter
}

func addDirectiveEvidence(registry *LanguageRegistry, collector *detectionCollector, text string) {
	if hasClassicASPLanguageDirective(text) {
		if descriptor, ok := registry.Lookup("classic-asp"); ok {
			collector.add(descriptor.ID, EvidenceDirective, "asp-language-directive", priorityDirective)
		}
	}
	matches := modelineLanguage.FindAllStringSubmatch(text, 8)
	for _, match := range matches {
		name := ""
		if len(match) > 1 {
			name = match[1]
		}
		if name == "" && len(match) > 2 {
			name = match[2]
		}
		if descriptor, ok := registry.Resolve(name); ok {
			collector.add(descriptor.ID, EvidenceDirective, normalizeLanguageName(name), priorityDirective)
		}
	}
}

func hasClassicASPLanguageDirective(text string) bool {
	const maxDirectiveBytes = 2048
	lower := strings.ToLower(text)
	searchFrom := 0
	for {
		relativeStart := strings.Index(lower[searchFrom:], "<%@")
		if relativeStart < 0 {
			return false
		}
		start := searchFrom + relativeStart
		remaining := lower[start+3:]
		relativeEnd := strings.Index(remaining, "%>")
		if relativeEnd < 0 {
			return false
		}
		end := start + 3 + relativeEnd
		if end-start <= maxDirectiveBytes {
			directive := lower[start+3 : end]
			if containsASPDirectiveAssignment(directive, "language") {
				return true
			}
		}
		searchFrom = end + 2
		if searchFrom >= len(lower) {
			return false
		}
	}
}

func containsASPDirectiveAssignment(directive, name string) bool {
	for offset := 0; offset < len(directive); {
		index := strings.Index(directive[offset:], name)
		if index < 0 {
			return false
		}
		index += offset
		beforeOK := index == 0 || !isDirectiveWordByte(directive[index-1])
		after := index + len(name)
		afterOK := after == len(directive) || !isDirectiveWordByte(directive[after])
		if beforeOK && afterOK {
			for after < len(directive) && (directive[after] == ' ' || directive[after] == '\t' || directive[after] == '\r' || directive[after] == '\n') {
				after++
			}
			if after < len(directive) && directive[after] == '=' {
				after++
				for after < len(directive) && (directive[after] == ' ' || directive[after] == '\t' || directive[after] == '\r' || directive[after] == '\n') {
					after++
				}
				if directiveAssignmentHasValue(directive[after:]) {
					return true
				}
			}
		}
		offset = index + len(name)
	}
	return false
}

func directiveAssignmentHasValue(value string) bool {
	if value == "" {
		return false
	}
	if value[0] == '\'' || value[0] == '"' {
		quote := value[0]
		end := strings.IndexByte(value[1:], quote)
		if end < 0 {
			return false
		}
		token := strings.TrimSpace(value[1 : end+1])
		return token != "" && safeDirectiveValue.MatchString(token)
	}
	end := strings.IndexAny(value, " \t\r\n")
	if end < 0 {
		end = len(value)
	}
	token := value[:end]
	return token != "" && safeDirectiveValue.MatchString(token)
}

func isDirectiveWordByte(value byte) bool {
	return value == '_' || value >= 'a' && value <= 'z' || value >= '0' && value <= '9'
}

func addContentMarkerEvidence(registry *LanguageRegistry, collector *detectionCollector, text string) {
	markers := []struct {
		language string
		pattern  *regexp.Regexp
		detail   string
	}{
		{language: "classic-asp", pattern: regexp.MustCompile(`(?is)<%(?:@|=|\s)`), detail: "asp-server-block"},
		{language: "go", pattern: goContentMarker, detail: "go-package"},
		{language: "csharp", pattern: csharpContentMarker, detail: "csharp-declaration"},
		{language: "vbnet", pattern: vbnetContentMarker, detail: "vbnet-declaration"},
		{language: "python", pattern: pythonContentMarker, detail: "python-declaration"},
	}
	for _, marker := range markers {
		if marker.pattern.MatchString(text) {
			if descriptor, ok := registry.Lookup(marker.language); ok {
				collector.add(descriptor.ID, EvidenceContentMarker, marker.detail, priorityContent)
			}
		}
	}
}
