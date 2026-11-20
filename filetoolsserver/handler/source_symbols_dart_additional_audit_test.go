package handler

import (
	"os"
	"testing"
)

// Independent Dart maintenance oracles are read from original source and tests;
// neither Flutter app nor external test suite is executed.
func TestSourceSymbolsOriginalDartAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Dart corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "dart-chen08209-flclash", language: "dart",
			// Trace SHA-256 manifest validation through the desktop helper's
			// readiness request and the two independent test locations.
			search:        "readCoreSha256",
			primary:       "lib/core/desktop/core_manifest.dart",
			negativeFile:  "lib/core/desktop/launcher.dart",
			negativeQuery: "readCoreSha256",
			paths: []string{
				"lib/core/desktop/core_manifest.dart",
				"lib/core/desktop/helper_client.dart",
				"test/core/desktop/core_manifest_test.dart",
				"test/core/desktop/helper_client_test.dart",
				"lib/core/desktop/launcher.dart",
			},
			oracles: []extraAuditEvidence{
				{"lib/core/desktop/core_manifest.dart", "final class CoreManifest {"},
				{"lib/core/desktop/core_manifest.dart", "static Future<String?> readCoreSha256({String? path}) async {"},
				{"lib/core/desktop/core_manifest.dart", "final coreSha256 = value['coreSha256'];"},
				{"lib/core/desktop/core_manifest.dart", "RegExp(r'^[0-9a-f]{64}$').hasMatch(coreSha256)"},
				{"lib/core/desktop/core_manifest.dart", "p.dirname(Platform.resolvedExecutable), coreManifestName"},
				{"lib/core/desktop/helper_client.dart", "Future<String> _readCoreSha256Once() async {"},
				{"lib/core/desktop/helper_client.dart", "return (await CoreManifest.readCoreSha256()) ?? '';"},
				{"lib/core/desktop/helper_client.dart", "return WindowsHelperReadiness.manifestMissing;"},
				{"test/core/desktop/core_manifest_test.dart", "test('rejects malformed or non-SHA256 manifest values', () async {"},
				{"test/core/desktop/core_manifest_test.dart", "test('returns null when the manifest is missing', () async {"},
				{"test/core/desktop/helper_client_test.dart", "test('missing manifest never probes the Helper', () async {"},
				{"test/core/desktop/helper_client_test.dart", "test('an unreadable manifest is retried on the next probe', () async {"},
				{"lib/core/desktop/launcher.dart", "final class DirectCoreLauncher implements CoreProcessLauncher {"},
			},
			targets: []extraAuditSymbol{
				{"lib/core/desktop/core_manifest.dart", "CoreManifest", 7},
				{"lib/core/desktop/core_manifest.dart", "readCoreSha256", 10},
				{"lib/core/desktop/core_manifest.dart", "_defaultPath", 26},
				{"lib/core/desktop/helper_client.dart", "_readCoreSha256Once", 70},
				{"lib/core/desktop/helper_client.dart", "_readBundledCoreSha256", 99},
			},
		},
		{
			project: "dart-ente-ente", language: "dart",
			// Locate safe cleanup of an app directory without deleting its
			// root; ensure missing paths and recursive subdirs have tests.
			search:        "deleteDirectoryContents",
			primary:       "mobile/packages/ente_pure_utils/lib/src/directory_content_util.dart",
			negativeFile:  "mobile/packages/ente_pure_utils/lib/src/date_time_util.dart",
			negativeQuery: "deleteDirectoryContents",
			paths: []string{
				"mobile/packages/ente_pure_utils/lib/src/directory_content_util.dart",
				"mobile/packages/ente_pure_utils/test/directory_content_util_test.dart",
				"mobile/packages/ente_pure_utils/lib/ente_pure_utils.dart",
				"mobile/packages/ente_pure_utils/lib/src/directory_utils.dart",
				"mobile/packages/ente_pure_utils/lib/src/date_time_util.dart",
			},
			oracles: []extraAuditEvidence{
				{"mobile/packages/ente_pure_utils/lib/src/directory_content_util.dart", "Future<void> deleteDirectoryContents(String directoryPath) async {"},
				{"mobile/packages/ente_pure_utils/lib/src/directory_content_util.dart", "final contents = await directory.list().toList();"},
				{"mobile/packages/ente_pure_utils/lib/src/directory_content_util.dart", "recursive: fileOrDirectory is Directory,"},
				{"mobile/packages/ente_pure_utils/lib/src/directory_content_util.dart", "Future<bool> deleteFileSystemEntityIfPresent("},
				{"mobile/packages/ente_pure_utils/lib/src/directory_content_util.dart", "if (!isFileSystemPathMissing(e)) {"},
				{"mobile/packages/ente_pure_utils/lib/src/directory_content_util.dart", "e.osError?.errorCode == _noSuchFileOrDirectoryErrorCode;"},
				{"mobile/packages/ente_pure_utils/test/directory_content_util_test.dart", "test('deletes files and directories while preserving the root', () async {"},
				{"mobile/packages/ente_pure_utils/test/directory_content_util_test.dart", "test('ignores a missing root directory', () async {"},
				{"mobile/packages/ente_pure_utils/test/directory_content_util_test.dart", "test('deletes a directory recursively', () async {"},
				{"mobile/packages/ente_pure_utils/lib/ente_pure_utils.dart", "export 'src/directory_content_util.dart';"},
				{"mobile/packages/ente_pure_utils/lib/src/directory_utils.dart", "static Future<Directory> getTempsDir() async {"},
				{"mobile/packages/ente_pure_utils/lib/src/date_time_util.dart", "String getMonthAndYear(DateTime dateTime) {"},
			},
			targets: []extraAuditSymbol{
				{"mobile/packages/ente_pure_utils/lib/src/directory_content_util.dart", "deleteDirectoryContents", 116},
				{"mobile/packages/ente_pure_utils/lib/src/directory_content_util.dart", "deleteFileSystemEntityIfPresent", 131},
				{"mobile/packages/ente_pure_utils/lib/src/directory_content_util.dart", "isFileSystemPathMissing", 146},
				{"mobile/packages/ente_pure_utils/lib/src/directory_content_util.dart", "getFileSize", 150},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
