package sourceintelligence

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/zoster81/scripthold/internal/operation"
)

type matchedDelimiter struct {
	open bool
	text string
	rule DelimiterRule
}

type pendingHereDoc struct {
	delimiter        string
	stripLeadingTabs bool
	openingOffset    int
}

func normalizeScannerProfile(profile ScannerProfile) ScannerProfile {
	if profile.Identifier == (IdentifierPolicy{}) {
		profile.Identifier = DefaultIdentifierPolicy()
	}
	if len(profile.Delimiters) == 0 {
		profile.Delimiters = defaultDelimiterRules()
	} else {
		profile.Delimiters = append([]DelimiterRule(nil), profile.Delimiters...)
	}
	if profile.Directives && len(profile.DirectiveRules) == 0 {
		profile.DirectiveRules = []DirectiveRule{{Prefix: "#"}}
	} else {
		profile.DirectiveRules = append([]DirectiveRule(nil), profile.DirectiveRules...)
	}
	profile.HereDocs = append([]HereDocRule(nil), profile.HereDocs...)
	return profile
}

func defaultDelimiterRules() []DelimiterRule {
	return []DelimiterRule{{Open: "(", Close: ")"}, {Open: "[", Close: "]"}, {Open: "{", Close: "}"}}
}

func (scanner *sourceScanner) validateR27Profile() error {
	if len(scanner.profile.Delimiters) > 32 || len(scanner.profile.DirectiveRules) > 32 || len(scanner.profile.HereDocs) > 16 {
		return operation.New(operation.KindInvalidInput, "scanner profile contains too many lexical rules")
	}
	seenDelimiter := make(map[string]struct{}, len(scanner.profile.Delimiters)*2)
	for _, rule := range scanner.profile.Delimiters {
		if rule.Open == "" || rule.Close == "" || rule.Open == rule.Close {
			return operation.New(operation.KindInvalidInput, "delimiter pairs require distinct non-empty open and close values")
		}
		if !utf8.ValidString(rule.Open) || !utf8.ValidString(rule.Close) {
			return operation.New(operation.KindInvalidInput, "delimiter pairs must be valid UTF-8")
		}
		for _, value := range []string{rule.Open, rule.Close} {
			if _, duplicate := seenDelimiter[value]; duplicate {
				return operation.New(operation.KindInvalidInput, "delimiter spellings must be unique within one scanner profile")
			}
			seenDelimiter[value] = struct{}{}
		}
	}
	seenDirective := make(map[string]struct{}, len(scanner.profile.DirectiveRules))
	for _, rule := range scanner.profile.DirectiveRules {
		if strings.TrimSpace(rule.Prefix) == "" || !utf8.ValidString(rule.Prefix) {
			return operation.New(operation.KindInvalidInput, "directive prefixes must be non-empty valid UTF-8")
		}
		key := rule.Prefix
		if rule.CaseInsensitive {
			key = strings.ToLower(key)
		}
		if _, duplicate := seenDirective[key]; duplicate {
			return operation.New(operation.KindInvalidInput, "directive prefixes must be unique")
		}
		seenDirective[key] = struct{}{}
	}
	seenHereDoc := make(map[string]struct{}, len(scanner.profile.HereDocs))
	for _, rule := range scanner.profile.HereDocs {
		if strings.TrimSpace(rule.Operator) == "" || strings.ContainsAny(rule.Operator, "\r\n\t ") {
			return operation.New(operation.KindInvalidInput, "heredoc operators must be non-empty single-token values")
		}
		if _, duplicate := seenHereDoc[rule.Operator]; duplicate {
			return operation.New(operation.KindInvalidInput, "heredoc operators must be unique")
		}
		seenHereDoc[rule.Operator] = struct{}{}
	}
	return nil
}

func (scanner *sourceScanner) identifierStart(value rune) bool {
	policy := scanner.profile.Identifier
	return policy.Underscore && value == '_' ||
		policy.UnicodeLetters && unicode.IsLetter(value) ||
		strings.ContainsRune(policy.ExtraStart, value)
}

func (scanner *sourceScanner) identifierContinue(value rune) bool {
	policy := scanner.profile.Identifier
	if scanner.identifierStart(value) {
		return true
	}
	return policy.UnicodeDigits && unicode.IsDigit(value) ||
		policy.UnicodeMarks && (unicode.IsMark(value) || unicode.In(value, unicode.Pc)) ||
		strings.ContainsRune(policy.ExtraContinue, value)
}

func identifierStart(policy IdentifierPolicy, value rune) bool {
	if policy == (IdentifierPolicy{}) {
		policy = DefaultIdentifierPolicy()
	}
	return policy.Underscore && value == '_' || policy.UnicodeLetters && unicode.IsLetter(value) || strings.ContainsRune(policy.ExtraStart, value)
}

func identifierContinue(policy IdentifierPolicy, value rune) bool {
	if policy == (IdentifierPolicy{}) {
		policy = DefaultIdentifierPolicy()
	}
	return identifierStart(policy, value) || policy.UnicodeDigits && unicode.IsDigit(value) || policy.UnicodeMarks && (unicode.IsMark(value) || unicode.In(value, unicode.Pc)) || strings.ContainsRune(policy.ExtraContinue, value)
}

func (scanner *sourceScanner) directiveRuleAt(offset int) (DirectiveRule, bool) {
	var best DirectiveRule
	found := false
	for _, rule := range scanner.profile.DirectiveRules {
		if len(rule.Prefix) <= len(best.Prefix) {
			continue
		}
		if stringPrefixAt(scanner.text, offset, rule.Prefix, rule.CaseInsensitive) {
			best = rule
			found = true
		}
	}
	return best, found
}

func stringPrefixAt(text string, offset int, prefix string, insensitive bool) bool {
	if offset < 0 || offset+len(prefix) > len(text) {
		return false
	}
	actual := text[offset : offset+len(prefix)]
	if insensitive {
		return strings.EqualFold(actual, prefix)
	}
	return actual == prefix
}

func (scanner *sourceScanner) delimiterAt(offset int) (matchedDelimiter, bool) {
	if len(scanner.delimiters) > 0 {
		top := scanner.delimiters[len(scanner.delimiters)-1]
		if strings.HasPrefix(scanner.text[offset:], top.close) {
			return matchedDelimiter{open: false, text: top.close, rule: DelimiterRule{Open: top.open, Close: top.close}}, true
		}
	}
	var best matchedDelimiter
	for _, rule := range scanner.profile.Delimiters {
		for _, candidate := range []matchedDelimiter{{open: true, text: rule.Open, rule: rule}, {open: false, text: rule.Close, rule: rule}} {
			if len(candidate.text) <= len(best.text) || !strings.HasPrefix(scanner.text[offset:], candidate.text) {
				continue
			}
			best = candidate
		}
	}
	return best, best.text != ""
}

func (scanner *sourceScanner) scanDelimiter(match matchedDelimiter) error {
	start := scanner.at
	scanner.at += len(match.text)
	if match.open {
		if len(scanner.delimiters)+1 > scanner.limits.MaxNesting {
			return scanner.limitError("delimiter nesting", len(scanner.delimiters)+1, scanner.limits.MaxNesting)
		}
		scanner.delimiters = append(scanner.delimiters, delimiterEntry{open: match.rule.Open, close: match.rule.Close, offset: start})
		if len(scanner.delimiters) > scanner.result.MaxDepth {
			scanner.result.MaxDepth = len(scanner.delimiters)
		}
	} else if len(scanner.delimiters) == 0 {
		scanner.addDiagnostic("mismatched-delimiter", fmt.Sprintf("closing delimiter %q has no opener", match.text), start, scanner.at)
	} else {
		top := scanner.delimiters[len(scanner.delimiters)-1]
		if top.close == match.text {
			scanner.delimiters = scanner.delimiters[:len(scanner.delimiters)-1]
		} else {
			scanner.addDiagnostic("mismatched-delimiter", fmt.Sprintf("closing delimiter %q does not match %q", match.text, top.open), start, scanner.at)
		}
	}
	scanner.lineStart = false
	return scanner.emit(TokenPunctuation, start, scanner.at)
}

func PairDelimiterTokens(tokens []Token, rules []DelimiterRule) map[int]int {
	if len(rules) == 0 {
		rules = defaultDelimiterRules()
	}
	openToClose := make(map[string]string, len(rules))
	closeSet := make(map[string]struct{}, len(rules))
	for _, rule := range rules {
		if rule.Open == "" || rule.Close == "" || rule.Open == rule.Close {
			continue
		}
		openToClose[rule.Open] = rule.Close
		closeSet[rule.Close] = struct{}{}
	}
	type entry struct {
		index int
		close string
	}
	var stack []entry
	pairs := make(map[int]int)
	for index, token := range tokens {
		if close, ok := openToClose[token.Text]; ok {
			stack = append(stack, entry{index: index, close: close})
			continue
		}
		if _, closeToken := closeSet[token.Text]; !closeToken || len(stack) == 0 {
			continue
		}
		top := stack[len(stack)-1]
		if top.close != token.Text {
			continue
		}
		stack = stack[:len(stack)-1]
		pairs[top.index] = index
		pairs[index] = top.index
	}
	return pairs
}

func (scanner *sourceScanner) hereDocOpeningAt(offset int) (HereDocRule, int, bool) {
	var best HereDocRule
	for _, rule := range scanner.profile.HereDocs {
		if len(rule.Operator) <= len(best.Operator) || !strings.HasPrefix(scanner.text[offset:], rule.Operator) {
			continue
		}
		best = rule
	}
	if best.Operator == "" {
		return HereDocRule{}, 0, false
	}
	return best, len(best.Operator), true
}

func (scanner *sourceScanner) scanHereDocOpening(rule HereDocRule, operatorBytes int) error {
	start := scanner.at
	delimiter, ok := parseHereDocDelimiter(scanner.text, scanner.at+operatorBytes, rule.AllowQuotedDelimiter)
	if !ok {
		scanner.at += operatorBytes
		scanner.lineStart = false
		return scanner.emit(TokenOperator, start, scanner.at)
	}
	if len(scanner.pendingHereDocs)+1 > scanner.limits.MaxNesting {
		return scanner.limitError("pending heredocs", len(scanner.pendingHereDocs)+1, scanner.limits.MaxNesting)
	}
	scanner.pendingHereDocs = append(scanner.pendingHereDocs, pendingHereDoc{delimiter: delimiter, stripLeadingTabs: rule.StripLeadingTabs, openingOffset: start})
	scanner.at += operatorBytes
	scanner.lineStart = false
	return scanner.emit(TokenOperator, start, scanner.at)
}

func parseHereDocDelimiter(text string, offset int, allowQuoted bool) (string, bool) {
	for offset < len(text) && isHorizontalSpace(text[offset]) {
		offset++
	}
	if offset >= len(text) || isNewlineStart(text[offset]) {
		return "", false
	}
	if allowQuoted && (text[offset] == '\'' || text[offset] == '"') {
		quote := text[offset]
		end := strings.IndexByte(text[offset+1:], quote)
		if end < 0 {
			return "", false
		}
		value := text[offset+1 : offset+1+end]
		return value, value != ""
	}
	start := offset
	for offset < len(text) && !isHorizontalSpace(text[offset]) && !isNewlineStart(text[offset]) && !strings.ContainsRune(";|&()<>", rune(text[offset])) {
		offset++
	}
	if offset == start {
		return "", false
	}
	return text[start:offset], true
}

func (scanner *sourceScanner) consumePendingHereDocs() error {
	pending := scanner.pendingHereDocs
	scanner.pendingHereDocs = nil
	for pendingIndex, heredoc := range pending {
		bodyStart := scanner.at
		terminated := false
		for scanner.at < len(scanner.text) {
			if scanner.at&4095 == 0 {
				if err := scanner.checkContext(); err != nil {
					return err
				}
			}
			lineStart := scanner.at
			lineEnd := lineStart
			for lineEnd < len(scanner.text) && !isNewlineStart(scanner.text[lineEnd]) {
				lineEnd++
			}
			candidateStart := lineStart
			if heredoc.stripLeadingTabs {
				for candidateStart < lineEnd && scanner.text[candidateStart] == '\t' {
					candidateStart++
				}
			}
			if scanner.text[candidateStart:lineEnd] == heredoc.delimiter {
				if bodyStart < lineStart {
					if err := scanner.emit(TokenHereDoc, bodyStart, lineStart); err != nil {
						return err
					}
				} else {
					if err := scanner.emitSynthetic(TokenHereDoc, bodyStart, len(scanner.delimiters)); err != nil {
						return err
					}
				}
				scanner.at = lineEnd
				if scanner.at < len(scanner.text) {
					if scanner.text[scanner.at] == '\r' && scanner.at+1 < len(scanner.text) && scanner.text[scanner.at+1] == '\n' {
						scanner.at += 2
					} else {
						scanner.at++
					}
				}
				scanner.lineStart = true
				terminated = true
				break
			}
			scanner.at = lineEnd
			if scanner.at < len(scanner.text) {
				if scanner.text[scanner.at] == '\r' && scanner.at+1 < len(scanner.text) && scanner.text[scanner.at+1] == '\n' {
					scanner.at += 2
				} else {
					scanner.at++
				}
			}
		}
		if !terminated {
			scanner.addDiagnostic("unterminated-heredoc", fmt.Sprintf("heredoc %q has no body terminator", heredoc.delimiter), heredoc.openingOffset, len(scanner.text))
			for _, unresolved := range pending[pendingIndex+1:] {
				scanner.addDiagnostic("unterminated-heredoc", fmt.Sprintf("heredoc %q cannot begin because a prior heredoc is unterminated", unresolved.delimiter), unresolved.openingOffset, len(scanner.text))
			}
			if bodyStart < len(scanner.text) {
				if err := scanner.emit(TokenHereDoc, bodyStart, len(scanner.text)); err != nil {
					return err
				}
			}
			return nil
		}
	}
	return nil
}
