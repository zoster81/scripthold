package sourceintelligence

import "strings"

// ScopeBoundaryStyle identifies how one lexical scope is closed by an analyzer.
type ScopeBoundaryStyle string

const (
	ScopeBoundaryBrace       ScopeBoundaryStyle = "brace"
	ScopeBoundaryExplicitEnd ScopeBoundaryStyle = "explicit-end"
	ScopeBoundaryIndent      ScopeBoundaryStyle = "indent"
)

// ScopeBoundary stores only the generic information needed by reusable closing
// adapters. Language-specific syntax remains in the analyzer.
type ScopeBoundary struct {
	Style ScopeBoundaryStyle
	Value int
	Label string
}

// ScopeFrame is the language-neutral ownership record for one open symbol scope.
type ScopeFrame struct {
	SymbolID      string
	QualifiedName string
	Kind          string
	Boundary      ScopeBoundary
}

// ScopeStack tracks lexical ownership without assuming brace, End-delimited, or
// indentation syntax in the common symbol model.
type ScopeStack struct {
	frames []ScopeFrame
}

func NewScopeStack() *ScopeStack { return &ScopeStack{} }

func (stack *ScopeStack) Push(frame ScopeFrame) {
	if stack == nil {
		return
	}
	stack.frames = append(stack.frames, frame)
}

func (stack *ScopeStack) Current() (ScopeFrame, bool) {
	if stack == nil || len(stack.frames) == 0 {
		return ScopeFrame{}, false
	}
	return stack.frames[len(stack.frames)-1], true
}

func (stack *ScopeStack) Pop() (ScopeFrame, bool) {
	if stack == nil || len(stack.frames) == 0 {
		return ScopeFrame{}, false
	}
	last := len(stack.frames) - 1
	frame := stack.frames[last]
	stack.frames = stack.frames[:last]
	return frame, true
}

// CloseBrace pops only brace-managed scopes deeper than the supplied current
// delimiter depth. Frames managed by another syntax style are never crossed.
func (stack *ScopeStack) CloseBrace(depth int) []ScopeFrame {
	if stack == nil {
		return nil
	}
	var popped []ScopeFrame
	for len(stack.frames) > 0 {
		top := stack.frames[len(stack.frames)-1]
		if top.Boundary.Style != ScopeBoundaryBrace || top.Boundary.Value <= depth {
			break
		}
		stack.frames = stack.frames[:len(stack.frames)-1]
		popped = append(popped, top)
	}
	return popped
}

// CloseExplicit closes only the top matching explicit-End scope. It deliberately
// refuses to search through intervening frames, preventing malformed input from
// silently re-parenting declarations.
func (stack *ScopeStack) CloseExplicit(label string) (ScopeFrame, bool) {
	if stack == nil || len(stack.frames) == 0 {
		return ScopeFrame{}, false
	}
	top := stack.frames[len(stack.frames)-1]
	if top.Boundary.Style != ScopeBoundaryExplicitEnd || !strings.EqualFold(strings.TrimSpace(top.Boundary.Label), strings.TrimSpace(label)) {
		return ScopeFrame{}, false
	}
	stack.frames = stack.frames[:len(stack.frames)-1]
	return top, true
}

// Dedent pops only indentation-managed scopes deeper than the new indentation
// level. Other scope styles are never crossed.
func (stack *ScopeStack) Dedent(indent int) []ScopeFrame {
	if stack == nil {
		return nil
	}
	var popped []ScopeFrame
	for len(stack.frames) > 0 {
		top := stack.frames[len(stack.frames)-1]
		if top.Boundary.Style != ScopeBoundaryIndent || top.Boundary.Value <= indent {
			break
		}
		stack.frames = stack.frames[:len(stack.frames)-1]
		popped = append(popped, top)
	}
	return popped
}

func (stack *ScopeStack) Len() int {
	if stack == nil {
		return 0
	}
	return len(stack.frames)
}

func (stack *ScopeStack) Frames() []ScopeFrame {
	if stack == nil {
		return nil
	}
	return append([]ScopeFrame(nil), stack.frames...)
}
