package sourceintelligence

import (
	"strconv"
	"strings"
)

type haskellCPPBranchEvent struct {
	offset int
	scope  string
}

// Each CPP alternative gets a separate lexical identity. Parent-scope
// signatures still suppress their corresponding nested branch definitions.
func haskellCPPBranchEvents(source string) []haskellCPPBranchEvent {
	events := make([]haskellCPPBranchEvent, 0, 16)
	parents := make([]string, 0, 8)
	scope := ""
	nextBranch := 0
	for start := 0; start < len(source); {
		end := strings.IndexByte(source[start:], '\n')
		after := len(source)
		if end >= 0 {
			after = start + end + 1
		}
		line := strings.TrimRight(source[start:after], "\r\n")
		if strings.HasPrefix(line, "#") {
			parts := strings.Fields(line)
			if len(parts) > 0 {
				changed := false
				switch parts[0] {
				case "#if", "#ifdef", "#ifndef":
					parents = append(parents, scope)
					nextBranch++
					scope += strconv.Itoa(nextBranch) + "/"
					changed = true
				case "#else", "#elif":
					if len(parents) > 0 {
						nextBranch++
						scope = parents[len(parents)-1] + strconv.Itoa(nextBranch) + "/"
						changed = true
					}
				case "#endif":
					if len(parents) > 0 {
						scope = parents[len(parents)-1]
						parents = parents[:len(parents)-1]
						changed = true
					}
				}
				if changed {
					events = append(events, haskellCPPBranchEvent{after, scope})
				}
			}
		}
		start = after
	}
	return events
}

func haskellSeenInCPPAncestor(seen map[string]map[string]struct{}, name, scope string) bool {
	group := seen[name]
	for {
		if _, found := group[scope]; found {
			return true
		}
		if scope == "" {
			return false
		}
		scope = scope[:strings.LastIndexByte(scope[:len(scope)-1], '/')+1]
	}
}

func haskellRememberCPPBinding(seen map[string]map[string]struct{}, name, scope string) {
	group := seen[name]
	if group == nil {
		group = make(map[string]struct{})
		seen[name] = group
	}
	group[scope] = struct{}{}
}

// haskellCPPDirectiveAt checks the physical source line, not scanner tokens.
// The lexer may expose directive tokens as a logical line with indentation 0.
func haskellCPPDirectiveAt(source string, offset int) bool {
	if offset < 0 || offset >= len(source) {
		return false
	}
	start := strings.LastIndexAny(source[:offset], "\r\n") + 1
	line := source[start:]
	if end := strings.IndexAny(line, "\r\n"); end >= 0 {
		line = line[:end]
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return false
	}
	switch fields[0] {
	case "#if", "#ifdef", "#ifndef", "#elif", "#else", "#endif":
		return true
	default:
		return false
	}
}
