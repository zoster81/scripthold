package markdownintelligence

import (
	"fmt"

	"github.com/zoster81/marksplice"
)

// SectionSummary is a snapshot-bound view of one Marksplice derived section.
type SectionSummary struct {
	TargetID              string `json:"targetId"`
	HeadingTargetID       string `json:"headingTargetId"`
	ParentHeadingTargetID string `json:"parentHeadingTargetId,omitempty"`
	Level                 int    `json:"level"`
	Range                 Range  `json:"range"`
	BodyRange             Range  `json:"bodyRange"`
}

// RelationshipSummary exposes one parser-resolved Marksplice relationship
// without serializing Marksplice node identities.
type RelationshipSummary struct {
	Kind                        string `json:"kind"`
	Destination                 string `json:"destination"`
	SourceOffset                int    `json:"sourceOffset"`
	SourceTargetID              string `json:"sourceTargetId,omitempty"`
	Title                       string `json:"title,omitempty"`
	Email                       bool   `json:"email,omitempty"`
	Reference                   string `json:"reference,omitempty"`
	ReferenceForm               string `json:"referenceForm,omitempty"`
	ReferenceDefinitionTargetID string `json:"referenceDefinitionTargetId,omitempty"`
	FragmentStatus              string `json:"fragmentStatus,omitempty"`
	FragmentTargetID            string `json:"fragmentTargetId,omitempty"`
}

// FragmentResolution is one unique Marksplice fragment target.
type FragmentResolution struct {
	Kind     string `json:"kind"`
	Value    string `json:"value"`
	TargetID string `json:"targetId"`
}

// FencedBlockSummary preserves the broader read-only fenced-block view exposed
// by Marksplice, including blocks that do not qualify for FencedCode mutation.
type FencedBlockSummary struct {
	TargetID           string `json:"targetId"`
	Range              Range  `json:"range"`
	Closed             bool   `json:"closed"`
	Fence              string `json:"fence"`
	OpeningFenceLength int    `json:"openingFenceLength"`
	OpeningIndent      int    `json:"openingIndent"`
	Info               string `json:"info,omitempty"`
	Language           string `json:"language,omitempty"`
}

// AlertSummary is the read-only GitHub alert overlay on a blockquote target.
type AlertSummary struct {
	TargetID    string `json:"targetId"`
	Kind        string `json:"kind"`
	Range       Range  `json:"range"`
	MarkerRange Range  `json:"markerRange"`
}

// FootnoteReferenceSummary is relationship metadata for one parser-proven
// footnote occurrence. Its source range is diagnostic, not generic mutation
// authority.
type FootnoteReferenceSummary struct {
	Label              string `json:"label"`
	Occurrence         int    `json:"occurrence"`
	Range              Range  `json:"range"`
	LabelRange         Range  `json:"labelRange"`
	DefinitionTargetID string `json:"definitionTargetId,omitempty"`
}

// HeadingAnchorSummary is one Marksplice-derived heading anchor.
type HeadingAnchorSummary struct {
	Value    string `json:"value"`
	TargetID string `json:"targetId"`
}

// FrontMatterSummary describes the recognized document-leading metadata
// envelope. Fields themselves are returned through the ordinary node model.
type FrontMatterSummary struct {
	Format       string `json:"format"`
	Range        Range  `json:"range"`
	OpeningRange Range  `json:"openingRange"`
	ClosingRange Range  `json:"closingRange"`
}

// InspectResult groups bounded semantic views for one Markdown snapshot.
type InspectResult struct {
	Nodes              []NodeSummary              `json:"nodes,omitempty"`
	Sections           []SectionSummary           `json:"sections,omitempty"`
	Relationships      []RelationshipSummary      `json:"relationships,omitempty"`
	HeadingAnchors     []HeadingAnchorSummary     `json:"headingAnchors,omitempty"`
	FencedBlocks       []FencedBlockSummary       `json:"fencedBlocks,omitempty"`
	Alerts             []AlertSummary             `json:"alerts,omitempty"`
	FootnoteReferences []FootnoteReferenceSummary `json:"footnoteReferences,omitempty"`
	FrontMatter        *FrontMatterSummary        `json:"frontMatter,omitempty"`
	Truncated          bool                       `json:"truncated,omitempty"`
}

// Inspect returns bounded semantic summaries from Marksplice. The same positive
// limit is applied independently to each variable-length public collection.
func (s *Snapshot) Inspect(limit int) (InspectResult, error) {
	if limit <= 0 {
		return InspectResult{}, fmt.Errorf("%w: inspect limit must be positive", marksplice.ErrInvalidQuery)
	}
	result := InspectResult{}

	nodes, truncated, err := s.queryNodesBounded(nil, limit)
	if err != nil {
		return InspectResult{}, err
	}
	result.Nodes = nodes
	result.Truncated = truncated

	sections, truncated, err := s.querySectionsBounded(nil, nil, limit)
	if err != nil {
		return InspectResult{}, err
	}
	result.Sections = sections
	result.Truncated = result.Truncated || truncated

	relationships := s.document.LinkRelationships()
	if len(relationships) > limit {
		relationships = relationships[:limit]
		result.Truncated = true
	}
	result.Relationships = make([]RelationshipSummary, 0, len(relationships))
	for _, relationship := range relationships {
		result.Relationships = append(result.Relationships, s.relationshipSummary(relationship))
	}

	anchors := s.document.HeadingAnchors()
	if len(anchors) > limit {
		anchors = anchors[:limit]
		result.Truncated = true
	}
	result.HeadingAnchors = make([]HeadingAnchorSummary, 0, len(anchors))
	for _, anchor := range anchors {
		target, ok := s.nodeTargetID(anchor.HeadingID())
		if !ok {
			continue
		}
		result.HeadingAnchors = append(result.HeadingAnchors, HeadingAnchorSummary{Value: anchor.Value(), TargetID: target})
	}

	fenced := s.document.FencedBlocks()
	if len(fenced) > limit {
		fenced = fenced[:limit]
		result.Truncated = true
	}
	result.FencedBlocks = make([]FencedBlockSummary, 0, len(fenced))
	for _, block := range fenced {
		rangeValue := block.Range()
		summary := FencedBlockSummary{
			TargetID:           targetID(s.fingerprint, "fenced_block", rangeValue),
			Range:              fromMarkspliceRange(rangeValue),
			Closed:             block.Closed(),
			Fence:              string([]byte{block.FenceChar()}),
			OpeningFenceLength: block.OpeningFenceLength(),
			OpeningIndent:      block.OpeningIndent(),
		}
		if info, ok := block.Info(); ok {
			summary.Info = info
		}
		if language, ok := block.Language(); ok {
			summary.Language = language
		}
		result.FencedBlocks = append(result.FencedBlocks, summary)
	}

	alerts := s.document.Alerts()
	if len(alerts) > limit {
		alerts = alerts[:limit]
		result.Truncated = true
	}
	result.Alerts = make([]AlertSummary, 0, len(alerts))
	for _, alert := range alerts {
		target, ok := s.nodeTargetID(alert.ID())
		if !ok {
			continue
		}
		result.Alerts = append(result.Alerts, AlertSummary{
			TargetID:    target,
			Kind:        alertKindName(alert.Kind()),
			Range:       fromMarkspliceRange(alert.Range()),
			MarkerRange: fromMarkspliceRange(alert.MarkerRange()),
		})
	}

	footnoteReferences := s.document.FootnoteReferences()
	if len(footnoteReferences) > limit {
		footnoteReferences = footnoteReferences[:limit]
		result.Truncated = true
	}
	result.FootnoteReferences = make([]FootnoteReferenceSummary, 0, len(footnoteReferences))
	for _, reference := range footnoteReferences {
		summary := FootnoteReferenceSummary{
			Label: reference.Label(), Occurrence: reference.Occurrence(),
			Range: fromMarkspliceRange(reference.Range()), LabelRange: fromMarkspliceRange(reference.LabelRange()),
		}
		if definitionID, ok := reference.DefinitionID(); ok {
			summary.DefinitionTargetID, _ = s.nodeTargetID(definitionID)
		}
		result.FootnoteReferences = append(result.FootnoteReferences, summary)
	}

	if frontMatter, ok := s.document.FrontMatter(); ok {
		result.FrontMatter = &FrontMatterSummary{
			Format:       frontMatterFormatName(frontMatter.Format()),
			Range:        fromMarkspliceRange(frontMatter.Range()),
			OpeningRange: fromMarkspliceRange(frontMatter.OpeningRange()),
			ClosingRange: fromMarkspliceRange(frontMatter.ClosingRange()),
		}
	}
	return result, nil
}

// QuerySections returns bounded derived sections in source order.
func (s *Snapshot) QuerySections(levels []int, within *Range, limit int) ([]SectionSummary, bool, error) {
	return s.querySectionsBounded(levels, within, limit)
}

func (s *Snapshot) querySectionsBounded(levels []int, within *Range, limit int) ([]SectionSummary, bool, error) {
	if s == nil || s.document == nil {
		return nil, false, fmt.Errorf("%w: markdown snapshot is unavailable", marksplice.ErrInvalidQuery)
	}
	if limit <= 0 {
		return nil, false, fmt.Errorf("%w: section query limit must be positive", marksplice.ErrInvalidQuery)
	}
	query := marksplice.SectionQuery{Levels: append([]int(nil), levels...), Limit: limit + 1}
	if within != nil {
		value := marksplice.Range{Start: within.Start, End: within.End}
		query.Within = &value
	}
	sections, err := s.document.QuerySections(query)
	if err != nil {
		return nil, false, err
	}
	truncated := len(sections) > limit
	if truncated {
		sections = sections[:limit]
	}
	result := make([]SectionSummary, 0, len(sections))
	for _, section := range sections {
		result = append(result, s.sectionSummary(section))
	}
	return result, truncated, nil
}

// Relationships returns at most limit Marksplice link relationships.
func (s *Snapshot) Relationships(limit int) ([]RelationshipSummary, bool, error) {
	if s == nil || s.document == nil || limit <= 0 {
		return nil, false, fmt.Errorf("%w: relationship limit must be positive", marksplice.ErrInvalidQuery)
	}
	values := s.document.LinkRelationships()
	truncated := len(values) > limit
	if truncated {
		values = values[:limit]
	}
	result := make([]RelationshipSummary, 0, len(values))
	for _, value := range values {
		result = append(result, s.relationshipSummary(value))
	}
	return result, truncated, nil
}

// ResolveFragment delegates fragment resolution entirely to Marksplice.
func (s *Snapshot) ResolveFragment(fragment string) (FragmentResolution, bool) {
	if s == nil || s.document == nil {
		return FragmentResolution{}, false
	}
	target, ok := s.document.ResolveFragment(fragment)
	if !ok {
		return FragmentResolution{}, false
	}
	targetID, ok := s.nodeTargetID(target.NodeID())
	if !ok {
		return FragmentResolution{}, false
	}
	return FragmentResolution{Kind: fragmentTargetKindName(target.Kind()), Value: target.Value(), TargetID: targetID}, true
}

// ValidateFragment delegates unique fragment validation to Marksplice.
func (s *Snapshot) ValidateFragment(fragment string) bool {
	return s != nil && s.document != nil && s.document.ValidateFragment(fragment)
}

// TOCStale delegates managed-TOC shape and staleness decisions to Marksplice.
func (s *Snapshot) TOCStale(headingTargetID string) (bool, bool, error) {
	resolved, ok, err := s.resolveNodeTarget(headingTargetID)
	if err != nil || !ok {
		return false, false, err
	}
	if resolved.node.Kind() != marksplice.KindHeading {
		return false, false, marksplice.ErrInvalidTargetKind
	}
	stale, recognized := s.document.TOCStale(resolved.node.ID())
	return stale, recognized, nil
}

// Generate returns explicit derived Markdown output. It is never used as an
// existing-document mutation path.
func (s *Snapshot) Generate(kind string) ([]byte, error) {
	if s == nil || s.document == nil {
		return nil, fmt.Errorf("%w: markdown snapshot is unavailable", marksplice.ErrInvalidQuery)
	}
	switch kind {
	case "toc":
		return append([]byte(nil), s.document.GenerateTOC()...), nil
	case "canonical_markdown":
		return s.document.CanonicalMarkdown()
	default:
		return nil, fmt.Errorf("%w: generation kind must be toc or canonical_markdown", marksplice.ErrInvalidQuery)
	}
}

type resolvedNodeTarget struct {
	node       marksplice.Node
	rangeValue marksplice.Range
}

func (s *Snapshot) resolveNodeTarget(target string) (resolvedNodeTarget, bool, error) {
	if s == nil || s.document == nil {
		return resolvedNodeTarget{}, false, fmt.Errorf("%w: markdown snapshot is unavailable", marksplice.ErrInvalidQuery)
	}
	if !validTargetID(target) {
		return resolvedNodeTarget{}, false, nil
	}
	s.nodeTargetIndexOnce.Do(func() {
		matches, err := s.document.QueryNodes(marksplice.NodeQuery{Limit: maxTargetScanNodes + 1})
		if err != nil {
			s.nodeTargetIndexErr = err
			return
		}
		if len(matches) > maxTargetScanNodes {
			s.nodeTargetIndexErr = fmt.Errorf("%w: markdown target scan exceeds %d nodes", marksplice.ErrInvalidQuery, maxTargetScanNodes)
			return
		}
		index := make(map[string]resolvedNodeTarget, len(matches))
		for _, match := range matches {
			node := match.Node()
			rangeValue := match.Range()
			key := targetID(s.fingerprint, kindName(node.Kind()), rangeValue)
			if _, exists := index[key]; !exists {
				index[key] = resolvedNodeTarget{node: node, rangeValue: rangeValue}
			}
		}
		s.nodeTargetIndex = index
	})
	if s.nodeTargetIndexErr != nil {
		return resolvedNodeTarget{}, false, s.nodeTargetIndexErr
	}
	resolved, ok := s.nodeTargetIndex[target]
	return resolved, ok, nil
}

func (s *Snapshot) queryNodesBounded(kinds []string, limit int) ([]NodeSummary, bool, error) {
	if limit <= 0 {
		return nil, false, fmt.Errorf("%w: node query limit must be positive", marksplice.ErrInvalidQuery)
	}
	markspliceKinds, err := parseKinds(kinds)
	if err != nil {
		return nil, false, err
	}
	matches, err := s.document.QueryNodes(marksplice.NodeQuery{Kinds: markspliceKinds, Limit: limit + 1})
	if err != nil {
		return nil, false, err
	}
	truncated := len(matches) > limit
	if truncated {
		matches = matches[:limit]
	}
	result := make([]NodeSummary, 0, len(matches))
	for _, match := range matches {
		result = append(result, s.summarize(match))
	}
	return result, truncated, nil
}

func (s *Snapshot) sectionSummary(section marksplice.Section) SectionSummary {
	headingTarget, _ := s.nodeTargetID(section.HeadingID())
	summary := SectionSummary{
		TargetID:        targetID(s.fingerprint, "section", section.Range()),
		HeadingTargetID: headingTarget,
		Level:           section.Level(),
		Range:           fromMarkspliceRange(section.Range()),
		BodyRange:       fromMarkspliceRange(section.BodyRange()),
	}
	if parent, ok := section.ParentHeadingID(); ok {
		summary.ParentHeadingTargetID, _ = s.nodeTargetID(parent)
	}
	return summary
}

func (s *Snapshot) relationshipSummary(value marksplice.LinkRelationship) RelationshipSummary {
	summary := RelationshipSummary{
		Kind:           relationshipKindName(value.Kind()),
		Destination:    value.Destination(),
		SourceOffset:   value.SourceOffset(),
		FragmentStatus: fragmentStatusName(value.FragmentStatus()),
		Email:          value.IsEmail(),
	}
	if title, ok := value.Title(); ok {
		summary.Title = title
	}
	if reference, form, ok := value.Reference(); ok {
		summary.Reference = reference
		summary.ReferenceForm = referenceFormName(form)
	}
	if sourceID, ok := value.SourceNodeID(); ok {
		summary.SourceTargetID, _ = s.nodeTargetID(sourceID)
	}
	if definitionID, ok := value.ReferenceDefinitionID(); ok {
		summary.ReferenceDefinitionTargetID, _ = s.nodeTargetID(definitionID)
	}
	if target, ok := value.FragmentTarget(); ok {
		summary.FragmentTargetID, _ = s.nodeTargetID(target.NodeID())
	}
	return summary
}

func (s *Snapshot) nodeTargetID(id marksplice.NodeID) (string, bool) {
	node, ok := s.document.Node(id)
	if !ok {
		return "", false
	}
	rangeValue, ok := s.nodeRange(node)
	if !ok {
		return "", false
	}
	return targetID(s.fingerprint, kindName(node.Kind()), rangeValue), true
}

func (s *Snapshot) nodeRange(node marksplice.Node) (marksplice.Range, bool) {
	id := node.ID()
	switch node.Kind() {
	case marksplice.KindParagraph:
		value, ok := s.document.Paragraph(id)
		if ok {
			return value.Range(), true
		}
	case marksplice.KindHeading:
		value, ok := s.document.Heading(id)
		if ok {
			return value.Range(), true
		}
	case marksplice.KindListItem:
		value, ok := s.document.ListItem(id)
		if ok {
			return value.Range(), true
		}
	case marksplice.KindTask:
		value, ok := s.document.Task(id)
		if ok {
			return value.Range(), true
		}
	case marksplice.KindTableCell:
		value, ok := s.document.TableCell(id)
		if ok {
			return value.Range(), true
		}
	case marksplice.KindFencedCode:
		value, ok := s.document.FencedCode(id)
		if ok {
			return value.Range(), true
		}
	case marksplice.KindStrikethrough:
		value, ok := s.document.Strikethrough(id)
		if ok {
			return value.Range(), true
		}
	case marksplice.KindCodeSpan:
		value, ok := s.document.CodeSpan(id)
		if ok {
			return value.Range(), true
		}
	case marksplice.KindEmphasis:
		value, ok := s.document.Emphasis(id)
		if ok {
			return value.Range(), true
		}
	case marksplice.KindStrong:
		value, ok := s.document.Strong(id)
		if ok {
			return value.Range(), true
		}
	case marksplice.KindInlineLink:
		value, ok := s.document.InlineLink(id)
		if ok {
			return value.Range(), true
		}
	case marksplice.KindReferenceDefinition:
		value, ok := s.document.ReferenceDefinition(id)
		if ok {
			return value.Range(), true
		}
	case marksplice.KindAutoLink:
		value, ok := s.document.AutoLink(id)
		if ok {
			return value.Range(), true
		}
	case marksplice.KindFrontMatterField:
		value, ok := s.document.FrontMatterField(id)
		if ok {
			return value.Range(), true
		}
	case marksplice.KindHTMLComment:
		value, ok := s.document.HTMLComment(id)
		if ok {
			return value.Range(), true
		}
	case marksplice.KindHTMLAnchor:
		value, ok := s.document.HTMLAnchor(id)
		if ok {
			return value.Range(), true
		}
	case marksplice.KindImage:
		value, ok := s.document.Image(id)
		if ok {
			return value.Range(), true
		}
	case marksplice.KindTableRow:
		value, ok := s.document.TableRow(id)
		if ok {
			return value.Range(), true
		}
	case marksplice.KindTable:
		value, ok := s.document.Table(id)
		if ok {
			return value.Range(), true
		}
	case marksplice.KindThematicBreak:
		value, ok := s.document.ThematicBreak(id)
		if ok {
			return value.Range(), true
		}
	case marksplice.KindBlockquote:
		value, ok := s.document.Blockquote(id)
		if ok {
			return value.Range(), true
		}
	case marksplice.KindFootnoteDefinition:
		value, ok := s.document.FootnoteDefinition(id)
		if ok {
			return value.Range(), true
		}
	case marksplice.KindMathExpression:
		value, ok := s.document.MathExpression(id)
		if ok {
			return value.Range(), true
		}
	}
	return marksplice.Range{}, false
}

func fromMarkspliceRange(value marksplice.Range) Range {
	return Range{Start: value.Start, End: value.End}
}

func relationshipKindName(kind marksplice.LinkRelationshipKind) string {
	switch kind {
	case marksplice.LinkRelationshipInlineLink:
		return "inline_link"
	case marksplice.LinkRelationshipReferenceLink:
		return "reference_link"
	case marksplice.LinkRelationshipInlineImage:
		return "inline_image"
	case marksplice.LinkRelationshipReferenceImage:
		return "reference_image"
	case marksplice.LinkRelationshipAutoLink:
		return "autolink"
	default:
		return "unknown"
	}
}

func fragmentStatusName(status marksplice.LinkFragmentStatus) string {
	switch status {
	case marksplice.LinkFragmentNotApplicable:
		return "not_applicable"
	case marksplice.LinkFragmentResolved:
		return "resolved"
	case marksplice.LinkFragmentMissing:
		return "missing"
	case marksplice.LinkFragmentAmbiguous:
		return "ambiguous"
	case marksplice.LinkFragmentInvalid:
		return "invalid"
	default:
		return "unknown"
	}
}

func referenceFormName(form marksplice.ReferenceForm) string {
	switch form {
	case marksplice.ReferenceFormFull:
		return "full"
	case marksplice.ReferenceFormCollapsed:
		return "collapsed"
	case marksplice.ReferenceFormShortcut:
		return "shortcut"
	default:
		return "unknown"
	}
}

func fragmentTargetKindName(kind marksplice.FragmentTargetKind) string {
	switch kind {
	case marksplice.FragmentTargetHeading:
		return "heading"
	case marksplice.FragmentTargetHTMLAnchor:
		return "html_anchor"
	default:
		return "unknown"
	}
}
