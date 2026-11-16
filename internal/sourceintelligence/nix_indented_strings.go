package sourceintelligence

// maskNixIndentedStrings blanks Nix indented and double-quoted string content
// while preserving byte offsets and line endings for bounded let navigation.
// Double-quote delimiters remain visible for unsupported-root detection.
// It does not try to interpret interpolations or evaluate Nix expressions.
func maskNixIndentedStrings(source string) string {
	masked := []byte(source)
	inIndented, inDouble, inBlockComment := false, false, false
	blank := func(index int) {
		if masked[index] != '\r' && masked[index] != '\n' {
			masked[index] = ' '
		}
	}
	for i := 0; i < len(source); {
		if inIndented {
			if i+1 < len(source) && source[i] == '\'' && source[i+1] == '\'' {
				blank(i)
				blank(i + 1)
				if i+2 < len(source) && (source[i+2] == '$' ||
					source[i+2] == '\'' || source[i+2] == '\\') {
					blank(i + 2)
					i += 3
					continue
				}
				inIndented = false
				i += 2
				continue
			}
			blank(i)
			i++
			continue
		}
		if inBlockComment {
			if i+1 < len(source) && source[i] == '*' && source[i+1] == '/' {
				inBlockComment = false
				i += 2
				continue
			}
			i++
			continue
		}
		if inDouble {
			if source[i] == '\\' && i+1 < len(source) {
				blank(i)
				blank(i + 1)
				i += 2
				continue
			}
			if source[i] == '"' {
				inDouble = false
				i++
				continue
			}
			blank(i)
			i++
			continue
		}
		if source[i] == '#' {
			for i < len(source) && source[i] != '\r' && source[i] != '\n' {
				i++
			}
			continue
		}
		if i+1 < len(source) && source[i] == '/' && source[i+1] == '*' {
			inBlockComment = true
			i += 2
			continue
		}
		if source[i] == '"' {
			inDouble = true
			i++
			continue
		}
		if i+1 < len(source) && source[i] == '\'' && source[i+1] == '\'' {
			blank(i)
			blank(i + 1)
			inIndented = true
			i += 2
			continue
		}
		i++
	}
	return string(masked)
}
