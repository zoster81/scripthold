package sourceintelligence

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/zoster81/scripthold/internal/operation"
)

type tclWordForm uint8

const (
	tclWordRaw tclWordForm = iota
	tclWordQuoted
	tclWordBraced
)

type tclWord struct {
	form                     tclWordForm
	start, end               int
	contentStart, contentEnd int
	static                   bool
	value                    string
}

type tclSourceParser struct {
	state            *phase8State
	text             string
	maxNesting       int
	nextContextCheck int
}

func newTclSourceParser(state *phase8State, maxNesting int) *tclSourceParser {
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	return &tclSourceParser{state: state, text: state.document.Text, maxNesting: maxNesting}
}

func (p *tclSourceParser) parseScript(start, end, depth int, parent *SymbolParent, stopAtBracket, collect bool) (int, bool, error) {
	if depth > p.maxNesting {
		return start, false, operation.Wrap(operation.KindLimit, "analyze_tcl_source", p.state.document.Path, fmt.Errorf("tcl substitution nesting %d exceeds limit %d", depth, p.maxNesting))
	}
	at := start
	for at < end && !p.state.stopped {
		if err := p.checkContext(at); err != nil {
			return at, false, err
		}
		at = p.skipWordSeparators(at, end)
		if at >= end {
			break
		}
		if stopAtBracket && p.text[at] == ']' {
			return at + 1, true, nil
		}
		if isTclCommandSeparator(p.text[at]) {
			at = consumeTclCommandSeparator(p.text, at, end)
			continue
		}
		if p.text[at] == '#' {
			at = skipTclComment(p.text, at, end)
			continue
		}

		commandStart := at
		words := make([]tclWord, 0, 8)
		commandEnd := at
		for at < end {
			if err := p.checkContext(at); err != nil {
				return at, false, err
			}
			at = p.skipWordSeparators(at, end)
			if at >= end {
				commandEnd = at
				break
			}
			if stopAtBracket && p.text[at] == ']' {
				commandEnd = at
				if collect {
					if err := p.processCommand(commandStart, commandEnd, words, depth, parent); err != nil {
						return at, false, err
					}
				}
				return at + 1, true, nil
			}
			if isTclCommandSeparator(p.text[at]) {
				commandEnd = at
				if collect {
					if err := p.processCommand(commandStart, commandEnd, words, depth, parent); err != nil {
						return at, false, err
					}
				}
				at = consumeTclCommandSeparator(p.text, at, end)
				words = nil
				break
			}

			word, next, complete, err := p.parseWord(at, end, depth, parent, collect, stopAtBracket)
			if err != nil {
				return at, false, err
			}
			words = append(words, word)
			at = next
			commandEnd = at
			if !complete {
				if collect {
					_ = p.processCommand(commandStart, commandEnd, words, depth, parent)
				}
				return at, false, nil
			}
			if (word.form == tclWordBraced || word.form == tclWordQuoted) && at < end && !isTclWordBoundary(p.text[at], stopAtBracket) {
				p.markIncomplete("tcl-extra-characters-after-grouped-word", "characters follow a braced or quoted Tcl word without a separator", at, min(at+1, end))
				return at, false, nil
			}
		}
		if len(words) > 0 && collect {
			if err := p.processCommand(commandStart, commandEnd, words, depth, parent); err != nil {
				return at, false, err
			}
		}
	}
	return at, !stopAtBracket, nil
}

func (p *tclSourceParser) parseWord(start, end, depth int, parent *SymbolParent, collect, stopAtBracket bool) (tclWord, int, bool, error) {
	if start >= end {
		return tclWord{}, start, true, nil
	}
	if p.text[start] == '{' && !tclArgumentExpansionPrefix(p.text, start, end) {
		return p.parseBracedWord(start, end, depth)
	}
	if p.text[start] == '"' {
		return p.parseQuotedWord(start, end, depth, parent, collect)
	}
	return p.parseRawWord(start, end, depth, parent, collect, stopAtBracket)
}

func (p *tclSourceParser) parseBracedWord(start, end, depth int) (tclWord, int, bool, error) {
	nesting := 1
	at := start + 1
	for at < end {
		if err := p.checkContext(at); err != nil {
			return tclWord{}, at, false, err
		}
		if p.text[at] == '\\' && at+1 < end {
			at = consumeTclBackslashSequence(p.text, at, end)
			continue
		}
		switch p.text[at] {
		case '{':
			nesting++
			if depth+nesting > p.maxNesting {
				return tclWord{}, at, false, operation.Wrap(operation.KindLimit, "analyze_tcl_source", p.state.document.Path, fmt.Errorf("tcl braced-word nesting exceeds limit %d", p.maxNesting))
			}
			at++
		case '}':
			nesting--
			at++
			if nesting == 0 {
				contentEnd := at - 1
				return tclWord{form: tclWordBraced, start: start, end: at, contentStart: start + 1, contentEnd: contentEnd, static: true, value: p.text[start+1 : contentEnd]}, at, true, nil
			}
		default:
			_, size := utf8.DecodeRuneInString(p.text[at:end])
			at += max(size, 1)
		}
	}
	p.markIncomplete("tcl-unterminated-braced-word", "braced Tcl word has no matching close brace", start, min(start+1, end))
	return tclWord{form: tclWordBraced, start: start, end: end, contentStart: min(start+1, end), contentEnd: end}, end, false, nil
}

func (p *tclSourceParser) parseQuotedWord(start, end, depth int, parent *SymbolParent, collect bool) (tclWord, int, bool, error) {
	at := start + 1
	static := true
	for at < end {
		if err := p.checkContext(at); err != nil {
			return tclWord{}, at, false, err
		}
		switch p.text[at] {
		case '\\':
			static = false
			at = consumeTclBackslashSequence(p.text, at, end)
		case '$':
			static = false
			next, complete := consumeTclVariableReference(p.text, at, end)
			if !complete {
				p.markIncomplete("tcl-unterminated-variable-reference", "braced Tcl variable reference has no matching close brace", at, min(at+2, end))
				return tclWord{form: tclWordQuoted, start: start, end: end, contentStart: start + 1, contentEnd: end}, end, false, nil
			}
			at = next
		case '[':
			static = false
			next, closed, err := p.parseScript(at+1, end, depth+1, parent, true, collect)
			if err != nil {
				return tclWord{}, at, false, err
			}
			if !closed {
				p.markIncomplete("tcl-unterminated-command-substitution", "Tcl command substitution has no matching close bracket", at, min(at+1, end))
				return tclWord{form: tclWordQuoted, start: start, end: end, contentStart: start + 1, contentEnd: end}, end, false, nil
			}
			at = next
		case '"':
			contentEnd := at
			at++
			word := tclWord{form: tclWordQuoted, start: start, end: at, contentStart: start + 1, contentEnd: contentEnd, static: static}
			if static {
				word.value = p.text[word.contentStart:word.contentEnd]
			}
			return word, at, true, nil
		default:
			_, size := utf8.DecodeRuneInString(p.text[at:end])
			at += max(size, 1)
		}
	}
	p.markIncomplete("tcl-unterminated-string", "quoted Tcl word has no matching double quote", start, min(start+1, end))
	return tclWord{form: tclWordQuoted, start: start, end: end, contentStart: min(start+1, end), contentEnd: end}, end, false, nil
}

func (p *tclSourceParser) parseRawWord(start, end, depth int, parent *SymbolParent, collect, stopAtBracket bool) (tclWord, int, bool, error) {
	at := start
	static := !tclArgumentExpansionPrefix(p.text, start, end)
	for at < end {
		if err := p.checkContext(at); err != nil {
			return tclWord{}, at, false, err
		}
		if isHorizontalSpace(p.text[at]) || isTclCommandSeparator(p.text[at]) || stopAtBracket && p.text[at] == ']' {
			break
		}
		if p.text[at] == '\\' {
			if at+1 < end && isNewlineStart(p.text[at+1]) {
				break
			}
			static = false
			at = consumeTclBackslashSequence(p.text, at, end)
			continue
		}
		switch p.text[at] {
		case '$':
			static = false
			next, complete := consumeTclVariableReference(p.text, at, end)
			if !complete {
				p.markIncomplete("tcl-unterminated-variable-reference", "braced Tcl variable reference has no matching close brace", at, min(at+2, end))
				return tclWord{form: tclWordRaw, start: start, end: end, contentStart: start, contentEnd: end}, end, false, nil
			}
			at = next
		case '[':
			static = false
			next, closed, err := p.parseScript(at+1, end, depth+1, parent, true, collect)
			if err != nil {
				return tclWord{}, at, false, err
			}
			if !closed {
				p.markIncomplete("tcl-unterminated-command-substitution", "Tcl command substitution has no matching close bracket", at, min(at+1, end))
				return tclWord{form: tclWordRaw, start: start, end: end, contentStart: start, contentEnd: end}, end, false, nil
			}
			at = next
		default:
			_, size := utf8.DecodeRuneInString(p.text[at:end])
			at += max(size, 1)
		}
	}
	word := tclWord{form: tclWordRaw, start: start, end: at, contentStart: start, contentEnd: at, static: static}
	if static {
		word.value = p.text[start:at]
	}
	if at < end && p.text[at] == '\\' && at+1 < end && isNewlineStart(p.text[at+1]) {
		at = consumeTclBackslashSequence(p.text, at, end)
	}
	return word, at, true, nil
}

func (p *tclSourceParser) processCommand(start, end int, words []tclWord, depth int, parent *SymbolParent) error {
	if len(words) == 0 || p.state.stopped || !words[0].static {
		return nil
	}
	command := strings.ToLower(words[0].value)
	switch command {
	case "proc":
		if len(words) != 4 || !words[1].static || words[1].value == "" {
			return nil
		}
		name := words[1]
		body := words[3]
		p.state.add(SymbolSpec{
			Kind: SymbolKindFunction, NativeKind: "proc", Name: name.value, Parent: parent,
			Declaration: OffsetRange{Start: start, End: max(end, body.end)},
			NameRange:   OffsetRange{Start: name.contentStart, End: name.contentEnd},
			Signature:   &OffsetRange{Start: start, End: body.start},
			Body:        &OffsetRange{Start: body.start, End: body.end},
			Evidence:    SymbolEvidenceStructural,
		})
	case "namespace":
		if len(words) != 4 || !words[1].static || !strings.EqualFold(words[1].value, "eval") || !words[2].static || words[2].value == "" || words[3].form != tclWordBraced {
			return nil
		}
		name := words[2]
		body := words[3]
		symbol, ok := p.state.add(SymbolSpec{
			Kind: SymbolKindNamespace, NativeKind: "namespace-eval", Name: name.value, Parent: parent,
			Declaration: OffsetRange{Start: start, End: max(end, body.end)},
			NameRange:   OffsetRange{Start: name.contentStart, End: name.contentEnd},
			Signature:   &OffsetRange{Start: start, End: body.start},
			Body:        &OffsetRange{Start: body.start, End: body.end},
			Evidence:    SymbolEvidenceStructural,
		})
		if ok && !p.state.stopped {
			child := &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
			_, _, err := p.parseScript(body.contentStart, body.contentEnd, depth+1, child, false, true)
			return err
		}
	case "source":
		if len(words) >= 2 && words[1].static {
			p.state.addImportDependency(words[1].value, words[1].contentStart, words[1].contentEnd)
		}
	case "package":
		if len(words) >= 3 && words[1].static && strings.EqualFold(words[1].value, "require") && words[2].static {
			p.state.addImportDependency(words[2].value, words[2].contentStart, words[2].contentEnd)
		}
	}
	return nil
}

func (p *tclSourceParser) skipWordSeparators(at, end int) int {
	for at < end {
		if isHorizontalSpace(p.text[at]) {
			at++
			continue
		}
		if p.text[at] == '\\' && at+1 < end && isNewlineStart(p.text[at+1]) {
			at = consumeTclBackslashSequence(p.text, at, end)
			continue
		}
		break
	}
	return at
}

func (p *tclSourceParser) checkContext(offset int) error {
	due, next := tclContextCheckDue(p.nextContextCheck, offset)
	p.nextContextCheck = next
	if !due {
		return nil
	}
	if err := p.state.ctx.Err(); err != nil {
		return operation.Wrap(operation.KindCancelled, "analyze_tcl_source", p.state.document.Path, err)
	}
	return nil
}

func tclContextCheckDue(next, offset int) (bool, int) {
	if offset < next {
		return false, next
	}
	return true, offset + 4096
}

func (p *tclSourceParser) markIncomplete(code, message string, start, end int) {
	p.state.builder.MarkIncomplete()
	value := OffsetRange{Start: max(start, 0), End: min(max(end, start+1), len(p.text))}
	_ = p.state.builder.AddDiagnostic(DiagnosticSpec{Code: code, Message: message, Severity: DiagnosticWarning, Range: &value, AffectsCoverage: true})
}

func isTclCommandSeparator(value byte) bool {
	return value == '\r' || value == '\n' || value == ';'
}

func consumeTclCommandSeparator(text string, at, end int) int {
	if at < end && text[at] == '\r' && at+1 < end && text[at+1] == '\n' {
		return at + 2
	}
	return min(at+1, end)
}

func isTclWordBoundary(value byte, stopAtBracket bool) bool {
	return isHorizontalSpace(value) || isTclCommandSeparator(value) || stopAtBracket && value == ']'
}

func skipTclComment(text string, at, end int) int {
	for at < end && !isNewlineStart(text[at]) {
		at++
	}
	return at
}

func consumeTclBackslashSequence(text string, at, end int) int {
	if at >= end || text[at] != '\\' {
		return at
	}
	at++
	if at >= end {
		return at
	}
	if text[at] == '\r' && at+1 < end && text[at+1] == '\n' {
		at += 2
	} else if isNewlineStart(text[at]) {
		at++
	} else {
		_, size := utf8.DecodeRuneInString(text[at:end])
		return at + max(size, 1)
	}
	for at < end && (text[at] == ' ' || text[at] == '\t') {
		at++
	}
	return at
}

func consumeTclVariableReference(text string, at, end int) (int, bool) {
	if at >= end || text[at] != '$' {
		return at, true
	}
	at++
	if at >= end {
		return at, true
	}
	if text[at] == '{' {
		at++
		for at < end {
			if text[at] == '\\' && at+1 < end {
				at = consumeTclBackslashSequence(text, at, end)
				continue
			}
			if text[at] == '}' {
				return at + 1, true
			}
			_, size := utf8.DecodeRuneInString(text[at:end])
			at += max(size, 1)
		}
		return at, false
	}
	for at < end {
		r, size := utf8.DecodeRuneInString(text[at:end])
		if !(r == '_' || r == ':' || unicodeTclVariableRune(r)) {
			break
		}
		at += max(size, 1)
	}
	return at, true
}

func unicodeTclVariableRune(value rune) bool {
	return value >= utf8.RuneSelf || value >= '0' && value <= '9' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func tclArgumentExpansionPrefix(text string, start, end int) bool {
	return start+3 < end && text[start:start+3] == "{*}" && !isTclWordBoundary(text[start+3], false)
}
