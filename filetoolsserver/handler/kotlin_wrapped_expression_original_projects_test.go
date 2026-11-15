package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Full original files are read as-is; no third-party project code is executed.
func TestKotlinOriginalWrappedExpressionNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original projects not configured")
	}
	cases := []struct {
		project, source, name string
		start, end            int
	}{
		{"project-kotlin-jakewharton-timber", "timber/src/androidMain/kotlin/timber/log/Timber.kt", "isLoggable", 144, 145},
		{"project-kotlin-jakewharton-timber", "timber/src/androidMain/kotlin/timber/log/Timber.kt", "formatMessage", 173, 174},
		{"project-kotlin-square-moshi", "moshi-kotlin/src/main/java/com/squareup/moshi/kotlin/reflect/JvmSignatureSearcher.kt", "getter", 17, 18},
		{"project-kotlin-square-moshi", "moshi-kotlin/src/main/java/com/squareup/moshi/kotlin/reflect/JvmSignatureSearcher.kt", "syntheticMethodForAnnotations", 14, 15},
	}
	for _, tc := range cases {
		t.Run(tc.name+"-"+tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			path := filepath.Join(root, filepath.FromSlash(tc.source))
			stat, err := os.Lstat(path)
			if err != nil || !stat.Mode().IsRegular() || stat.Size() > 1<<20 {
				t.Fatalf("original Kotlin file unavailable or unsafe: %v", err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
			if tc.start < 1 || tc.end > len(lines) ||
				!strings.Contains(lines[tc.start-1], "fun "+tc.name+"(") ||
				!strings.HasSuffix(lines[tc.start-1], "=") ||
				strings.TrimSpace(lines[tc.end-1]) == "" {
				t.Fatalf("original Kotlin declaration oracle changed: %s:%d-%d", tc.source, tc.start, tc.end)
			}
			expected := strings.TrimLeft(strings.Join(lines[tc.start-1:tc.end], "\n"), " \t")
			h := NewHandler([]string{root})
			ctx := context.Background()
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "kotlin", MaxFiles: 1,
			})
			if err != nil || digest.FilesParsed != 1 || digest.Truncated {
				t.Fatalf("original Kotlin digest failed: %v %+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "kotlin",
				Presentation: "detailed", MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || outline.Truncated {
				t.Fatalf("original Kotlin outline failed: %v %+v", err, outline)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "kotlin",
				Query: tc.name, Match: "exact", FileDetails: "relevant",
				MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || len(found.Files) != 1 || found.Truncated {
				t.Fatalf("original Kotlin find failed: %v %+v", err, found)
			}
			id := ""
			for _, s := range found.Symbols {
				if s.Name == tc.name && s.Kind == "method" &&
					s.DeclarationRange.Start.Line == tc.start && s.DeclarationRange.End.Line == tc.end {
					id = s.ID
				}
			}
			if id == "" {
				t.Fatalf("original Kotlin wrapped method missing or truncated: %s:%d %+v", tc.source, tc.start, found.Symbols)
			}
			inOutline := false
			for _, s := range outline.Symbols {
				if s.ID == id {
					inOutline = true
				}
			}
			if !inOutline {
				t.Fatalf("original Kotlin symbol absent from outline: %s", tc.name)
			}
			f := found.Files[0]
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "kotlin", Encoding: f.Encoding,
				SymbolID: id, SourceFingerprint: f.SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || shown.Show == nil ||
				strings.ReplaceAll(shown.Show.Text, "\r\n", "\n") != expected {
				t.Fatalf("Kotlin original show mismatch: %s:%d err=%v shown=%+v", tc.source, tc.start, err, shown.Show)
			}
		})
	}
}

func TestKotlinOriginalValueClassNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original projects not configured")
	}
	cases := []struct {
		project, source, class, parent, member, memberKind string
		classStart, classEnd, memberLine                   int
	}{
		{"project-kotlin-square-moshi", "moshi-kotlin/src/main/java/com/squareup/moshi/kotlin/reflect/Invokable.kt",
			"InvokableConstructor", "com.squareup.moshi.kotlin.reflect.Invokable", "parameterTypes", "property", 24, 42, 25},
		{"project-kotlin-square-moshi", "moshi-kotlin/src/main/java/com/squareup/moshi/kotlin/reflect/Invokable.kt",
			"InvokableMethod", "com.squareup.moshi.kotlin.reflect.Invokable", "parameterTypes", "property", 45, 68, 46},
		{"project-kotlin-zhanghai-materialfiles", "app/src/main/java/me/zhanghai/android/files/file/FileSize.kt",
			"FileSize", "me.zhanghai.android.files.file", "formatInBytes", "method", 14, 25, 20},
	}
	for _, tc := range cases {
		t.Run(tc.class, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			path := filepath.Join(root, filepath.FromSlash(tc.source))
			stat, err := os.Lstat(path)
			if err != nil || !stat.Mode().IsRegular() || stat.Size() > 1<<20 {
				t.Fatalf("original Kotlin class fixture unavailable or unsafe: %v", err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
			if tc.classStart < 1 || tc.classEnd > len(lines) || tc.memberLine > tc.classEnd ||
				!strings.Contains(lines[tc.classStart-1], "value class "+tc.class+"(") ||
				!strings.Contains(lines[tc.memberLine-1], tc.member) ||
				strings.TrimSpace(lines[tc.classEnd-1]) != "}" {
				t.Fatalf("original Kotlin class oracle changed: %s:%d-%d", tc.source, tc.classStart, tc.classEnd)
			}
			expected := strings.TrimLeft(strings.Join(lines[tc.classStart-1:tc.classEnd], "\n"), " \t")
			h := NewHandler([]string{root})
			ctx := context.Background()
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "kotlin",
				Presentation: "detailed", MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || outline.Truncated || len(outline.Files) != 1 {
				t.Fatalf("original Kotlin outline failed: %v %+v", err, outline)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "kotlin",
				Query: tc.class, Match: "exact", FileDetails: "relevant", MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("original Kotlin class find failed: %v %+v", err, found)
			}
			id := ""
			for _, sym := range found.Symbols {
				if sym.Name == tc.class && sym.Kind == "class" && sym.NativeKind == "value-class" &&
					sym.ParentQualifiedName == tc.parent && sym.NameRange.Start.Line == tc.classStart {
					id = sym.ID
				}
			}
			if id == "" {
				t.Fatalf("original Kotlin value class not found or wrong owner: %s %+v", tc.class, found.Symbols)
			}
			memberSeen, classSeen := false, false
			for _, sym := range outline.Symbols {
				if sym.ID == id {
					classSeen = true
				}
				if sym.Name == tc.member && string(sym.Kind) == tc.memberKind &&
					sym.ParentID == id && sym.NameRange.Start.Line == tc.memberLine {
					memberSeen = true
				}
			}
			if !classSeen || !memberSeen {
				t.Fatalf("original Kotlin class/member ownership not indexed: %s class=%t member=%t", tc.class, classSeen, memberSeen)
			}
			f := found.Files[0]
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "kotlin", Encoding: f.Encoding,
				SymbolID: id, SourceFingerprint: f.SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || shown.Show == nil ||
				strings.ReplaceAll(shown.Show.Text, "\r\n", "\n") != expected {
				t.Fatalf("original Kotlin class show mismatch: %s err=%v shown=%+v", tc.class, err, shown.Show)
			}
		})
	}
}

func TestKotlinOriginalExplicitGetterNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original projects not configured")
	}
	cases := []struct {
		project, source, property, owner, sibling string
		start, end, siblingLine                   int
	}{
		{"project-kotlin-square-moshi", "moshi-kotlin/src/main/java/com/squareup/moshi/kotlin/reflect/Invokable.kt",
			"parameterTypes", "com.squareup.moshi.kotlin.reflect.Invokable.InvokableConstructor", "parameterAnnotations", 25, 26, 28},
		{"project-kotlin-square-moshi", "moshi-kotlin/src/main/java/com/squareup/moshi/kotlin/reflect/Invokable.kt",
			"parameterTypes", "com.squareup.moshi.kotlin.reflect.Invokable.InvokableMethod", "parameterAnnotations", 46, 47, 49},
		{"project-kotlin-square-moshi", "moshi-kotlin/src/main/java/com/squareup/moshi/kotlin/reflect/IndexedParameterMap.kt",
			"entries", "com.squareup.moshi.kotlin.reflect.IndexedParameterMap", "containsKey", 11, 17, 19},
		{"project-kotlin-zhanghai-materialfiles", "app/src/main/java/me/zhanghai/android/files/file/FileSize.kt",
			"isHumanReadableInBytes", "me.zhanghai.android.files.file.FileSize", "formatInBytes", 17, 18, 20},
	}
	for _, tc := range cases {
		t.Run(tc.property+"-"+tc.project+"-"+tc.owner, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			path := filepath.Join(root, filepath.FromSlash(tc.source))
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("original Kotlin property fixture unavailable or unsafe: %v", err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
			if tc.start < 1 || tc.end > len(lines) || tc.siblingLine > len(lines) ||
				!strings.Contains(lines[tc.start-1], "val "+tc.property+":") ||
				!strings.Contains(lines[tc.start], "get()") ||
				!strings.Contains(lines[tc.siblingLine-1], tc.sibling) {
				t.Fatalf("original getter oracle changed: %s:%d-%d", tc.source, tc.start, tc.end)
			}
			expected := strings.TrimLeft(strings.Join(lines[tc.start-1:tc.end], "\n"), " \t")
			h := NewHandler([]string{root})
			ctx := context.Background()
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "kotlin",
				Presentation: "detailed", MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || outline.Truncated {
				t.Fatalf("original Kotlin getter outline error: %v %+v", err, outline)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "kotlin",
				Query: tc.property, Match: "exact", FileDetails: "relevant", MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("original Kotlin getter find error: %v %+v", err, found)
			}
			id := ""
			for _, symbol := range found.Symbols {
				if symbol.Name == tc.property && string(symbol.Kind) == "property" &&
					symbol.ParentQualifiedName == tc.owner &&
					symbol.DeclarationRange.Start.Line == tc.start &&
					symbol.DeclarationRange.End.Line == tc.end {
					id = symbol.ID
				}
			}
			if id == "" {
				t.Fatalf("original Kotlin getter range or owner missing: %s:%d %+v", tc.source, tc.start, found.Symbols)
			}
			seenProperty, seenSibling := false, false
			for _, symbol := range outline.Symbols {
				if symbol.ID == id {
					seenProperty = true
				}
				if symbol.Name == tc.sibling && symbol.ParentQualifiedName == tc.owner &&
					symbol.DeclarationRange.Start.Line == tc.siblingLine {
					seenSibling = true
				}
			}
			if !seenProperty || !seenSibling {
				t.Fatalf("getter or separate sibling missing from outline: property=%t sibling=%t", seenProperty, seenSibling)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "kotlin", Encoding: found.Files[0].Encoding,
				SymbolID: id, SourceFingerprint: found.Files[0].SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || shown.Show == nil || strings.ReplaceAll(shown.Show.Text, "\r\n", "\n") != expected {
				t.Fatalf("original Kotlin getter excerpt mismatch: %s:%d err=%v shown=%+v", tc.source, tc.start, err, shown.Show)
			}
		})
	}
}

func TestKotlinOriginalMultilineGetterNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original projects not configured")
	}
	cases := []struct {
		project, source, property, owner, sibling string
		start, end, siblingLine                   int
	}{
		{"project-kotlin-jakewharton-timber", "timber/src/androidMain/kotlin/timber/log/Timber.kt",
			"tag", "timber.log.Timber.DebugTree", "createStackElementTag", 207, 213, 221},
		{"project-kotlin-zhanghai-materialfiles", "app/src/main/java/me/zhanghai/android/files/compat/StorageVolumeCompat.kt",
			"directoryCompat", "me.zhanghai.android.files.compat", "getDescriptionCompat", 32, 41, 44},
	}
	for _, tc := range cases {
		t.Run(tc.property+"-"+tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			path := filepath.Join(root, filepath.FromSlash(tc.source))
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("unsafe or missing original Kotlin source: %v", err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
			if tc.start < 1 || tc.end > len(lines) || tc.siblingLine > len(lines) ||
				!strings.Contains(lines[tc.start-1], tc.property+":") ||
				strings.TrimSpace(lines[tc.start]) != "get() =" ||
				!strings.Contains(lines[tc.siblingLine-1], tc.sibling+"(") {
				t.Fatalf("independent original getter oracle changed: %s:%d-%d", tc.source, tc.start, tc.end)
			}
			expected := strings.TrimLeft(strings.Join(lines[tc.start-1:tc.end], "\n"), " \t")
			h := NewHandler([]string{root})
			ctx := context.Background()
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "kotlin",
				Presentation: "detailed", MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || outline.Truncated {
				t.Fatalf("original Kotlin multiline outline failed: %v %+v", err, outline)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "kotlin",
				Query: tc.property, Match: "exact", FileDetails: "relevant", MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("original Kotlin multiline find failed: %v %+v", err, found)
			}
			id := ""
			for _, s := range found.Symbols {
				if s.Name == tc.property && string(s.Kind) == "property" &&
					s.ParentQualifiedName == tc.owner &&
					s.DeclarationRange.Start.Line == tc.start && s.DeclarationRange.End.Line == tc.end {
					id = s.ID
				}
			}
			if id == "" {
				t.Fatalf("original Kotlin multiline getter missing/partial: %+v", found.Symbols)
			}
			hasProperty, hasSibling := false, false
			for _, s := range outline.Symbols {
				if s.ID == id {
					hasProperty = true
				}
				if s.Name == tc.sibling && s.ParentQualifiedName == tc.owner &&
					s.DeclarationRange.Start.Line == tc.siblingLine {
					hasSibling = true
				}
			}
			if !hasProperty || !hasSibling {
				t.Fatalf("original Kotlin getter or sibling lost: property=%t sibling=%t", hasProperty, hasSibling)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "kotlin", Encoding: found.Files[0].Encoding,
				SymbolID: id, SourceFingerprint: found.Files[0].SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || shown.Show == nil ||
				strings.ReplaceAll(shown.Show.Text, "\r\n", "\n") != expected {
				t.Fatalf("original Kotlin multiline source mismatch: %s:%d err=%v shown=%+v", tc.source, tc.start, err, shown.Show)
			}
		})
	}
}

func TestKotlinOriginalPropertySetterNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original projects not configured")
	}
	cases := []struct {
		project, source, name, owner, sibling, siblingKind string
		start, end, siblingLine                            int
	}{
		{"project-kotlin-zhanghai-materialfiles",
			"app/src/main/java/me/zhanghai/android/files/compat/DrawableCompat.kt",
			"layoutDirectionCompat", "me.zhanghai.android.files.compat", "setTintCompat", "function", 12, 16, 18},
		{"project-kotlin-lysine-dev-okio", "okio-testing-support/src/nonWasmMain/kotlin/okio/TestingNonWasm.kt",
			"workingDirectory", "okio", "allowReadsWhileWriting", "property", 29, 43, 26},
	}
	for _, tc := range cases {
		t.Run(tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			path := filepath.Join(root, filepath.FromSlash(tc.source))
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("original Kotlin setter fixture unavailable or unsafe: %v", err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
			if tc.start < 1 || tc.end > len(lines) || tc.siblingLine > len(lines) ||
				!strings.Contains(lines[tc.start-1], "var ") ||
				!strings.Contains(lines[tc.start-1], tc.name+":") ||
				!strings.Contains(strings.Join(lines[tc.start:tc.end], "\n"), "set(value) {") ||
				!strings.Contains(lines[tc.siblingLine-1], tc.sibling) {
				t.Fatalf("original setter independent oracle changed: %s:%d-%d", tc.source, tc.start, tc.end)
			}
			expected := strings.TrimLeft(strings.Join(lines[tc.start-1:tc.end], "\n"), " \t")
			h := NewHandler([]string{root})
			ctx := context.Background()
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "kotlin",
				Presentation: "detailed", MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || outline.Truncated {
				t.Fatalf("original Kotlin setter outline failed: %v %+v", err, outline)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "kotlin",
				Query: tc.name, Match: "exact", FileDetails: "relevant", MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("original Kotlin setter find failed: %v %+v", err, found)
			}
			id := ""
			for _, sym := range found.Symbols {
				if sym.Name == tc.name && string(sym.Kind) == "property" &&
					sym.ParentQualifiedName == tc.owner &&
					sym.DeclarationRange.Start.Line == tc.start && sym.DeclarationRange.End.Line == tc.end {
					id = sym.ID
				}
			}
			if id == "" {
				t.Fatalf("original Kotlin setter property range missing: %s %+v", tc.source, found.Symbols)
			}
			hasProperty, hasSibling := false, false
			for _, sym := range outline.Symbols {
				if sym.ID == id {
					hasProperty = true
				}
				if sym.Name == tc.sibling && string(sym.Kind) == tc.siblingKind &&
					sym.ParentQualifiedName == tc.owner &&
					sym.DeclarationRange.Start.Line == tc.siblingLine {
					hasSibling = true
				}
			}
			if !hasProperty || !hasSibling {
				t.Fatalf("original Kotlin setter/sibling ownership failed: property=%t sibling=%t", hasProperty, hasSibling)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "kotlin", Encoding: found.Files[0].Encoding,
				SymbolID: id, SourceFingerprint: found.Files[0].SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || shown.Show == nil ||
				strings.ReplaceAll(shown.Show.Text, "\r\n", "\n") != expected {
				t.Fatalf("original Kotlin setter source mismatch: %s:%d err=%v shown=%+v", tc.source, tc.start, err, shown.Show)
			}
		})
	}
}

func TestKotlinOriginalMultilineClassOwnerNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original projects not configured")
	}
	cases := []struct {
		project, source, class, owner, member, memberKind string
		start, end, memberLine                            int
	}{
		{"project-kotlin-lysine-dev-okio", "okio/src/appleMain/kotlin/okio/ByteString.kt",
			"ByteString", "okio", "utf8", "property", 61, 229, 72},
		{"project-kotlin-square-moshi", "moshi-adapters/src/main/java/com/squareup/moshi/adapters/EnumJsonAdapter.kt",
			"EnumJsonAdapter", "com.squareup.moshi.adapters", "withUnknownFallback", "method", 38, 96, 59},
	}
	for _, tc := range cases {
		t.Run(tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			path := filepath.Join(root, filepath.FromSlash(tc.source))
			stat, err := os.Lstat(path)
			if err != nil || !stat.Mode().IsRegular() || stat.Size() > 1<<20 {
				t.Fatalf("original Kotlin class unavailable/unsafe: %v", err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
			if tc.start <= 0 || tc.end > len(lines) || tc.memberLine > len(lines) ||
				!strings.Contains(lines[tc.start-1], "class "+tc.class) ||
				!strings.Contains(lines[tc.memberLine-1], tc.member) ||
				strings.TrimSpace(lines[tc.end-1]) != "}" {
				t.Fatalf("original Kotlin multiline class oracle drift: %s:%d-%d", tc.source, tc.start, tc.end)
			}
			expected := strings.TrimLeft(strings.Join(lines[tc.start-1:tc.end], "\n"), " \t")
			h := NewHandler([]string{root})
			ctx := context.Background()
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "kotlin",
				Presentation: "detailed", MaxFiles: 1, MaxSymbols: 1024,
			})
			if err != nil || outline.Truncated {
				t.Fatalf("original Kotlin multiline class outline: %v %+v", err, outline)
			}
			qualified := tc.owner + "." + tc.class
			classID, memberFound := "", false
			for _, sym := range outline.Symbols {
				if sym.Name == tc.class && string(sym.Kind) == "class" &&
					sym.ParentQualifiedName == tc.owner &&
					sym.DeclarationRange.Start.Line == tc.start && sym.DeclarationRange.End.Line == tc.end {
					classID = sym.ID
				}
				if sym.Name == tc.member && string(sym.Kind) == tc.memberKind &&
					sym.ParentQualifiedName == qualified && sym.DeclarationRange.Start.Line == tc.memberLine {
					memberFound = true
				}
			}
			if classID == "" || !memberFound {
				t.Fatalf("original Kotlin class/member ownership: class=%s member=%t symbols=%+v", classID, memberFound, outline.Symbols)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "kotlin",
				Query: tc.class, Match: "exact", FileDetails: "relevant", MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("original Kotlin class find: %v %+v", err, found)
			}
			foundID := false
			for _, sym := range found.Symbols {
				if sym.ID == classID {
					foundID = true
				}
			}
			if !foundID {
				t.Fatalf("original Kotlin class not navigable by find: %s %+v", tc.class, found.Symbols)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "kotlin", Encoding: found.Files[0].Encoding,
				SymbolID: classID, SourceFingerprint: found.Files[0].SourceFingerprint, MaxBytes: 16384,
			})
			if err != nil || shown.Show == nil ||
				strings.ReplaceAll(shown.Show.Text, "\r\n", "\n") != expected {
				t.Fatalf("original Kotlin multiline class excerpt %s:%d error=%v shown=%+v", tc.source, tc.start, err, shown.Show)
			}
		})
	}
}

func TestKotlinOriginalKeywordPackageOwners(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original projects not configured")
	}
	cases := []struct {
		project, source, packageName, typeName, typeKind, member string
		packageLine, typeLine, typeEnd, memberLine               int
	}{
		{
			project:     "project-kotlin-lysine-dev-okio",
			source:      "okio/src/hashFunctions/kotlin/okio/internal/HashFunction.kt",
			packageName: "okio.internal", typeName: "HashFunction",
			typeKind: "interface", member: "digest",
			packageLine: 16, typeLine: 19, typeEnd: 27, memberLine: 26,
		},
		{
			project:     "project-kotlin-square-moshi",
			source:      "moshi/src/main/java/com/squareup/moshi/internal/JsonScope.kt",
			packageName: "com.squareup.moshi.internal", typeName: "JsonScope",
			typeKind: "class", member: "getPath",
			packageLine: 16, typeLine: 19, typeEnd: 85, memberLine: 56,
		},
	}
	for _, tc := range cases {
		t.Run(tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			path := filepath.Join(root, filepath.FromSlash(tc.source))
			stat, err := os.Lstat(path)
			if err != nil || !stat.Mode().IsRegular() || stat.Size() > 1<<20 {
				t.Fatalf("original Kotlin package fixture unavailable or unsafe: %v", err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
			if tc.packageLine < 1 || tc.typeEnd > len(lines) ||
				tc.memberLine > len(lines) ||
				strings.TrimSpace(lines[tc.packageLine-1]) != "package "+tc.packageName ||
				!strings.Contains(lines[tc.typeLine-1], tc.typeName) ||
				!strings.Contains(lines[tc.memberLine-1], tc.member+"(") ||
				strings.TrimSpace(lines[tc.typeEnd-1]) != "}" {
				t.Fatalf("independent Kotlin package oracle changed: %s", tc.source)
			}
			expectedClass := strings.TrimLeft(strings.Join(lines[tc.typeLine-1:tc.typeEnd], "\n"), " \t")
			h := NewHandler([]string{root})
			ctx := context.Background()
			_, outlined, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "kotlin",
				Presentation: "detailed", MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || outlined.Truncated {
				t.Fatalf("original Kotlin keyword package outline: %v %+v", err, outlined)
			}
			fullType := tc.packageName + "." + tc.typeName
			pkgID, typeID, memberPresent := "", "", false
			for _, sym := range outlined.Symbols {
				if string(sym.Kind) == "package" && sym.Name == tc.packageName &&
					sym.DeclarationRange.Start.Line == tc.packageLine &&
					sym.DeclarationRange.End.Line == tc.packageLine {
					pkgID = sym.ID
				}
				if sym.Name == tc.typeName && string(sym.Kind) == tc.typeKind &&
					sym.ParentQualifiedName == tc.packageName &&
					sym.DeclarationRange.Start.Line == tc.typeLine &&
					sym.DeclarationRange.End.Line == tc.typeEnd {
					typeID = sym.ID
				}
				if sym.Name == tc.member && string(sym.Kind) == "method" &&
					sym.ParentQualifiedName == fullType &&
					sym.DeclarationRange.Start.Line == tc.memberLine {
					memberPresent = true
				}
			}
			if pkgID == "" || typeID == "" || !memberPresent {
				t.Fatalf("original keyword package symbols missing: pkg=%t type=%t member=%t",
					pkgID != "", typeID != "", memberPresent)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "kotlin",
				Query: tc.typeName, Match: "exact", FileDetails: "relevant",
				MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("original Kotlin type find failed: %v %+v", err, found)
			}
			typeFound := false
			for _, sym := range found.Symbols {
				if sym.ID == typeID {
					typeFound = true
				}
			}
			if !typeFound {
				t.Fatalf("keyword package type not findable: %s", fullType)
			}
			for _, excerpt := range []struct{ id, expected string }{
				{pkgID, "package " + tc.packageName},
				{typeID, expectedClass},
			} {
				_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "show", Path: path, Language: "kotlin",
					Encoding: found.Files[0].Encoding, SymbolID: excerpt.id,
					SourceFingerprint: found.Files[0].SourceFingerprint, MaxBytes: 16384,
				})
				if err != nil || shown.Show == nil ||
					strings.ReplaceAll(shown.Show.Text, "\r\n", "\n") != excerpt.expected {
					t.Fatalf("original keyword package excerpt mismatch: %s err=%v shown=%+v",
						tc.source, err, shown.Show)
				}
			}
		})
	}
}
