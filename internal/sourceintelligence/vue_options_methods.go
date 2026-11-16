package sourceintelligence

import (
	"context"
	"fmt"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

// appendVueOptionsMethods indexes direct shorthand methods, computed getters,
// watch callbacks, and lifecycle hooks under an exported Vue Options object. Dynamic spreads,
// function-valued properties, and nested object members remain unsupported.
func appendVueOptionsMethods(ctx context.Context, document *SourceDocument, options AnalyzeOptions, regions []compositeEmbeddedRegion, result *AnalyzerResult) error {
	maxNesting := options.MaxNesting
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	for regionIndex, region := range regions {
		if region.kind != "script" || (region.language != "javascript" && region.language != "typescript") {
			continue
		}
		if !strings.Contains(document.Text[region.content.Start:region.content.End], "export") {
			continue
		}
		if err := ctx.Err(); err != nil {
			return operation.Wrap(operation.KindCancelled, "analyze_vue_options", document.Path, err)
		}
		masked, err := MaskOutsideRanges(document.Text, []OffsetRange{region.content})
		if err != nil {
			return err
		}
		masked, err = maskECMAScriptRegexLiterals(ctx, masked, maxNesting)
		if err != nil {
			return err
		}
		clone := *document
		clone.Text = masked
		clone.lineStarts = buildLineStarts(masked)
		profile := JavaScriptScannerProfile()
		if region.language == "typescript" {
			profile = TypeScriptScannerProfile()
		}
		scan, err := ScanSource(ctx, &clone, profile, ScannerLimits{
			MaxTokens: scannerTokenBudget(masked), MaxTokenBytes: 1024 * 1024, MaxNesting: maxNesting,
		})
		if err != nil {
			return err
		}
		pairs := PairDelimiterTokens(scan.Tokens, nil)
		regionID := fmt.Sprintf("script-%d", regionIndex+1)
		builder := NewSymbolBuilder(document, SymbolBuilderOptions{
			Context: ctx, Language: region.language, Analyzer: string(AnalyzerVue), RegionID: regionID,
			IncludeSignatures: options.IncludeSignatures, MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits,
		})
		if err := builder.checkReady(); err != nil {
			return err
		}
		tokens := scan.Tokens
		for i := 0; i < len(tokens); i++ {
			if tokens[i].Text != "export" || tokens[i].Nesting != 0 {
				continue
			}
			def := vueNextCodeToken(tokens, i+1, len(tokens))
			if def >= len(tokens) || tokens[def].Text != "default" {
				continue
			}
			open := vueNextCodeToken(tokens, def+1, len(tokens))
			if open >= len(tokens) || tokens[open].Text != "{" {
				continue
			}
			close, ok := pairs[open]
			if !ok || close <= open {
				builder.MarkIncomplete()
				continue
			}
			collectVueOptionsMethods(ctx, tokens, pairs, open+1, close, builder)
			i = close
		}
		if !scan.Complete {
			builder.MarkIncomplete()
		}
		result.Analysis = mergeCompositeAnalysis(result.Analysis, builder.Result(), options.Limits)
		if err := ctx.Err(); err != nil {
			return operation.Wrap(operation.KindCancelled, "analyze_vue_options", document.Path, err)
		}
	}
	return nil
}

func vueNextCodeToken(tokens []Token, start, end int) int {
	for start < end && (tokens[start].Kind == TokenNewline || tokens[start].Kind == TokenDirective) {
		start++
	}
	return start
}

func collectVueOptionsMethods(ctx context.Context, tokens []Token, pairs map[int]int, start, end int, builder *SymbolBuilder) {
	for i := start; i < end; {
		if ctx.Err() != nil {
			return
		}
		i = vueNextCodeToken(tokens, i, end)
		if i >= end {
			break
		}
		if tokens[i].Text == "methods" || tokens[i].Text == "computed" || tokens[i].Text == "watch" {
			kind, nativeKind := SymbolKindMethod, "vue-options-method"
			switch tokens[i].Text {
			case "computed":
				kind, nativeKind = SymbolKindProperty, "vue-options-computed"
			case "watch":
				nativeKind = "vue-options-watch"
			}
			colon := vueNextCodeToken(tokens, i+1, end)
			open := vueNextCodeToken(tokens, colon+1, end)
			if colon < end && tokens[colon].Text == ":" && open < end && tokens[open].Text == "{" {
				if close, ok := pairs[open]; ok && close < end {
					collectVueShorthandMethods(ctx, tokens, pairs, open+1, close, builder, kind, nativeKind)
					i = close + 1
					continue
				}
				builder.MarkIncomplete()
			}
		}
		if vueOptionsLifecycleHook(tokens[i].Text) {
			paren := vueNextCodeToken(tokens, i+1, end)
			if paren < end && tokens[paren].Text == "(" {
				if closeParen, ok := pairs[paren]; ok && closeParen < end {
					body := vueNextCodeToken(tokens, closeParen+1, end)
					if body < end && tokens[body].Text == "{" {
						if closeBody, ok := pairs[body]; ok && closeBody < end {
							collectVueShorthandMethods(ctx, tokens, pairs, i, closeBody+1, builder, SymbolKindMethod, "vue-options-lifecycle")
							i = closeBody + 1
							continue
						}
						builder.MarkIncomplete()
					}
				}
			}
		}
		if close, ok := pairs[i]; ok && close > i {
			i = close + 1
		} else {
			i++
		}
	}
}

func vueOptionsLifecycleHook(name string) bool {
	switch name {
	case "beforeCreate", "created", "beforeMount", "mounted", "beforeUpdate", "updated",
		"activated", "deactivated", "beforeDestroy", "destroyed", "beforeUnmount", "unmounted",
		"errorCaptured", "renderTracked", "renderTriggered", "serverPrefetch":
		return true
	}
	return false
}

func collectVueShorthandMethods(ctx context.Context, tokens []Token, pairs map[int]int, start, end int, builder *SymbolBuilder, kind SymbolKind, nativeKind string) {
	for i := start; i < end; {
		if ctx.Err() != nil {
			return
		}
		i = vueNextCodeToken(tokens, i, end)
		if i >= end {
			break
		}
		name := i
		if tokens[name].Text == "async" {
			name = vueNextCodeToken(tokens, name+1, end)
		}
		if name < end && (tokens[name].Kind == TokenIdentifier || tokens[name].Kind == TokenKeyword) {
			paren := vueNextCodeToken(tokens, name+1, end)
			if paren < end && tokens[paren].Text == "(" {
				if closeParen, ok := pairs[paren]; ok && closeParen < end {
					body := vueNextCodeToken(tokens, closeParen+1, end)
					if body < end && tokens[body].Text == "{" {
						if closeBody, ok := pairs[body]; ok && closeBody < end {
							_, err := builder.Add(SymbolSpec{
								Kind: kind, NativeKind: nativeKind, Name: tokens[name].Text,
								Declaration:   OffsetRange{Start: tokens[i].StartOffset, End: tokens[closeBody].EndOffset},
								NameRange:     OffsetRange{Start: tokens[name].StartOffset, End: tokens[name].EndOffset},
								Signature:     &OffsetRange{Start: tokens[i].StartOffset, End: tokens[body].StartOffset},
								Body:          &OffsetRange{Start: tokens[body].StartOffset, End: tokens[closeBody].EndOffset},
								Evidence:      SymbolEvidenceStructural,
								Disambiguator: tokenRangeText(tokens, paren, closeParen+1),
							})
							if err != nil && operation.KindOf(err) != operation.KindLimit {
								builder.MarkIncomplete()
							}
							i = closeBody + 1
							continue
						}
						builder.MarkIncomplete()
					}
				}
			}
		}
		if close, ok := pairs[i]; ok && close > i {
			i = close + 1
		} else {
			i++
		}
	}
}
