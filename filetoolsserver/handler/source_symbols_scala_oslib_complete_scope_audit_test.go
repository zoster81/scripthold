package handler

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestScalaOsLibSelectedOriginalCompleteMemberInventory compares a complete
// independently source-read selection of package, type and type-member
// declarations across four os-lib originals. Local method/test-body variables
// and functions, anonymous-class bodies, same-line constructor parameter val
// fields, and secondary names of destructuring val patterns are outside this
// selected source projection. The first destructured val is indexed; this does
// not imply complete Scala syntax recognition or compiler/type semantics.
func TestScalaOsLibSelectedOriginalCompleteMemberInventory(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("original Scala os-lib corpus not configured")
	}
	project := filepath.Join(root, "scala61-os-lib-audit-selected")
	type decl struct {
		file                       string
		line                       int
		name, kind, native, parent string
	}
	oracle := []decl{
		{"os/src/Path.scala", 1, "os", "package", "package", ""},
		{"os/src/Path.scala", 14, "PathChunk", "trait", "trait", "os"},
		{"os/src/Path.scala", 18, "StringPathChunkConversion", "trait", "trait", "os"},
		{"os/src/Path.scala", 24, "PathChunk", "module", "object", "os"},
		{"os/src/Path.scala", 117, "BasePath", "trait", "trait", "os"},
		{"os/src/Path.scala", 179, "BasePath", "module", "object", "os"},
		{"os/src/Path.scala", 221, "SegmentedPath", "trait", "trait", "os"},
		{"os/src/Path.scala", 240, "BasePathImpl", "trait", "trait", "os"},
		{"os/src/Path.scala", 265, "PathError", "module", "object", "os"},
		{"os/src/Path.scala", 292, "FilePath", "trait", "trait", "os"},
		{"os/src/Path.scala", 297, "FilePath", "module", "object", "os"},
		{"os/src/Path.scala", 317, "RelPath", "class", "class", "os"},
		{"os/src/Path.scala", 356, "RelPath", "module", "object", "os"},
		{"os/src/Path.scala", 389, "SubPath", "class", "class", "os"},
		{"os/src/Path.scala", 417, "SubPath", "module", "object", "os"},
		{"os/src/Path.scala", 444, "Path", "module", "object", "os"},
		{"os/src/Path.scala", 571, "ReadablePath", "trait", "trait", "os"},
		{"os/src/Path.scala", 581, "Path", "class", "class", "os"},
		{"os/src/Path.scala", 654, "PathConvertible", "trait", "trait", "os"},
		{"os/src/Path.scala", 659, "PathConvertible", "module", "object", "os"},
		{"os/src/Path.scala", 25, "segmentsFromString", "method", "def", "os.PathChunk"},
		{"os/src/Path.scala", 31, "reduceUps", "method", "def", "os.PathChunk"},
		{"os/src/Path.scala", 39, "segmentsFromStringLiteralValidation", "method", "def", "os.PathChunk"},
		{"os/src/Path.scala", 50, "validLiteralSegments", "method", "def", "os.PathChunk"},
		{"os/src/Path.scala", 58, "RelPathChunk", "class", "class", "os.PathChunk"},
		{"os/src/Path.scala", 64, "SubPathChunk", "class", "class", "os.PathChunk"},
		{"os/src/Path.scala", 71, "StringPathChunkInternal", "class", "class", "os.PathChunk"},
		{"os/src/Path.scala", 79, "StringPathChunk", "class", "class", "os.PathChunk"},
		{"os/src/Path.scala", 82, "StringPathChunk", "method", "def", "os.PathChunk"},
		{"os/src/Path.scala", 84, "SymbolPathChunk", "class", "class", "os.PathChunk"},
		{"os/src/Path.scala", 90, "ArrayPathChunk", "class", "class", "os.PathChunk"},
		{"os/src/Path.scala", 97, "SeqPathChunk", "class", "class", "os.PathChunk"},
		{"os/src/Path.scala", 59, "segments", "method", "def", "os.PathChunk.RelPathChunk"},
		{"os/src/Path.scala", 60, "ups", "method", "def", "os.PathChunk.RelPathChunk"},
		{"os/src/Path.scala", 61, "toString", "method", "def", "os.PathChunk.RelPathChunk"},
		{"os/src/Path.scala", 65, "segments", "method", "def", "os.PathChunk.SubPathChunk"},
		{"os/src/Path.scala", 66, "ups", "method", "def", "os.PathChunk.SubPathChunk"},
		{"os/src/Path.scala", 67, "toString", "method", "def", "os.PathChunk.SubPathChunk"},
		{"os/src/Path.scala", 73, "segments", "method", "def", "os.PathChunk.StringPathChunkInternal"},
		{"os/src/Path.scala", 74, "ups", "method", "def", "os.PathChunk.StringPathChunkInternal"},
		{"os/src/Path.scala", 75, "toString", "method", "def", "os.PathChunk.StringPathChunkInternal"},
		{"os/src/Path.scala", 86, "segments", "method", "def", "os.PathChunk.SymbolPathChunk"},
		{"os/src/Path.scala", 87, "ups", "method", "def", "os.PathChunk.SymbolPathChunk"},
		{"os/src/Path.scala", 88, "toString", "method", "def", "os.PathChunk.SymbolPathChunk"},
		{"os/src/Path.scala", 91, "inner", "field", "val", "os.PathChunk.ArrayPathChunk"},
		{"os/src/Path.scala", 92, "segments", "method", "def", "os.PathChunk.ArrayPathChunk"},
		{"os/src/Path.scala", 93, "ups", "method", "def", "os.PathChunk.ArrayPathChunk"},
		{"os/src/Path.scala", 95, "toString", "method", "def", "os.PathChunk.ArrayPathChunk"},
		{"os/src/Path.scala", 99, "segments0", "field", "var", "os.PathChunk.SeqPathChunk"},
		{"os/src/Path.scala", 101, "ups0", "field", "var", "os.PathChunk.SeqPathChunk"},
		{"os/src/Path.scala", 103, "rel", "field", "val", "os.PathChunk.SeqPathChunk"},
		{"os/src/Path.scala", 106, "toString", "method", "def", "os.PathChunk.SeqPathChunk"},
		{"os/src/Path.scala", 118, "ThisType", "type", "type", "os.BasePath"},
		{"os/src/Path.scala", 125, "/", "method", "def", "os.BasePath"},
		{"os/src/Path.scala", 137, "relativeTo", "method", "def", "os.BasePath"},
		{"os/src/Path.scala", 143, "subRelativeTo", "method", "def", "os.BasePath"},
		{"os/src/Path.scala", 148, "startsWith", "method", "def", "os.BasePath"},
		{"os/src/Path.scala", 153, "endsWith", "method", "def", "os.BasePath"},
		{"os/src/Path.scala", 159, "last", "method", "def", "os.BasePath"},
		{"os/src/Path.scala", 165, "ext", "method", "def", "os.BasePath"},
		{"os/src/Path.scala", 170, "baseName", "method", "def", "os.BasePath"},
		{"os/src/Path.scala", 175, "segments", "method", "def", "os.BasePath"},
		{"os/src/Path.scala", 180, "checkSegment", "method", "def", "os.BasePath"},
		{"os/src/Path.scala", 216, "chunkify", "method", "def", "os.BasePath"},
		{"os/src/Path.scala", 222, "make", "method", "def", "os.SegmentedPath"},
		{"os/src/Path.scala", 227, "segments", "method", "def", "os.SegmentedPath"},
		{"os/src/Path.scala", 229, "/", "method", "def", "os.SegmentedPath"},
		{"os/src/Path.scala", 234, "endsWith", "method", "def", "os.SegmentedPath"},
		{"os/src/Path.scala", 241, "/", "method", "def", "os.BasePathImpl"},
		{"os/src/Path.scala", 243, "ext", "method", "def", "os.BasePathImpl"},
		{"os/src/Path.scala", 254, "baseName", "method", "def", "os.BasePathImpl"},
		{"os/src/Path.scala", 260, "last", "method", "def", "os.BasePathImpl"},
		{"os/src/Path.scala", 262, "lastOpt", "method", "def", "os.BasePathImpl"},
		{"os/src/Path.scala", 266, "IAE", "type", "type", "os.PathError"},
		{"os/src/Path.scala", 267, "errorMsg", "method", "def", "os.PathError"},
		{"os/src/Path.scala", 270, "InvalidSegment", "class", "class", "os.PathError"},
		{"os/src/Path.scala", 272, "AbsolutePathOutsideRoot", "module", "object", "os.PathError"},
		{"os/src/Path.scala", 275, "NoRelativePath", "class", "class", "os.PathError"},
		{"os/src/Path.scala", 278, "LastOnEmptyPath", "class", "class", "os.PathError"},
		{"os/src/Path.scala", 281, "NonCanonicalLiteral", "class", "class", "os.PathError"},
		{"os/src/Path.scala", 293, "toNIO", "method", "def", "os.FilePath"},
		{"os/src/Path.scala", 294, "resolveFrom", "method", "def", "os.FilePath"},
		{"os/src/Path.scala", 299, "apply", "method", "def", "os.FilePath"},
		{"os/src/Path.scala", 319, "lastOpt", "method", "def", "os.RelPath"},
		{"os/src/Path.scala", 320, "segments", "field", "val", "os.RelPath"},
		{"os/src/Path.scala", 321, "ThisType", "type", "type", "os.RelPath"},
		{"os/src/Path.scala", 323, "make", "method", "def", "os.RelPath"},
		{"os/src/Path.scala", 327, "relativeTo", "method", "def", "os.RelPath"},
		{"os/src/Path.scala", 333, "startsWith", "method", "def", "os.RelPath"},
		{"os/src/Path.scala", 337, "toString", "method", "def", "os.RelPath"},
		{"os/src/Path.scala", 339, "hashCode", "method", "def", "os.RelPath"},
		{"os/src/Path.scala", 340, "equals", "method", "def", "os.RelPath"},
		{"os/src/Path.scala", 346, "toNIO", "method", "def", "os.RelPath"},
		{"os/src/Path.scala", 348, "asSubPath", "method", "def", "os.RelPath"},
		{"os/src/Path.scala", 353, "resolveFrom", "method", "def", "os.RelPath"},
		{"os/src/Path.scala", 358, "apply", "method", "def", "os.RelPath"},
		{"os/src/Path.scala", 368, "apply", "method", "def", "os.RelPath"},
		{"os/src/Path.scala", 374, "relPathOrdering", "field", "val", "os.RelPath"},
		{"os/src/Path.scala", 377, "up", "field", "val", "os.RelPath"},
		{"os/src/Path.scala", 378, "rel", "field", "val", "os.RelPath"},
		{"os/src/Path.scala", 379, "SubRelPath", "method", "def", "os.RelPath"},
		{"os/src/Path.scala", 380, "fromStringSegments", "method", "def", "os.RelPath"},
		{"os/src/Path.scala", 391, "lastOpt", "method", "def", "os.SubPath"},
		{"os/src/Path.scala", 392, "segments", "field", "val", "os.SubPath"},
		{"os/src/Path.scala", 393, "ThisType", "type", "type", "os.SubPath"},
		{"os/src/Path.scala", 394, "make", "method", "def", "os.SubPath"},
		{"os/src/Path.scala", 399, "relativeTo", "method", "def", "os.SubPath"},
		{"os/src/Path.scala", 402, "startsWith", "method", "def", "os.SubPath"},
		{"os/src/Path.scala", 404, "toString", "method", "def", "os.SubPath"},
		{"os/src/Path.scala", 405, "hashCode", "method", "def", "os.SubPath"},
		{"os/src/Path.scala", 406, "equals", "method", "def", "os.SubPath"},
		{"os/src/Path.scala", 412, "toNIO", "method", "def", "os.SubPath"},
		{"os/src/Path.scala", 414, "resolveFrom", "method", "def", "os.SubPath"},
		{"os/src/Path.scala", 418, "relativeTo0", "method", "def", "os.SubPath"},
		{"os/src/Path.scala", 430, "apply", "method", "def", "os.SubPath"},
		{"os/src/Path.scala", 432, "apply", "method", "def", "os.SubPath"},
		{"os/src/Path.scala", 438, "subPathOrdering", "field", "val", "os.SubPath"},
		{"os/src/Path.scala", 441, "sub", "field", "val", "os.SubPath"},
		{"os/src/Path.scala", 448, "Serializer", "trait", "trait", "os.Path"},
		{"os/src/Path.scala", 460, "pathSerializer", "field", "val", "os.Path"},
		{"os/src/Path.scala", 464, "defaultPathSerializer", "module", "object", "os.Path"},
		{"os/src/Path.scala", 480, "apply", "method", "def", "os.Path"},
		{"os/src/Path.scala", 490, "expandUser", "method", "def", "os.Path"},
		{"os/src/Path.scala", 499, "apply", "method", "def", "os.Path"},
		{"os/src/Path.scala", 500, "apply", "method", "def", "os.Path"},
		{"os/src/Path.scala", 514, "pathOrdering", "field", "val", "os.Path"},
		{"os/src/Path.scala", 550, "driveRelative", "method", "def", "os.Path"},
		{"os/src/Path.scala", 565, "driveRoot", "field", "val", "os.Path"},
		{"os/src/Path.scala", 586, "toSource", "method", "def", "os.Path"},
		{"os/src/Path.scala", 591, "root", "method", "def", "os.Path"},
		{"os/src/Path.scala", 592, "fileSystem", "method", "def", "os.Path"},
		{"os/src/Path.scala", 594, "segments", "method", "def", "os.Path"},
		{"os/src/Path.scala", 595, "getSegment", "method", "def", "os.Path"},
		{"os/src/Path.scala", 596, "segmentCount", "method", "def", "os.Path"},
		{"os/src/Path.scala", 597, "ThisType", "type", "type", "os.Path"},
		{"os/src/Path.scala", 599, "lastOpt", "method", "def", "os.Path"},
		{"os/src/Path.scala", 601, "/", "method", "def", "os.Path"},
		{"os/src/Path.scala", 608, "toString", "method", "def", "os.Path"},
		{"os/src/Path.scala", 613, "toPlatformString", "method", "def", "os.Path"},
		{"os/src/Path.scala", 615, "equals", "method", "def", "os.Path"},
		{"os/src/Path.scala", 619, "hashCode", "method", "def", "os.Path"},
		{"os/src/Path.scala", 621, "startsWith", "method", "def", "os.Path"},
		{"os/src/Path.scala", 623, "endsWith", "method", "def", "os.Path"},
		{"os/src/Path.scala", 625, "relativeTo", "method", "def", "os.Path"},
		{"os/src/Path.scala", 644, "toIO", "method", "def", "os.Path"},
		{"os/src/Path.scala", 645, "toNIO", "method", "def", "os.Path"},
		{"os/src/Path.scala", 646, "toURI", "method", "def", "os.Path"},
		{"os/src/Path.scala", 647, "toURL", "method", "def", "os.Path"},
		{"os/src/Path.scala", 649, "resolveFrom", "method", "def", "os.Path"},
		{"os/src/Path.scala", 651, "getInputStream", "method", "def", "os.Path"},
		{"os/src/Path.scala", 449, "serializeString", "method", "def", "os.Path.Serializer"},
		{"os/src/Path.scala", 450, "serializeFile", "method", "def", "os.Path.Serializer"},
		{"os/src/Path.scala", 451, "serializePath", "method", "def", "os.Path.Serializer"},
		{"os/src/Path.scala", 452, "deserialize", "method", "def", "os.Path.Serializer"},
		{"os/src/Path.scala", 453, "deserialize", "method", "def", "os.Path.Serializer"},
		{"os/src/Path.scala", 454, "deserialize", "method", "def", "os.Path.Serializer"},
		{"os/src/Path.scala", 455, "deserialize", "method", "def", "os.Path.Serializer"},
		{"os/src/Path.scala", 465, "serializeString", "method", "def", "os.Path.defaultPathSerializer"},
		{"os/src/Path.scala", 466, "serializeFile", "method", "def", "os.Path.defaultPathSerializer"},
		{"os/src/Path.scala", 467, "serializePath", "method", "def", "os.Path.defaultPathSerializer"},
		{"os/src/Path.scala", 468, "deserialize", "method", "def", "os.Path.defaultPathSerializer"},
		{"os/src/Path.scala", 469, "deserialize", "method", "def", "os.Path.defaultPathSerializer"},
		{"os/src/Path.scala", 470, "deserialize", "method", "def", "os.Path.defaultPathSerializer"},
		{"os/src/Path.scala", 471, "deserialize", "method", "def", "os.Path.defaultPathSerializer"},
		{"os/src/Path.scala", 572, "toSource", "method", "def", "os.ReadablePath"},
		{"os/src/Path.scala", 574, "getInputStream", "method", "def", "os.ReadablePath"},
		{"os/src/Path.scala", 655, "apply", "method", "def", "os.PathConvertible"},
		{"os/src/Path.scala", 656, "isCustomFs", "method", "def", "os.PathConvertible"},
		{"os/src/Path.scala", 660, "StringConvertible", "module", "object", "os.PathConvertible"},
		{"os/src/Path.scala", 663, "JavaIoFileConvertible", "module", "object", "os.PathConvertible"},
		{"os/src/Path.scala", 666, "NioPathConvertible", "module", "object", "os.PathConvertible"},
		{"os/src/Path.scala", 671, "UriPathConvertible", "module", "object", "os.PathConvertible"},
		{"os/src/Path.scala", 661, "apply", "method", "def", "os.PathConvertible.StringConvertible"},
		{"os/src/Path.scala", 664, "apply", "method", "def", "os.PathConvertible.JavaIoFileConvertible"},
		{"os/src/Path.scala", 667, "apply", "method", "def", "os.PathConvertible.NioPathConvertible"},
		{"os/src/Path.scala", 668, "isCustomFs", "method", "def", "os.PathConvertible.NioPathConvertible"},
		{"os/src/Path.scala", 672, "apply", "method", "def", "os.PathConvertible.UriPathConvertible"},
		{"os/src/GlobInterpolator.scala", 1, "os", "package", "package", ""},
		{"os/src/GlobInterpolator.scala", 3, "GlobInterpolator", "module", "object", "os"},
		{"os/src/GlobInterpolator.scala", 17, "GlobInterpolator", "class", "class", "os"},
		{"os/src/GlobInterpolator.scala", 4, "Interped", "class", "class", "os.GlobInterpolator"},
		{"os/src/GlobInterpolator.scala", 18, "g", "method", "def", "os.GlobInterpolator"},
		{"os/src/GlobInterpolator.scala", 19, "g", "method", "def", "os.GlobInterpolator"},
		{"os/src/GlobInterpolator.scala", 5, "unapplySeq", "method", "def", "os.GlobInterpolator.Interped"},
		{"os/src/ProcessOps.scala", 1, "os", "package", "package", ""},
		{"os/src/ProcessOps.scala", 10, "call", "module", "object", "os"},
		{"os/src/ProcessOps.scala", 78, "spawn", "module", "object", "os"},
		{"os/src/ProcessOps.scala", 155, "proc", "class", "class", "os"},
		{"os/src/ProcessOps.scala", 396, "ProcGroup", "class", "class", "os"},
		{"os/src/ProcessOps.scala", 628, "ProcessOps", "module", "object", "os"},
		{"os/src/ProcessOps.scala", 15, "apply", "method", "def", "os.call"},
		{"os/src/ProcessOps.scala", 47, "apply", "method", "def", "os.call"},
		{"os/src/ProcessOps.scala", 83, "apply", "method", "def", "os.spawn"},
		{"os/src/ProcessOps.scala", 111, "apply", "method", "def", "os.spawn"},
		{"os/src/ProcessOps.scala", 156, "commandChunks", "method", "def", "os.proc"},
		{"os/src/ProcessOps.scala", 192, "call", "method", "def", "os.proc"},
		{"os/src/ProcessOps.scala", 236, "call", "method", "def", "os.proc"},
		{"os/src/ProcessOps.scala", 260, "call", "method", "def", "os.proc"},
		{"os/src/ProcessOps.scala", 295, "spawn", "method", "def", "os.proc"},
		{"os/src/ProcessOps.scala", 362, "spawn", "method", "def", "os.proc"},
		{"os/src/ProcessOps.scala", 387, "pipeTo", "method", "def", "os.proc"},
		{"os/src/ProcessOps.scala", 399, "isWindows", "field", "val", "os.ProcGroup"},
		{"os/src/ProcessOps.scala", 442, "call", "method", "def", "os.ProcGroup"},
		{"os/src/ProcessOps.scala", 486, "call", "method", "def", "os.ProcGroup"},
		{"os/src/ProcessOps.scala", 537, "spawn", "method", "def", "os.ProcGroup"},
		{"os/src/ProcessOps.scala", 583, "wrapWithBrokenPipeHandler", "method", "def", "os.ProcGroup"},
		{"os/src/ProcessOps.scala", 624, "pipeTo", "method", "def", "os.ProcGroup"},
		{"os/src/ProcessOps.scala", 629, "spawnHook", "field", "val", "os.ProcessOps"},
		{"os/src/ProcessOps.scala", 630, "buildProcess", "method", "def", "os.ProcessOps"},
		{"os/test/src/PathTests.scala", 1, "test.os", "package", "package", ""},
		{"os/test/src/PathTests.scala", 10, "PathTests", "module", "object", "test.os"},
		{"os/test/src/PathTests.scala", 11, "nonCanonicalLiteral", "method", "def", "test.os.PathTests"},
		{"os/test/src/PathTests.scala", 13, "removeLiteralErr", "method", "def", "test.os.PathTests"},
		{"os/test/src/PathTests.scala", 16, "tests", "field", "val", "test.os.PathTests"},
		{"os/test/src/PathTests.scala", 579, "sameFile", "method", "def", "test.os.PathTests"},
		{"os/test/src/PathTests.scala", 582, "sameFile", "method", "def", "test.os.PathTests"},
		{"os/test/src/PathTests.scala", 585, "sameFile", "method", "def", "test.os.PathTests"},
		{"os/test/src/PathTests.scala", 588, "posix", "method", "def", "test.os.PathTests"},
		{"os/test/src/PathTests.scala", 589, "posix", "method", "def", "test.os.PathTests"},
		{"os/test/src/PathTests.scala", 590, "posix", "method", "def", "test.os.PathTests"},
	}
	// These four source-visible declarations were independently verified after
	// the first RED inventory exposed omissions in the human oracle, not the parser.
	oracle = append(oracle,
		decl{"os/src/Path.scala", 15, "segments", "method", "def", "os.PathChunk"},
		decl{"os/src/Path.scala", 16, "ups", "method", "def", "os.PathChunk"},
		decl{"os/src/Path.scala", 20, "stringToPathChunk", "method", "def", "os.StringPathChunkConversion"},
		decl{"os/src/Path.scala", 104, "segments", "field", "val", "os.PathChunk.SeqPathChunk"},
	)
	files := []string{"os/src/Path.scala", "os/src/GlobInterpolator.scala", "os/src/ProcessOps.scala", "os/test/src/PathTests.scala"}
	originals := map[string][]string{}
	paths := make([]string, 0, len(files))
	for _, rel := range files {
		path := filepath.Join(project, filepath.FromSlash(rel))
		requireAdditionalOriginalFile(t, path)
		bytes, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(bytes) > 1<<20 {
			t.Fatalf("oversized os-lib original %s", rel)
		}
		originals[rel] = strings.Split(strings.ReplaceAll(string(bytes), "\r\n", "\n"), "\n")
		paths = append(paths, path)
	}
	expected := map[string]decl{}
	for _, d := range oracle {
		source, ok := originals[d.file]
		if !ok || d.line < 1 || d.line > len(source) || !strings.Contains(source[d.line-1], d.name) {
			t.Fatalf("expected Scala original declaration missing %s:%d %s", d.file, d.line, d.name)
		}
		key := d.file + ":" + strconv.Itoa(d.line)
		if _, ok := expected[key]; ok {
			t.Fatalf("duplicated independent oracle location %s", key)
		}
		expected[key] = d
	}
	h := NewHandler([]string{project})
	failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: paths, Language: "scala", Encoding: "utf-8",
		Presentation: "detailed", MaxFiles: len(files), MaxSymbols: 512,
	})
	if err != nil || failed != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != len(files) {
		t.Fatalf("incomplete os-lib analysis err=%v failed=%+v files=%d count=%d truncated=%v complete=%v", err, failed, out.FilesParsed, len(out.Symbols), out.Truncated, out.CoverageComplete)
	}
	observed := map[string]bool{}
	unexpected := 0
	for _, s := range out.Symbols {
		rel, err := filepath.Rel(project, s.Path)
		if err != nil {
			t.Errorf("symbol path unexpected: %v", err)
			continue
		}
		key := filepath.ToSlash(rel) + ":" + strconv.Itoa(s.NameRange.Start.Line)
		if observed[key] {
			t.Errorf("duplicate symbol at %s", key)
		}
		observed[key] = true
		want, ok := expected[key]
		if !ok {
			unexpected++
			if unexpected <= 25 {
				t.Errorf("false Scala type/member declaration at %s: %s %s owner=%s", key, s.Kind, s.Name, s.ParentQualifiedName)
			}
			continue
		}
		if s.Name != want.name || string(s.Kind) != want.kind || s.NativeKind != want.native || s.ParentQualifiedName != want.parent {
			t.Errorf("incorrect Scala symbol at %s: got %s %s native=%s parent=%s; want %+v", key, s.Kind, s.Name, s.NativeKind, s.ParentQualifiedName, want)
		}
	}
	if unexpected > 25 {
		t.Errorf("and %d further unexpected declaration locations", unexpected-25)
	}
	missing := 0
	for key, d := range expected {
		if !observed[key] {
			missing++
			if missing <= 25 {
				t.Errorf("missing Scala source declaration at %s: %s", key, d.name)
			}
		}
	}
	if missing > 25 {
		t.Errorf("and %d additional missing declaration locations", missing-25)
	}
	if len(out.Symbols) != len(expected) {
		t.Errorf("source-first Scala os-lib count=%d handler=%d", len(expected), len(out.Symbols))
	}
	t.Logf("os-lib source-first selected originals=%d declaration inventory=%d indexed=%d missing=%d unexpected=%d", len(files), len(expected), len(out.Symbols), missing, unexpected)
}
