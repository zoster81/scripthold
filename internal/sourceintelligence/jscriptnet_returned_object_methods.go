package sourceintelligence

// A returned JScript.NET object literal is a lexical container, not a
// declaration of the variable that eventually receives its runtime value.
// Only direct identifier-valued object members of the form
//
//	Member: function (...) { ... }
//
// are indexed. In particular, this never derives "Package.IconLink" from an
// immediately invoked function expression or follows dynamic assignments.
func (parser *ecmaParser) collectJScriptNetReturnedObjectMethods() {
	for i := 0; i+1 < len(parser.tokens) && !parser.stopped; i++ {
		if parser.ctx.Err() != nil {
			return
		}
		if parser.tokens[i].Text != "return" || parser.tokens[i+1].Text != "{" {
			continue
		}
		open := i + 1
		close, ok := parser.pairs[open]
		if !ok || close <= open {
			parser.builder.MarkIncomplete()
			continue
		}
		parser.collectJScriptNetLiteralMembers(open, close, nil, 0)
	}
}

func (parser *ecmaParser) collectJScriptNetLiteralMembers(open, close int, lexicalOwner *SymbolParent, depth int) {
	if depth > 32 {
		parser.builder.MarkIncomplete()
		return
	}
	// Opening delimiters are emitted after nesting is incremented; direct
	// property keys have the same scanner depth as their opening brace.
	level := parser.tokens[open].Nesting
	for i := open + 1; i < close && !parser.stopped; {
		if parser.ctx.Err() != nil {
			return
		}
		key := parser.tokens[i]
		if key.Nesting != level || key.Kind != TokenIdentifier {
			i++
			continue
		}
		colon := parser.skipTrivia(i+1, close)
		if colon >= close || parser.tokens[colon].Text != ":" {
			i++
			continue
		}
		value := parser.skipTrivia(colon+1, close)
		if value >= close {
			return
		}
		if parser.tokens[value].Text == "{" {
			end, paired := parser.pairs[value]
			if paired && end > value && end <= close {
				body := OffsetRange{Start: parser.tokens[value].StartOffset, End: parser.tokens[end].EndOffset}
				object, added := parser.add(SymbolSpec{
					Kind: SymbolKindNamespace, NativeKind: "returned-object", Name: key.Text,
					Parent:      lexicalOwner,
					Declaration: OffsetRange{Start: key.StartOffset, End: parser.tokens[end].EndOffset},
					NameRange:   OffsetRange{Start: key.StartOffset, End: key.EndOffset},
					Body:        &body, Evidence: SymbolEvidenceStructural,
					Disambiguator: "returned-object-literal",
				})
				if added {
					nestedOwner := &SymbolParent{ID: object.ID, QualifiedName: object.QualifiedName}
					parser.collectJScriptNetLiteralMembers(value, end, nestedOwner, depth+1)
				}
				i = end + 1
				continue
			}
			parser.builder.MarkIncomplete()
			return
		}
		if lexicalOwner != nil && parser.tokens[value].Text == "function" {
			paren := parser.skipTrivia(value+1, close)
			if paren < close && parser.tokens[paren].Text == "(" {
				closeParen, paired := parser.pairs[paren]
				if paired && closeParen > paren && closeParen < close {
					bodyOpen := parser.skipTrivia(closeParen+1, close)
					if bodyOpen < close && parser.tokens[bodyOpen].Text == "{" {
						bodyClose, bodyPaired := parser.pairs[bodyOpen]
						if bodyPaired && bodyClose > bodyOpen && bodyClose < close {
							body := OffsetRange{
								Start: parser.tokens[bodyOpen].StartOffset,
								End:   parser.tokens[bodyClose].EndOffset,
							}
							parser.add(SymbolSpec{
								Kind:       SymbolKindMethod,
								NativeKind: "object-function",
								Name:       key.Text,
								Parent:     lexicalOwner,
								RegionID:   "jscript-net-returned-object",
								Declaration: OffsetRange{
									Start: key.StartOffset, End: parser.tokens[bodyClose].EndOffset,
								},
								NameRange: OffsetRange{Start: key.StartOffset, End: key.EndOffset},
								Signature: &OffsetRange{
									Start: key.StartOffset, End: parser.tokens[bodyOpen].StartOffset,
								},
								Body:          &body,
								Evidence:      SymbolEvidenceStructural,
								Disambiguator: "returned-object-function",
							})
							i = bodyClose + 1
							continue
						}
					}
				}
			}
		}
		i++
	}
}
