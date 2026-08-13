package sourceintelligence

import (
	"context"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/zoster81/scripthold/internal/operation"
)

const ScannerMaxDiagnostics = 64

// TokenKind is a language-neutral lexical category.
type TokenKind string

const (
	TokenIdentifier  TokenKind = "identifier"
	TokenKeyword     TokenKind = "keyword"
	TokenNumber      TokenKind = "number"
	TokenString      TokenKind = "string"
	TokenOperator    TokenKind = "operator"
	TokenPunctuation TokenKind = "punctuation"
	TokenNewline     TokenKind = "newline"
	TokenIndent      TokenKind = "indent"
	TokenDedent      TokenKind = "dedent"
	TokenDirective   TokenKind = "directive"
	TokenEOF         TokenKind = "eof"
)

// Token stores decoded UTF-8 offsets only. Public coordinate conversion remains
// the responsibility of SourceDocument/common symbol construction.
type Token struct {
	Kind        TokenKind
	Text        string
	StartOffset int
	EndOffset   int
	Nesting     int
}

// ScannerDiagnostic is a bounded recoverable lexical diagnostic.
type ScannerDiagnostic struct {
	Code        string
	Message     string
	StartOffset int
	EndOffset   int
}

// ScannerLimits bound retained tokens, individual token text and delimiter depth.
type ScannerLimits struct {
	MaxTokens     int
	MaxTokenBytes int
	MaxNesting    int
}

// ScanResult contains deterministic lexical output and recoverability evidence.
type ScanResult struct {
	Tokens               []Token
	Diagnostics          []ScannerDiagnostic
	DiagnosticsTruncated bool
	Complete             bool
	MaxDepth             int
}

type delimiterEntry struct {
	open   byte
	close  byte
	offset int
}

type matchedStringRule struct {
	rule           StringRule
	prefix         string
	openingBytes   int
	closingPattern string
}

type sourceScanner struct {
	ctx           context.Context
	document      *SourceDocument
	text          string
	profile       ScannerProfile
	limits        ScannerLimits
	keywords      map[string]struct{}
	result        ScanResult
	at            int
	lineStart     bool
	pendingIndent bool
	continued     bool
	delimiters    []delimiterEntry
	indentStack   []int
}

// ScanSource performs bounded lexical scanning over one already-decoded source document.
func ScanSource(ctx context.Context, document *SourceDocument, profile ScannerProfile, limits ScannerLimits) (ScanResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return ScanResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	if !utf8.ValidString(document.Text) {
		return ScanResult{}, operation.New(operation.KindInvalidInput, "source document text must be valid UTF-8")
	}
	if profile.Name == "" {
		return ScanResult{}, operation.New(operation.KindInvalidInput, "scanner profile name is required")
	}
	if limits.MaxTokens <= 0 || limits.MaxTokenBytes <= 0 || limits.MaxNesting <= 0 {
		return ScanResult{}, operation.New(operation.KindInvalidInput, "scanner limits must be positive")
	}
	if err := ctx.Err(); err != nil {
		return ScanResult{}, operation.Wrap(operation.KindCancelled, "scan_source", document.Path, err)
	}

	scanner := &sourceScanner{
		ctx:           ctx,
		document:      document,
		text:          document.Text,
		profile:       profile,
		limits:        limits,
		keywords:      make(map[string]struct{}, len(profile.Keywords)),
		result:        ScanResult{Complete: true},
		lineStart:     true,
		pendingIndent: profile.Indentation,
		indentStack:   []int{0},
	}
	for _, keyword := range profile.Keywords {
		scanner.keywords[scanner.keywordKey(keyword)] = struct{}{}
	}
	if err := scanner.validateProfile(); err != nil {
		return ScanResult{}, err
	}
	if err := scanner.run(); err != nil {
		return ScanResult{}, err
	}
	return scanner.result, nil
}

func (scanner *sourceScanner) validateProfile() error {
	for _, comment := range scanner.profile.BlockComments {
		if comment.Start == "" || comment.End == "" {
			return operation.New(operation.KindInvalidInput, "block comment delimiters must be non-empty")
		}
	}
	for _, rule := range scanner.profile.Strings {
		if rule.Delimiter == "" {
			return operation.New(operation.KindInvalidInput, "string delimiter must be non-empty")
		}
		if rule.RepeatedDelimiterMin > 0 && len(rule.Delimiter) != 1 {
			return operation.New(operation.KindInvalidInput, "repeated string delimiter must be one byte")
		}
	}
	return nil
}

func (scanner *sourceScanner) run() error {
	for scanner.at < len(scanner.text) {
		if scanner.at&4095 == 0 {
			if err := scanner.checkContext(); err != nil {
				return err
			}
		}

		if scanner.profile.Indentation && scanner.pendingIndent && len(scanner.delimiters) == 0 {
			if err := scanner.processIndentation(); err != nil {
				return err
			}
			if scanner.at >= len(scanner.text) {
				break
			}
		}

		if isHorizontalSpace(scanner.text[scanner.at]) {
			scanner.at++
			continue
		}
		if isNewlineStart(scanner.text[scanner.at]) {
			if err := scanner.scanNewline(); err != nil {
				return err
			}
			continue
		}
		if scanner.lineStart && scanner.profile.Directives && scanner.text[scanner.at] == '#' {
			if err := scanner.scanDirective(); err != nil {
				return err
			}
			continue
		}
		if prefix := scanner.lineCommentPrefixAt(scanner.at); prefix != "" {
			scanner.skipLineComment(prefix)
			continue
		}
		if rule, ok := scanner.blockCommentRuleAt(scanner.at); ok {
			if err := scanner.skipBlockComment(rule); err != nil {
				return err
			}
			continue
		}
		if match, ok := scanner.stringRuleAt(scanner.at); ok {
			if err := scanner.scanString(match); err != nil {
				return err
			}
			continue
		}
		if scanner.profile.ExplicitContinuation != "" && strings.HasPrefix(scanner.text[scanner.at:], scanner.profile.ExplicitContinuation) && scanner.continuationEndsPhysicalLine(scanner.at+len(scanner.profile.ExplicitContinuation)) {
			scanner.at += len(scanner.profile.ExplicitContinuation)
			scanner.continued = true
			scanner.lineStart = false
			continue
		}

		r, size := utf8.DecodeRuneInString(scanner.text[scanner.at:])
		if isIdentifierStart(r) {
			if err := scanner.scanIdentifier(); err != nil {
				return err
			}
			continue
		}
		if unicode.IsDigit(r) {
			if err := scanner.scanNumber(); err != nil {
				return err
			}
			continue
		}
		if isDelimiterOrPunctuation(r) {
			if err := scanner.scanPunctuation(byte(r), size); err != nil {
				return err
			}
			continue
		}
		if err := scanner.scanOperator(); err != nil {
			return err
		}
	}

	if err := scanner.checkContext(); err != nil {
		return err
	}
	if len(scanner.delimiters) > 0 {
		for _, delimiter := range scanner.delimiters {
			scanner.addDiagnostic("unterminated-delimiter", fmt.Sprintf("delimiter %q is not closed", delimiter.open), delimiter.offset, delimiter.offset+1)
		}
	}
	if scanner.profile.Indentation {
		for len(scanner.indentStack) > 1 {
			scanner.indentStack = scanner.indentStack[:len(scanner.indentStack)-1]
			if err := scanner.emitSynthetic(TokenDedent, len(scanner.text), len(scanner.indentStack)-1); err != nil {
				return err
			}
		}
	}
	return scanner.emitSynthetic(TokenEOF, len(scanner.text), len(scanner.delimiters))
}

func (scanner *sourceScanner) processIndentation() error {
	start := scanner.at
	columns := 0
	for scanner.at < len(scanner.text) {
		switch scanner.text[scanner.at] {
		case ' ':
			columns++
			scanner.at++
		case '\t':
			columns += 8 - columns%8
			scanner.at++
		case '\f':
			columns = 0
			scanner.at++
		default:
			goto indentationDone
		}
	}

indentationDone:
	if scanner.at >= len(scanner.text) || isNewlineStart(scanner.text[scanner.at]) || scanner.lineCommentPrefixAt(scanner.at) != "" {
		return nil
	}
	top := scanner.indentStack[len(scanner.indentStack)-1]
	switch {
	case columns > top:
		scanner.indentStack = append(scanner.indentStack, columns)
		if err := scanner.emitSynthetic(TokenIndent, scanner.at, len(scanner.indentStack)-1); err != nil {
			return err
		}
	case columns < top:
		for len(scanner.indentStack) > 1 && columns < scanner.indentStack[len(scanner.indentStack)-1] {
			scanner.indentStack = scanner.indentStack[:len(scanner.indentStack)-1]
			if err := scanner.emitSynthetic(TokenDedent, scanner.at, len(scanner.indentStack)-1); err != nil {
				return err
			}
		}
		if columns != scanner.indentStack[len(scanner.indentStack)-1] {
			scanner.addDiagnostic("inconsistent-indentation", "indentation does not match an earlier level", start, scanner.at)
		}
	}
	scanner.pendingIndent = false
	return nil
}

func (scanner *sourceScanner) scanNewline() error {
	start := scanner.at
	if scanner.text[scanner.at] == '\r' && scanner.at+1 < len(scanner.text) && scanner.text[scanner.at+1] == '\n' {
		scanner.at += 2
	} else {
		scanner.at++
	}
	suppressed := scanner.continued || scanner.profile.ImplicitContinuation && len(scanner.delimiters) > 0
	scanner.continued = false
	if !suppressed {
		if err := scanner.emit(TokenNewline, start, scanner.at); err != nil {
			return err
		}
	}
	scanner.lineStart = true
	scanner.pendingIndent = scanner.profile.Indentation && !suppressed && len(scanner.delimiters) == 0
	return nil
}

func (scanner *sourceScanner) scanDirective() error {
	start := scanner.at
	for scanner.at < len(scanner.text) && !isNewlineStart(scanner.text[scanner.at]) {
		scanner.at++
	}
	scanner.lineStart = false
	return scanner.emit(TokenDirective, start, scanner.at)
}

func (scanner *sourceScanner) skipLineComment(prefix string) {
	scanner.at += len(prefix)
	for scanner.at < len(scanner.text) && !isNewlineStart(scanner.text[scanner.at]) {
		scanner.at++
	}
	scanner.lineStart = false
}

func (scanner *sourceScanner) skipBlockComment(rule BlockCommentRule) error {
	start := scanner.at
	scanner.at += len(rule.Start)
	depth := 1
	for scanner.at < len(scanner.text) {
		if scanner.at&4095 == 0 {
			if err := scanner.checkContext(); err != nil {
				return err
			}
		}
		if rule.Nestable && strings.HasPrefix(scanner.text[scanner.at:], rule.Start) {
			depth++
			if depth > scanner.limits.MaxNesting {
				return scanner.limitError("block comment nesting", depth, scanner.limits.MaxNesting)
			}
			scanner.at += len(rule.Start)
			continue
		}
		if strings.HasPrefix(scanner.text[scanner.at:], rule.End) {
			depth--
			scanner.at += len(rule.End)
			if depth == 0 {
				scanner.lineStart = scanner.segmentEndsAtLineStart(start, scanner.at)
				return nil
			}
			continue
		}
		_, size := utf8.DecodeRuneInString(scanner.text[scanner.at:])
		scanner.at += size
	}
	scanner.addDiagnostic("unterminated-comment", "block comment is not terminated", start, len(scanner.text))
	return nil
}

func (scanner *sourceScanner) scanString(match matchedStringRule) error {
	start := scanner.at
	if err := scanner.consumeString(match, start); err != nil {
		return err
	}
	scanner.lineStart = false
	return scanner.emit(TokenString, start, scanner.at)
}

func (scanner *sourceScanner) consumeString(match matchedStringRule, start int) error {
	scanner.at += match.openingBytes
	if match.rule.InterpolationMarker != "" && strings.Contains(match.prefix, match.rule.InterpolationMarker) {
		return scanner.consumeInterpolatedString(match, start)
	}
	return scanner.consumeOpaqueString(match, start)
}

func (scanner *sourceScanner) consumeOpaqueString(match matchedStringRule, start int) error {
	for scanner.at < len(scanner.text) {
		if scanner.at&4095 == 0 {
			if err := scanner.checkContext(); err != nil {
				return err
			}
		}
		if match.rule.BackslashEscapes && scanner.text[scanner.at] == '\\' {
			scanner.at++
			if scanner.at < len(scanner.text) {
				_, size := utf8.DecodeRuneInString(scanner.text[scanner.at:])
				scanner.at += size
			}
			continue
		}
		if match.rule.DoubledDelimiterEscape && strings.HasPrefix(scanner.text[scanner.at:], match.closingPattern+match.closingPattern) {
			scanner.at += len(match.closingPattern) * 2
			continue
		}
		if strings.HasPrefix(scanner.text[scanner.at:], match.closingPattern) {
			scanner.at += len(match.closingPattern)
			return nil
		}
		if !match.rule.Multiline && isNewlineStart(scanner.text[scanner.at]) {
			scanner.addDiagnostic("unterminated-string", "string literal reaches a physical line ending", start, scanner.at)
			return nil
		}
		_, size := utf8.DecodeRuneInString(scanner.text[scanner.at:])
		scanner.at += size
	}
	scanner.addDiagnostic("unterminated-string", "string literal is not terminated", start, len(scanner.text))
	return nil
}

func (scanner *sourceScanner) consumeInterpolatedString(match matchedStringRule, start int) error {
	interpolationDepth := 0
	for scanner.at < len(scanner.text) {
		if scanner.at&4095 == 0 {
			if err := scanner.checkContext(); err != nil {
				return err
			}
		}
		if interpolationDepth == 0 {
			if match.rule.BackslashEscapes && scanner.text[scanner.at] == '\\' {
				scanner.at++
				if scanner.at < len(scanner.text) {
					_, size := utf8.DecodeRuneInString(scanner.text[scanner.at:])
					scanner.at += size
				}
				continue
			}
			if match.rule.DoubledDelimiterEscape && strings.HasPrefix(scanner.text[scanner.at:], match.closingPattern+match.closingPattern) {
				scanner.at += len(match.closingPattern) * 2
				continue
			}
			if strings.HasPrefix(scanner.text[scanner.at:], match.closingPattern) {
				scanner.at += len(match.closingPattern)
				return nil
			}
			if strings.HasPrefix(scanner.text[scanner.at:], "{{") || strings.HasPrefix(scanner.text[scanner.at:], "}}") {
				scanner.at += 2
				continue
			}
			if scanner.text[scanner.at] == '{' {
				interpolationDepth = 1
				scanner.at++
				continue
			}
			if !match.rule.Multiline && isNewlineStart(scanner.text[scanner.at]) {
				scanner.addDiagnostic("unterminated-string", "string literal reaches a physical line ending", start, scanner.at)
				return nil
			}
			_, size := utf8.DecodeRuneInString(scanner.text[scanner.at:])
			scanner.at += size
			continue
		}

		if prefix := scanner.lineCommentPrefixAt(scanner.at); prefix != "" {
			scanner.skipLineComment(prefix)
			continue
		}
		if rule, ok := scanner.blockCommentRuleAt(scanner.at); ok {
			if err := scanner.skipBlockComment(rule); err != nil {
				return err
			}
			continue
		}
		if nested, ok := scanner.stringRuleAt(scanner.at); ok {
			nestedStart := scanner.at
			if err := scanner.consumeString(nested, nestedStart); err != nil {
				return err
			}
			continue
		}
		switch scanner.text[scanner.at] {
		case '{':
			interpolationDepth++
			if interpolationDepth > scanner.limits.MaxNesting {
				return scanner.limitError("interpolation nesting", interpolationDepth, scanner.limits.MaxNesting)
			}
			scanner.at++
		case '}':
			interpolationDepth--
			scanner.at++
		default:
			_, size := utf8.DecodeRuneInString(scanner.text[scanner.at:])
			scanner.at += size
		}
	}
	scanner.addDiagnostic("unterminated-string", "string literal is not terminated", start, len(scanner.text))
	return nil
}

func (scanner *sourceScanner) scanIdentifier() error {
	start := scanner.at
	_, size := utf8.DecodeRuneInString(scanner.text[scanner.at:])
	scanner.at += size
	for scanner.at < len(scanner.text) {
		r, runeSize := utf8.DecodeRuneInString(scanner.text[scanner.at:])
		if !isIdentifierContinue(r) {
			break
		}
		scanner.at += runeSize
	}
	text := scanner.text[start:scanner.at]
	kind := TokenIdentifier
	if _, ok := scanner.keywords[scanner.keywordKey(text)]; ok {
		kind = TokenKeyword
	}
	scanner.lineStart = false
	return scanner.emit(kind, start, scanner.at)
}

func (scanner *sourceScanner) scanNumber() error {
	start := scanner.at
	for scanner.at < len(scanner.text) {
		r, size := utf8.DecodeRuneInString(scanner.text[scanner.at:])
		if !(unicode.IsDigit(r) || unicode.IsLetter(r) || r == '_' || r == '.') {
			break
		}
		scanner.at += size
	}
	scanner.lineStart = false
	return scanner.emit(TokenNumber, start, scanner.at)
}

func (scanner *sourceScanner) scanPunctuation(value byte, size int) error {
	start := scanner.at
	scanner.at += size
	if close, ok := matchingClose(value); ok {
		if len(scanner.delimiters)+1 > scanner.limits.MaxNesting {
			return scanner.limitError("delimiter nesting", len(scanner.delimiters)+1, scanner.limits.MaxNesting)
		}
		scanner.delimiters = append(scanner.delimiters, delimiterEntry{open: value, close: close, offset: start})
		if len(scanner.delimiters) > scanner.result.MaxDepth {
			scanner.result.MaxDepth = len(scanner.delimiters)
		}
	} else if isClosingDelimiter(value) {
		if len(scanner.delimiters) == 0 {
			scanner.addDiagnostic("mismatched-delimiter", fmt.Sprintf("closing delimiter %q has no opener", value), start, scanner.at)
		} else {
			top := scanner.delimiters[len(scanner.delimiters)-1]
			if top.close == value {
				scanner.delimiters = scanner.delimiters[:len(scanner.delimiters)-1]
			} else {
				scanner.addDiagnostic("mismatched-delimiter", fmt.Sprintf("closing delimiter %q does not match %q", value, top.open), start, scanner.at)
			}
		}
	}
	scanner.lineStart = false
	return scanner.emit(TokenPunctuation, start, scanner.at)
}

func (scanner *sourceScanner) scanOperator() error {
	start := scanner.at
	r, size := utf8.DecodeRuneInString(scanner.text[scanner.at:])
	scanner.at += size
	if isOperatorRune(r) {
		for scanner.at < len(scanner.text) {
			next, nextSize := utf8.DecodeRuneInString(scanner.text[scanner.at:])
			if !isOperatorRune(next) || isDelimiterOrPunctuation(next) {
				break
			}
			scanner.at += nextSize
		}
	}
	scanner.lineStart = false
	return scanner.emit(TokenOperator, start, scanner.at)
}

func (scanner *sourceScanner) emit(kind TokenKind, start, end int) error {
	if end-start > scanner.limits.MaxTokenBytes {
		return scanner.limitError("token bytes", end-start, scanner.limits.MaxTokenBytes)
	}
	if len(scanner.result.Tokens)+1 > scanner.limits.MaxTokens {
		return scanner.limitError("token count", len(scanner.result.Tokens)+1, scanner.limits.MaxTokens)
	}
	scanner.result.Tokens = append(scanner.result.Tokens, Token{
		Kind: kind, Text: scanner.text[start:end], StartOffset: start, EndOffset: end, Nesting: len(scanner.delimiters),
	})
	return nil
}

func (scanner *sourceScanner) emitSynthetic(kind TokenKind, offset, nesting int) error {
	if len(scanner.result.Tokens)+1 > scanner.limits.MaxTokens {
		return scanner.limitError("token count", len(scanner.result.Tokens)+1, scanner.limits.MaxTokens)
	}
	scanner.result.Tokens = append(scanner.result.Tokens, Token{Kind: kind, StartOffset: offset, EndOffset: offset, Nesting: nesting})
	return nil
}

func (scanner *sourceScanner) addDiagnostic(code, message string, start, end int) {
	scanner.result.Complete = false
	if len(scanner.result.Diagnostics) >= ScannerMaxDiagnostics {
		scanner.result.DiagnosticsTruncated = true
		return
	}
	scanner.result.Diagnostics = append(scanner.result.Diagnostics, ScannerDiagnostic{Code: code, Message: message, StartOffset: start, EndOffset: end})
}

func (scanner *sourceScanner) lineCommentPrefixAt(offset int) string {
	best := ""
	for _, prefix := range scanner.profile.LineComments {
		if len(prefix) > len(best) && strings.HasPrefix(scanner.text[offset:], prefix) {
			best = prefix
		}
	}
	return best
}

func (scanner *sourceScanner) blockCommentRuleAt(offset int) (BlockCommentRule, bool) {
	best := BlockCommentRule{}
	found := false
	for _, rule := range scanner.profile.BlockComments {
		if strings.HasPrefix(scanner.text[offset:], rule.Start) && (!found || len(rule.Start) > len(best.Start)) {
			best = rule
			found = true
		}
	}
	return best, found
}

func (scanner *sourceScanner) stringRuleAt(offset int) (matchedStringRule, bool) {
	for _, rule := range scanner.profile.Strings {
		for _, prefix := range rule.Prefixes {
			prefixEnd := offset + len(prefix)
			if prefixEnd > len(scanner.text) || !stringPrefixMatches(scanner.text[offset:prefixEnd], prefix, rule.CaseInsensitivePrefix) {
				continue
			}
			if rule.RepeatedDelimiterMin > 0 {
				if prefixEnd >= len(scanner.text) || scanner.text[prefixEnd] != rule.Delimiter[0] {
					continue
				}
				count := 0
				for prefixEnd+count < len(scanner.text) && scanner.text[prefixEnd+count] == rule.Delimiter[0] {
					count++
				}
				if count < rule.RepeatedDelimiterMin {
					continue
				}
				return matchedStringRule{rule: rule, prefix: prefix, openingBytes: len(prefix) + count, closingPattern: strings.Repeat(rule.Delimiter, count)}, true
			}
			if strings.HasPrefix(scanner.text[prefixEnd:], rule.Delimiter) {
				return matchedStringRule{rule: rule, prefix: prefix, openingBytes: len(prefix) + len(rule.Delimiter), closingPattern: rule.Delimiter}, true
			}
		}
	}
	return matchedStringRule{}, false
}

func stringPrefixMatches(actual, expected string, caseInsensitive bool) bool {
	if caseInsensitive {
		return strings.EqualFold(actual, expected)
	}
	return actual == expected
}

func (scanner *sourceScanner) continuationEndsPhysicalLine(offset int) bool {
	for offset < len(scanner.text) && isHorizontalSpace(scanner.text[offset]) {
		offset++
	}
	if offset >= len(scanner.text) || isNewlineStart(scanner.text[offset]) {
		return true
	}
	if prefix := scanner.lineCommentPrefixAt(offset); prefix != "" {
		offset += len(prefix)
		for offset < len(scanner.text) && !isNewlineStart(scanner.text[offset]) {
			offset++
		}
		return offset >= len(scanner.text) || isNewlineStart(scanner.text[offset])
	}
	return false
}

func (scanner *sourceScanner) segmentEndsAtLineStart(start, end int) bool {
	lastNewline := -1
	for index := start; index < end; index++ {
		if scanner.text[index] == '\r' || scanner.text[index] == '\n' {
			lastNewline = index
		}
	}
	if lastNewline < 0 {
		return false
	}
	for index := lastNewline + 1; index < end; index++ {
		if !isHorizontalSpace(scanner.text[index]) {
			return false
		}
	}
	return true
}

func (scanner *sourceScanner) keywordKey(value string) string {
	if scanner.profile.CaseInsensitive {
		return strings.ToLower(value)
	}
	return value
}

func (scanner *sourceScanner) checkContext() error {
	if err := scanner.ctx.Err(); err != nil {
		return operation.Wrap(operation.KindCancelled, "scan_source", scanner.document.Path, err)
	}
	return nil
}

func (scanner *sourceScanner) limitError(name string, actual, limit int) error {
	return operation.Wrap(operation.KindLimit, "scan_source", scanner.document.Path, fmt.Errorf("%s %d exceeds limit %d", name, actual, limit))
}

func isHorizontalSpace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\f' || value == '\v'
}

func isNewlineStart(value byte) bool { return value == '\r' || value == '\n' }

func isIdentifierStart(value rune) bool {
	return value == '_' || unicode.IsLetter(value)
}

func isIdentifierContinue(value rune) bool {
	return isIdentifierStart(value) || unicode.IsDigit(value) || unicode.IsMark(value) || unicode.In(value, unicode.Pc)
}

func isDelimiterOrPunctuation(value rune) bool {
	return strings.ContainsRune("()[]{}.,:;", value)
}

func matchingClose(value byte) (byte, bool) {
	switch value {
	case '(':
		return ')', true
	case '[':
		return ']', true
	case '{':
		return '}', true
	default:
		return 0, false
	}
}

func isClosingDelimiter(value byte) bool {
	return value == ')' || value == ']' || value == '}'
}

func isOperatorRune(value rune) bool {
	return strings.ContainsRune("+-*/%=&|^!<>?~\\@#$", value)
}
