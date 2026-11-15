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
