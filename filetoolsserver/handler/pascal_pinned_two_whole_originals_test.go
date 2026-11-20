package handler

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The 114 supported declarations were frozen from both entire original sources
// before consulting the development parser. Physical line identity distinguishes
// interface prototypes from separate implementation bodies.
func TestPascalTwoWholePinnedOriginalOutlines(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_PASCAL_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two complete pinned Pascal originals not configured")
	}
	cases := []struct {
		file, blob string
		size       int64
		entries    string
	}{
		{file: "Log4Pascal.pas", blob: "5d2219d858cccc772e2d74d697a6ebc62491e18a", size: 5783, entries: "11|Log4Pascal|module|unit|Log4Pascal|6|1|Log4Pascal\n16|TLogTypes|enum|enum|Log4Pascal.TLogTypes|3|3|TLogTypes\n18|TLogger|class|class|Log4Pascal.TLogger|3|3|TLogger\n20|FFileName|field|field|Log4Pascal.TLogger.FFileName|5|5|FFileName\n21|FIsInit|field|field|Log4Pascal.TLogger.FIsInit|5|5|FIsInit\n22|FOutFile|field|field|Log4Pascal.TLogger.FOutFile|5|5|FOutFile\n23|FQuietMode|field|field|Log4Pascal.TLogger.FQuietMode|5|5|FQuietMode\n24|FQuietTypes|field|field|Log4Pascal.TLogger.FQuietTypes|5|5|FQuietTypes\n25|Initialize|method|procedure-declaration|Log4Pascal.TLogger.Initialize|15|5|Initialize\n26|CreateFoldersIfNecessary|method|procedure-declaration|Log4Pascal.TLogger.CreateFoldersIfNecessary|15|5|CreateFoldersIfNecessary\n27|Finalize|method|procedure-declaration|Log4Pascal.TLogger.Finalize|15|5|Finalize\n28|Write|method|procedure-declaration|Log4Pascal.TLogger.Write|15|5|Write\n30|TLogger|constructor|constructor-declaration|Log4Pascal.TLogger.TLogger|17|5|Create\n31|Destroy|destructor|destructor-declaration|Log4Pascal.TLogger.Destroy|16|5|Destroy\n33|FileName|property|property|Log4Pascal.TLogger.FileName|14|5|FileName\n35|SetQuietMode|method|procedure-declaration|Log4Pascal.TLogger.SetQuietMode|15|5|SetQuietMode\n36|DisableTraceLog|method|procedure-declaration|Log4Pascal.TLogger.DisableTraceLog|15|5|DisableTraceLog\n37|DisableDebugLog|method|procedure-declaration|Log4Pascal.TLogger.DisableDebugLog|15|5|DisableDebugLog\n38|DisableInfoLog|method|procedure-declaration|Log4Pascal.TLogger.DisableInfoLog|15|5|DisableInfoLog\n39|DisableWarningLog|method|procedure-declaration|Log4Pascal.TLogger.DisableWarningLog|15|5|DisableWarningLog\n40|DisableErrorLog|method|procedure-declaration|Log4Pascal.TLogger.DisableErrorLog|15|5|DisableErrorLog\n41|DisableFatalLog|method|procedure-declaration|Log4Pascal.TLogger.DisableFatalLog|15|5|DisableFatalLog\n43|SetNoisyMode|method|procedure-declaration|Log4Pascal.TLogger.SetNoisyMode|15|5|SetNoisyMode\n44|EnableTraceLog|method|procedure-declaration|Log4Pascal.TLogger.EnableTraceLog|15|5|EnableTraceLog\n45|EnableDebugLog|method|procedure-declaration|Log4Pascal.TLogger.EnableDebugLog|15|5|EnableDebugLog\n46|EnableInfoLog|method|procedure-declaration|Log4Pascal.TLogger.EnableInfoLog|15|5|EnableInfoLog\n47|EnableWarningLog|method|procedure-declaration|Log4Pascal.TLogger.EnableWarningLog|15|5|EnableWarningLog\n48|EnableErrorLog|method|procedure-declaration|Log4Pascal.TLogger.EnableErrorLog|15|5|EnableErrorLog\n49|EnableFatalLog|method|procedure-declaration|Log4Pascal.TLogger.EnableFatalLog|15|5|EnableFatalLog\n51|Clear|method|procedure-declaration|Log4Pascal.TLogger.Clear|15|5|Clear\n53|Trace|method|procedure-declaration|Log4Pascal.TLogger.Trace|15|5|Trace\n54|Debug|method|procedure-declaration|Log4Pascal.TLogger.Debug|15|5|Debug\n55|Info|method|procedure-declaration|Log4Pascal.TLogger.Info|15|5|Info\n56|Warning|method|procedure-declaration|Log4Pascal.TLogger.Warning|15|5|Warning\n57|Error|method|procedure-declaration|Log4Pascal.TLogger.Error|15|5|Error\n58|Fatal|method|procedure-declaration|Log4Pascal.TLogger.Fatal|15|5|Fatal\n62|Logger|variable|var|Log4Pascal.Logger|3|3|Logger\n72|FORMAT_LOG|constant|const|Log4Pascal.FORMAT_LOG|3|3|FORMAT_LOG\n73|PREFIX_TRACE|constant|const|Log4Pascal.PREFIX_TRACE|3|3|PREFIX_TRACE\n74|PREFIX_DEBUG|constant|const|Log4Pascal.PREFIX_DEBUG|3|3|PREFIX_DEBUG\n75|PREFIX_INFO|constant|const|Log4Pascal.PREFIX_INFO|3|3|PREFIX_INFO\n76|PREFIX_WARN|constant|const|Log4Pascal.PREFIX_WARN|3|3|PREFIX_WARN\n77|PREFIX_ERROR|constant|const|Log4Pascal.PREFIX_ERROR|3|3|PREFIX_ERROR\n78|PREFIX_FATAL|constant|const|Log4Pascal.PREFIX_FATAL|3|3|PREFIX_FATAL\n82|Clear|method|procedure-definition|Log4Pascal.TLogger.Clear|19|1|Clear\n95|TLogger|constructor|constructor-definition|Log4Pascal.TLogger.TLogger|21|1|Create\n103|CreateFoldersIfNecessary|method|procedure-definition|Log4Pascal.TLogger.CreateFoldersIfNecessary|19|1|CreateFoldersIfNecessary\n105|FilePath|variable|var|Log4Pascal.TLogger.CreateFoldersIfNecessary.FilePath|3|3|FilePath\n106|FullApplicationPath|variable|var|Log4Pascal.TLogger.CreateFoldersIfNecessary.FullApplicationPath|3|3|FullApplicationPath\n118|Debug|method|procedure-definition|Log4Pascal.TLogger.Debug|19|1|Debug\n129|Destroy|destructor|destructor-definition|Log4Pascal.TLogger.Destroy|20|1|Destroy\n135|DisableDebugLog|method|procedure-definition|Log4Pascal.TLogger.DisableDebugLog|19|1|DisableDebugLog\n140|DisableErrorLog|method|procedure-definition|Log4Pascal.TLogger.DisableErrorLog|19|1|DisableErrorLog\n145|DisableFatalLog|method|procedure-definition|Log4Pascal.TLogger.DisableFatalLog|19|1|DisableFatalLog\n150|DisableInfoLog|method|procedure-definition|Log4Pascal.TLogger.DisableInfoLog|19|1|DisableInfoLog\n155|DisableTraceLog|method|procedure-definition|Log4Pascal.TLogger.DisableTraceLog|19|1|DisableTraceLog\n160|DisableWarningLog|method|procedure-definition|Log4Pascal.TLogger.DisableWarningLog|19|1|DisableWarningLog\n165|EnableDebugLog|method|procedure-definition|Log4Pascal.TLogger.EnableDebugLog|19|1|EnableDebugLog\n170|EnableErrorLog|method|procedure-definition|Log4Pascal.TLogger.EnableErrorLog|19|1|EnableErrorLog\n175|EnableFatalLog|method|procedure-definition|Log4Pascal.TLogger.EnableFatalLog|19|1|EnableFatalLog\n180|EnableInfoLog|method|procedure-definition|Log4Pascal.TLogger.EnableInfoLog|19|1|EnableInfoLog\n185|EnableTraceLog|method|procedure-definition|Log4Pascal.TLogger.EnableTraceLog|19|1|EnableTraceLog\n190|EnableWarningLog|method|procedure-definition|Log4Pascal.TLogger.EnableWarningLog|19|1|EnableWarningLog\n195|Error|method|procedure-definition|Log4Pascal.TLogger.Error|19|1|Error\n201|Fatal|method|procedure-definition|Log4Pascal.TLogger.Fatal|19|1|Fatal\n207|Finalize|method|procedure-definition|Log4Pascal.TLogger.Finalize|19|1|Finalize\n215|Initialize|method|procedure-definition|Log4Pascal.TLogger.Initialize|19|1|Initialize\n234|Info|method|procedure-definition|Log4Pascal.TLogger.Info|19|1|Info\n240|SetNoisyMode|method|procedure-definition|Log4Pascal.TLogger.SetNoisyMode|19|1|SetNoisyMode\n245|SetQuietMode|method|procedure-definition|Log4Pascal.TLogger.SetQuietMode|19|1|SetQuietMode\n250|Trace|method|procedure-definition|Log4Pascal.TLogger.Trace|19|1|Trace\n256|Warning|method|procedure-definition|Log4Pascal.TLogger.Warning|19|1|Warning\n262|Write|method|procedure-definition|Log4Pascal.TLogger.Write|19|1|Write\n264|FORMAT_DATETIME_DEFAULT|constant|const|Log4Pascal.TLogger.Write.FORMAT_DATETIME_DEFAULT|3|3|FORMAT_DATETIME_DEFAULT"},
		{file: "BitMath.pas", blob: "b5a816608c8620961ca3d94b938bc262c09b61b4", size: 6302, entries: "10|BitMath|module|unit|BitMath|6|1|BitMath\n44|BitShift|function|function-declaration|BitMath.BitShift|10|1|BitShift\n50|HighestBit|function|function-declaration|BitMath.HighestBit|10|1|HighestBit\n55|LowestBit|function|function-declaration|BitMath.LowestBit|10|1|LowestBit\n60|FloorLog2|function|function-declaration|BitMath.FloorLog2|10|1|FloorLog2\n65|Ceil2p|function|function-declaration|BitMath.Ceil2p|10|1|Ceil2p\n70|Floor2p|function|function-declaration|BitMath.Floor2p|10|1|Floor2p\n75|BitCount|function|function-declaration|BitMath.BitCount|10|1|BitCount\n80|BitCount64|function|function-declaration|BitMath.BitCount64|10|1|BitCount64\n85|IntNBytes|constant|const|BitMath.IntNBytes|7|7|IntNBytes\n86|IntHighByte|constant|const|BitMath.IntHighByte|7|7|IntHighByte\n87|Int64NBytes|constant|const|BitMath.Int64NBytes|7|7|Int64NBytes\n88|Int64HighByte|constant|const|BitMath.Int64HighByte|7|7|Int64HighByte\n89|ByteNBits|constant|const|BitMath.ByteNBits|7|7|ByteNBits\n90|ByteHighBit|constant|const|BitMath.ByteHighBit|7|7|ByteHighBit\n91|IntNBits|constant|const|BitMath.IntNBits|7|7|IntNBits\n92|IntHighBit|constant|const|BitMath.IntHighBit|7|7|IntHighBit\n94|TIntBytes|alias|type-alias|BitMath.TIntBytes|7|7|TIntBytes\n95|TInt64Bytes|alias|type-alias|BitMath.TInt64Bytes|7|7|TInt64Bytes\n96|PIntBytes|alias|type-alias|BitMath.PIntBytes|7|7|PIntBytes\n97|PInt64Bytes|alias|type-alias|BitMath.PInt64Bytes|7|7|PInt64Bytes\n99|ByteBitCount|variable|var|BitMath.ByteBitCount|7|7|ByteBitCount\n100|RevBytes|variable|var|BitMath.RevBytes|7|7|RevBytes\n107|BitShift|function|function-definition|BitMath.BitShift|10|1|BitShift\n119|HighestBit|function|function-definition|BitMath.HighestBit|10|1|HighestBit\n132|LowestBit|function|function-definition|BitMath.LowestBit|10|1|LowestBit\n145|FloorLog2|function|function-definition|BitMath.FloorLog2|10|1|FloorLog2\n154|Ceil2p|function|function-definition|BitMath.Ceil2p|10|1|Ceil2p\n163|Floor2p|function|function-definition|BitMath.Floor2p|10|1|Floor2p\n172|BitCount|function|function-definition|BitMath.BitCount|10|1|BitCount\n176|PtrBytes|variable|var|BitMath.BitCount.PtrBytes|10|10|PtrBytes\n188|BitCount64|function|function-definition|BitMath.BitCount64|10|1|BitCount64\n192|PtrBytes|variable|var|BitMath.BitCount64.PtrBytes|10|10|PtrBytes\n211|PrecalcRevBytes|function|procedure-definition|BitMath.PrecalcRevBytes|11|1|PrecalcRevBytes\n213|i|variable|var|BitMath.PrecalcRevBytes.i|10|10|i\n213|j|variable|var|BitMath.PrecalcRevBytes.j|13|10|j\n213|b|variable|var|BitMath.PrecalcRevBytes.b|16|10|b\n214|r|variable|var|BitMath.PrecalcRevBytes.r|10|10|r\n230|PrecalcBitCount|function|procedure-definition|BitMath.PrecalcBitCount|11|1|PrecalcBitCount\n232|i|variable|var|BitMath.PrecalcBitCount.i|10|10|i"},
	}
	h := NewHandler([]string{root})
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			st, err := os.Lstat(path)
			if err != nil || !st.Mode().IsRegular() || st.Size() != tc.size {
				t.Fatalf("whole original unavailable or changed: %+v %v", st, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			hash := sha1.New()
			_, _ = fmt.Fprintf(hash, "blob %d%c", len(data), 0)
			_, _ = hash.Write(data)
			if hex.EncodeToString(hash.Sum(nil)) != tc.blob {
				t.Fatal("raw original Git blob mismatch")
			}
			entries := strings.Split(tc.entries, "\n")
			ctx := context.Background()
			failed, out, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "pascal", Encoding: "utf-8", IncludeSignatures: true, MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || failed != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != 1 || len(out.Files) != 1 || len(out.Symbols) != len(entries) {
				t.Fatalf("complete source-first inventory expected=%d actual=%d err=%v failed=%+v", len(entries), len(out.Symbols), err, failed)
			}
			physical := make(map[string]int, len(out.Symbols))
			ids := make(map[string]bool, len(out.Symbols))
			for i, s := range out.Symbols {
				key := fmt.Sprintf("%s@%d", s.QualifiedName, s.NameRange.Start.Line)
				if _, dup := physical[key]; dup {
					t.Errorf("duplicate physical symbol %s", key)
				}
				physical[key] = i
				if s.ID == "" || ids[s.ID] {
					t.Errorf("missing/reused identity %+v", s)
				}
				ids[s.ID] = true
			}
			seen := make(map[string]bool, len(entries))
			for _, entry := range entries {
				f := strings.Split(entry, "|")
				if len(f) != 8 {
					t.Fatalf("invalid frozen source-first row %q", entry)
				}
				line, e := strconv.Atoi(f[0])
				if e != nil {
					t.Fatal(e)
				}
				nameCol, e := strconv.Atoi(f[5])
				if e != nil {
					t.Fatal(e)
				}
				declCol, e := strconv.Atoi(f[6])
				if e != nil {
					t.Fatal(e)
				}
				key := fmt.Sprintf("%s@%d", f[4], line)
				if seen[key] {
					t.Fatalf("duplicate independent source-first row %s", key)
				}
				seen[key] = true
				i, ok := physical[key]
				if !ok {
					t.Errorf("missing original declaration %s", key)
					continue
				}
				s := out.Symbols[i]
				if s.Name != f[1] || string(s.Kind) != f[2] || s.NativeKind != f[3] ||
					s.QualifiedName != f[4] || s.NameRange.Start.Line != line ||
					s.NameRange.Start.Column != nameCol || s.NameRange.End.Line != line ||
					s.NameRange.End.Column != nameCol+len(f[7]) ||
					s.DeclarationRange.Start.Line != line || s.DeclarationRange.Start.Column != declCol ||
					s.DeclarationRange.End.Line < line {
					t.Errorf("source-first Pascal identity/range mismatch expected=%q actual=%+v", entry, s)
				}
				if s.ParentID != "" && !ids[s.ParentID] {
					t.Errorf("broken parent ID reference %+v", s)
				}
			}
			if out.Files[0].SourceFingerprint == "" || out.Files[0].SourceBytes != tc.size {
				t.Fatal("original fingerprint/byte count missing")
			}
			module := strings.TrimSuffix(tc.file, filepath.Ext(tc.file))
			failed, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "pascal", Encoding: "utf-8",
				Query: module, Match: "exact", MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || failed != nil || found.Truncated || len(found.Symbols) != 1 || found.Symbols[0].Name != module {
				t.Fatalf("exact module find: err=%v failed=%+v found=%+v", err, failed, found.Symbols)
			}
			failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "pascal", Encoding: "utf-8",
				SymbolID: found.Symbols[0].ID, SourceFingerprint: out.Files[0].SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || failed != nil || shown.Show == nil || !strings.Contains(strings.ToLower(shown.Show.Text), strings.ToLower(module)) {
				t.Fatalf("fingerprint-bound original show: err=%v failed=%+v show=%+v", err, failed, shown.Show)
			}
			failed, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path}, Language: "pascal",
				Encoding: "utf-8", Query: module, Match: "exact", MaxFiles: 1, MaxResults: 256,
			})
			if err != nil || failed != nil || query.Search == nil || len(query.Search.Matches) != 1 ||
				query.Search.Matches[0].SymbolID != found.Symbols[0].ID {
				t.Fatalf("original structural query: err=%v failed=%+v result=%+v", err, failed, query)
			}
		})
	}
}
