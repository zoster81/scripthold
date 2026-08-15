package sourceintelligence

// RelationKind is the language-neutral public relation query vocabulary.
type RelationKind string

const (
	RelationDependencies    RelationKind = "dependencies"
	RelationDependents      RelationKind = "dependents"
	RelationReferences      RelationKind = "references"
	RelationDefinitions     RelationKind = "definitions"
	RelationInheritance     RelationKind = "inheritance"
	RelationImplementations RelationKind = "implementations"
	RelationOverrides       RelationKind = "overrides"
	RelationCallers         RelationKind = "callers"
	RelationCallees         RelationKind = "callees"
	RelationTrace           RelationKind = "trace"
	RelationImpact          RelationKind = "impact"
	RelationCycles          RelationKind = "cycles"
)

// ResolutionState is orthogonal to evidence strength. A structural fact can be
// unresolved or ambiguous without pretending either state is a weaker evidence
// level than textual/lexical/structural analysis.
type ResolutionState string

const (
	ResolutionResolved   ResolutionState = "resolved"
	ResolutionAmbiguous  ResolutionState = "ambiguous"
	ResolutionUnresolved ResolutionState = "unresolved"
	ResolutionExternal   ResolutionState = "external"
)

// IndexStaleness reports whether a result was produced from the authoritative
// current generation, a caller-allowed stale generation, or without an index.
type IndexStaleness string

const (
	IndexCurrent    IndexStaleness = "current"
	IndexStale      IndexStaleness = "stale"
	IndexNotIndexed IndexStaleness = "not-indexed"
)

// IndexEvidence binds project-wide source intelligence to one coherent index
// generation/fingerprint. A zero generation is permitted only when no index was
// used; query-layer validation enforces the public request contract.
type IndexEvidence struct {
	Generation  uint64         `json:"generation,omitempty"`
	Fingerprint string         `json:"fingerprint,omitempty"`
	Staleness   IndexStaleness `json:"staleness"`
}

// RelationEntity is the common endpoint identity carried by R27 relationships.
// Exact source bodies are deliberately excluded; authoritative source is read
// separately and fingerprint-verified when body text is required.
type RelationEntity struct {
	Path              string `json:"path"`
	Language          string `json:"language,omitempty"`
	SymbolID          string `json:"symbolId,omitempty"`
	QualifiedName     string `json:"qualifiedName,omitempty"`
	SourceFingerprint string `json:"sourceFingerprint,omitempty"`
	Range             *Range `json:"range,omitempty"`
}

// RelationRecord is one normalized evidence-qualified edge.
type RelationRecord struct {
	Kind       RelationKind    `json:"kind"`
	Source     RelationEntity  `json:"source"`
	Target     RelationEntity  `json:"target"`
	Evidence   SymbolEvidence  `json:"evidence"`
	Resolution ResolutionState `json:"resolution"`
}

// ContextReason records the deterministic priority class that caused an item
// to be retained by source_context.
type ContextReason string

const (
	ContextTarget                 ContextReason = "target"
	ContextEnclosing              ContextReason = "enclosing"
	ContextDirectDependency       ContextReason = "direct-dependency"
	ContextDirectRelatedBody      ContextReason = "direct-related-body"
	ContextDirectRelatedSignature ContextReason = "direct-related-signature"
	ContextReverseOrTypeRelation  ContextReason = "reverse-or-type-relation"
	ContextDeeperRelation         ContextReason = "deeper-relation"
)

// ContextRepresentation states whether source_context retained a verified body
// or degraded deterministically to a signature under the byte budget.
type ContextRepresentation string

const (
	ContextBody      ContextRepresentation = "body"
	ContextSignature ContextRepresentation = "signature"
)

// ContextItem is the normalized bounded source_context result unit.
type ContextItem struct {
	Entity         RelationEntity        `json:"entity"`
	Reason         ContextReason         `json:"reason"`
	Representation ContextRepresentation `json:"representation"`
	Priority       int                   `json:"priority"`
	Text           string                `json:"text"`
	Evidence       SymbolEvidence        `json:"evidence"`
	Resolution     ResolutionState       `json:"resolution"`
}
