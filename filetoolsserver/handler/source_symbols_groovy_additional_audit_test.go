package handler

import (
	"os"
	"testing"
)

// Two further original Groovy projects, with independent source-first
// maintenance oracles, relevant neighbors/tests and an unrelated alternative.
func TestSourceSymbolsOriginalGroovyAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Groovy corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "groovy-google-protobuf-gradle-plugin", language: "groovy",
			// Where are protoc/plugin coordinates split and resolved, and
			// what tests cover classifiers and extension defaults?
			search:        "artifactParts",
			primary:       "src/main/groovy/com/google/protobuf/gradle/ToolsLocator.groovy",
			negativeFile:  "src/main/groovy/com/google/protobuf/gradle/ProtobufExtract.groovy",
			negativeQuery: "artifactParts",
			paths: []string{
				"src/main/groovy/com/google/protobuf/gradle/ToolsLocator.groovy",
				"src/main/groovy/com/google/protobuf/gradle/ExecutableLocator.groovy",
				"src/main/groovy/com/google/protobuf/gradle/ProtobufExtension.groovy",
				"src/test/groovy/com/google/protobuf/gradle/ToolsLocatorSpec.groovy",
				"src/main/groovy/com/google/protobuf/gradle/ProtobufExtract.groovy",
			},
			oracles: []extraAuditEvidence{
				{"src/main/groovy/com/google/protobuf/gradle/ToolsLocator.groovy", "static List<String> artifactParts(String artifactCoordinate) {"},
				{"src/main/groovy/com/google/protobuf/gradle/ToolsLocator.groovy", "extension = extension ?: \"exe\""},
				{"src/main/groovy/com/google/protobuf/gradle/ToolsLocator.groovy", "protoc.path = 'protoc'"},
				{"src/main/groovy/com/google/protobuf/gradle/ExecutableLocator.groovy", "void setArtifact(String spec) {"},
				{"src/main/groovy/com/google/protobuf/gradle/ExecutableLocator.groovy", "this.artifact = null"},
				{"src/main/groovy/com/google/protobuf/gradle/ProtobufExtension.groovy", "@SuppressWarnings(["},
				{"src/main/groovy/com/google/protobuf/gradle/ProtobufExtension.groovy", "abstract class ProtobufExtension {"},
				{"src/main/groovy/com/google/protobuf/gradle/ProtobufExtension.groovy", "public void protoc(Action<ExecutableLocator> configureAction) {"},
				{"src/test/groovy/com/google/protobuf/gradle/ToolsLocatorSpec.groovy", "class ToolsLocatorSpec extends Specification {"},
				{"src/test/groovy/com/google/protobuf/gradle/ToolsLocatorSpec.groovy", "ToolsLocator.artifactParts(input)"},
				{"src/main/groovy/com/google/protobuf/gradle/ProtobufExtract.groovy", "class ProtobufExtract"},
			},
			targets: []extraAuditSymbol{
				{"src/main/groovy/com/google/protobuf/gradle/ToolsLocator.groovy", "artifactParts", 46},
				{"src/main/groovy/com/google/protobuf/gradle/ToolsLocator.groovy", "resolve", 78},
				{"src/main/groovy/com/google/protobuf/gradle/ToolsLocator.groovy", "resolveLocator", 93},
				{"src/main/groovy/com/google/protobuf/gradle/ExecutableLocator.groovy", "setArtifact", 66},
				{"src/main/groovy/com/google/protobuf/gradle/ExecutableLocator.groovy", "setPath", 74},
				{"src/main/groovy/com/google/protobuf/gradle/ProtobufExtension.groovy", "ProtobufExtension", 51},
				{"src/main/groovy/com/google/protobuf/gradle/ProtobufExtension.groovy", "protoc", 128},
				{"src/test/groovy/com/google/protobuf/gradle/ToolsLocatorSpec.groovy", "ToolsLocatorSpec", 38},
			},
		},
		{
			project: "groovy-hujiangtechnology-gradle_plugin_android_aspectjx", language: "groovy",
			// Find who owns the include/exclude plugin configuration,
			// matches names, and bypasses weaving when disabled.
			search:        "isFilterMatched",
			primary:       "aspectjx/src/main/groovy/com/hujiang/gradle/plugin/android/aspectjx/internal/AJXUtils.groovy",
			negativeFile:  "aspectjx/src/main/groovy/com/hujiang/gradle/plugin/android/aspectjx/AJXPlugin.groovy",
			negativeQuery: "isFilterMatched",
			paths: []string{
				"aspectjx/src/main/groovy/com/hujiang/gradle/plugin/android/aspectjx/AJXExtension.groovy",
				"aspectjx/src/main/groovy/com/hujiang/gradle/plugin/android/aspectjx/AJXTransform.groovy",
				"aspectjx/src/main/groovy/com/hujiang/gradle/plugin/android/aspectjx/internal/AJXUtils.groovy",
				"aspectjx/src/main/groovy/com/hujiang/gradle/plugin/android/aspectjx/internal/procedure/CheckAspectJXEnableProcedure.groovy",
				"aspectjx/src/main/groovy/com/hujiang/gradle/plugin/android/aspectjx/AJXPlugin.groovy",
			},
			oracles: []extraAuditEvidence{
				{"aspectjx/src/main/groovy/com/hujiang/gradle/plugin/android/aspectjx/AJXExtension.groovy", "boolean enabled = true"},
				{"aspectjx/src/main/groovy/com/hujiang/gradle/plugin/android/aspectjx/AJXExtension.groovy", "AJXExtension include(String...filters) {"},
				{"aspectjx/src/main/groovy/com/hujiang/gradle/plugin/android/aspectjx/AJXExtension.groovy", "AJXExtension exclude(String...filters) {"},
				{"aspectjx/src/main/groovy/com/hujiang/gradle/plugin/android/aspectjx/internal/AJXUtils.groovy", "static boolean isFilterMatched(String str, List<String> filters, FilterPolicy filterPolicy) {"},
				{"aspectjx/src/main/groovy/com/hujiang/gradle/plugin/android/aspectjx/internal/AJXUtils.groovy", "return filterPolicy == FilterPolicy.INCLUDE"},
				{"aspectjx/src/main/groovy/com/hujiang/gradle/plugin/android/aspectjx/internal/AJXUtils.groovy", "static boolean isContained(String str, String filter) {"},
				{"aspectjx/src/main/groovy/com/hujiang/gradle/plugin/android/aspectjx/internal/procedure/CheckAspectJXEnableProcedure.groovy", "if (!ajxExtensionConfig.enabled || isExcludeAll) {"},
				{"aspectjx/src/main/groovy/com/hujiang/gradle/plugin/android/aspectjx/AJXTransform.groovy", "new CheckAspectJXEnableProcedure(project, variantCache, transformInvocation)"},
				{"aspectjx/src/main/groovy/com/hujiang/gradle/plugin/android/aspectjx/AJXPlugin.groovy", "project.extensions.create(\"aspectjx\", AJXExtension)"},
			},
			targets: []extraAuditSymbol{
				{"aspectjx/src/main/groovy/com/hujiang/gradle/plugin/android/aspectjx/AJXExtension.groovy", "include", 35},
				{"aspectjx/src/main/groovy/com/hujiang/gradle/plugin/android/aspectjx/AJXExtension.groovy", "exclude", 43},
				{"aspectjx/src/main/groovy/com/hujiang/gradle/plugin/android/aspectjx/internal/AJXUtils.groovy", "isFilterMatched", 204},
				{"aspectjx/src/main/groovy/com/hujiang/gradle/plugin/android/aspectjx/internal/AJXUtils.groovy", "isContained", 222},
				{"aspectjx/src/main/groovy/com/hujiang/gradle/plugin/android/aspectjx/internal/procedure/CheckAspectJXEnableProcedure.groovy", "doWorkContinuously", 35},
				{"aspectjx/src/main/groovy/com/hujiang/gradle/plugin/android/aspectjx/AJXTransform.groovy", "transform", 64},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
