package handler

import (
	"os"
	"testing"
)

// Original project evidence is independently read before source_* queries.
func TestSourceSymbolsOriginalKotlinAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Kotlin corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "kotlin-wave10b-adrielcafe-broker", language: "kotlin",
			search: "emitRetainedEvents", primary: "broker-core/src/main/java/cafe/adriel/broker/Broker.kt",
			negativeFile:  "broker-lifecycle/src/main/java/cafe/adriel/broker/lifecycle/BrokerKtx.kt",
			negativeQuery: "emitRetainedEvents",
			paths: []string{
				"broker-core/src/main/java/cafe/adriel/broker/Broker.kt",
				"broker-core/src/main/java/cafe/adriel/broker/BrokerSubscriber.kt",
				"broker-core/src/main/java/cafe/adriel/broker/BrokerKtx.kt",
				"broker-core/src/test/java/cafe/adriel/broker/BrokerTest.kt",
				"broker-lifecycle/src/main/java/cafe/adriel/broker/lifecycle/BrokerKtx.kt",
			},
			oracles: []extraAuditEvidence{
				{"broker-core/src/main/java/cafe/adriel/broker/Broker.kt", "open class Broker("},
				{"broker-core/src/main/java/cafe/adriel/broker/Broker.kt", "private val retainedEvents by lazy { ConcurrentHashMap<KClass<out Any>, Any>() }"},
				{"broker-core/src/main/java/cafe/adriel/broker/Broker.kt", "if (retain) retainedEvents[event::class] = event"},
				{"broker-core/src/main/java/cafe/adriel/broker/Broker.kt", ".onStart { if (emitRetained) emitRetainedEvents() }"},
				{"broker-core/src/main/java/cafe/adriel/broker/Broker.kt", ".filter { event -> event::class == eventClass }"},
				{"broker-core/src/main/java/cafe/adriel/broker/Broker.kt", "override fun <T : Any> removeRetained(eventClass: KClass<T>): T? ="},
				{"broker-core/src/main/java/cafe/adriel/broker/Broker.kt", "private suspend fun FlowCollector<Any>.emitRetainedEvents() ="},
				{"broker-core/src/main/java/cafe/adriel/broker/Broker.kt", "emitAll(retainedEvents.values.asFlow())"},
				{"broker-core/src/main/java/cafe/adriel/broker/BrokerSubscriber.kt", "interface BrokerSubscriber {"},
				{"broker-core/src/main/java/cafe/adriel/broker/BrokerSubscriber.kt", "emitRetained: Boolean = false,"},
				{"broker-core/src/main/java/cafe/adriel/broker/BrokerKtx.kt", "inline fun <reified T : Any> BrokerSubscriber.subscribe("},
				{"broker-core/src/main/java/cafe/adriel/broker/BrokerKtx.kt", "subscribe(subscriber, T::class, scope, dispatcher, emitRetained, onEvent)"},
				{"broker-core/src/test/java/cafe/adriel/broker/BrokerTest.kt", "coVerify(exactly = 2) { listenerSuccess(event) }"},
				{"broker-core/src/test/java/cafe/adriel/broker/BrokerTest.kt", "expectThat(broker.getRetained<TestEvent>()).isNull()"},
				{"broker-lifecycle/src/main/java/cafe/adriel/broker/lifecycle/BrokerKtx.kt", "ON_START -> subscribe(subscriber, owner.lifecycleScope, dispatcher, emitRetained, onEvent)"},
				{"broker-lifecycle/src/main/java/cafe/adriel/broker/lifecycle/BrokerKtx.kt", "ON_STOP -> unsubscribe(subscriber)"},
			},
			targets: []extraAuditSymbol{
				{"broker-core/src/main/java/cafe/adriel/broker/Broker.kt", "Broker", 23},
				{"broker-core/src/main/java/cafe/adriel/broker/Broker.kt", "publish", 31},
				{"broker-core/src/main/java/cafe/adriel/broker/Broker.kt", "subscribe", 37},
				{"broker-core/src/main/java/cafe/adriel/broker/Broker.kt", "emitRetainedEvents", 74},
				{"broker-core/src/main/java/cafe/adriel/broker/BrokerSubscriber.kt", "BrokerSubscriber", 8},
				{"broker-core/src/test/java/cafe/adriel/broker/BrokerTest.kt", "BrokerTest", 16},
			},
		},
		{
			project: "kotlin-wave10b-petertrr-kotlin-multiplatform-diff", language: "kotlin",
			search:        "IgnoreWsStringDiffEqualizer",
			primary:       "src/commonMain/kotlin/io/github/petertrr/diffutils/algorithm/IgnoreWsStringDiffEqualizer.kt",
			negativeFile:  "src/commonMain/kotlin/io/github/petertrr/diffutils/algorithm/EqualsDiffEqualizer.kt",
			negativeQuery: "IgnoreWsStringDiffEqualizer",
			paths: []string{
				"src/commonMain/kotlin/io/github/petertrr/diffutils/text/DiffRowGenerator.kt",
				"src/commonMain/kotlin/io/github/petertrr/diffutils/algorithm/IgnoreWsStringDiffEqualizer.kt",
				"src/commonMain/kotlin/io/github/petertrr/diffutils/algorithm/EqualsDiffEqualizer.kt",
				"src/commonMain/kotlin/io/github/petertrr/diffutils/algorithm/myers/MyersDiff.kt",
				"src/commonTest/kotlin/io/github/petertrr/diffutils/text/DiffRowGeneratorTest.kt",
			},
			oracles: []extraAuditEvidence{
				{"src/commonMain/kotlin/io/github/petertrr/diffutils/text/DiffRowGenerator.kt", "private val ignoreWhiteSpaces: Boolean = false,"},
				{"src/commonMain/kotlin/io/github/petertrr/diffutils/text/DiffRowGenerator.kt", "private val algorithm: DiffAlgorithm<String> = defaultAlgorithm(ignoreWhiteSpaces),"},
				{"src/commonMain/kotlin/io/github/petertrr/diffutils/text/DiffRowGenerator.kt", "private fun defaultAlgorithm(ignoreWhiteSpaces: Boolean): DiffAlgorithm<String> {"},
				{"src/commonMain/kotlin/io/github/petertrr/diffutils/text/DiffRowGenerator.kt", "if (ignoreWhiteSpaces) IgnoreWsStringDiffEqualizer() else EqualsDiffEqualizer()"},
				{"src/commonMain/kotlin/io/github/petertrr/diffutils/text/DiffRowGenerator.kt", "return MyersDiff(equalizer)"},
				{"src/commonMain/kotlin/io/github/petertrr/diffutils/algorithm/IgnoreWsStringDiffEqualizer.kt", "public class IgnoreWsStringDiffEqualizer : DiffEqualizer<String> {"},
				{"src/commonMain/kotlin/io/github/petertrr/diffutils/algorithm/IgnoreWsStringDiffEqualizer.kt", "private val ws = Regex("},
				{"src/commonMain/kotlin/io/github/petertrr/diffutils/algorithm/IgnoreWsStringDiffEqualizer.kt", "ws.replace(one.trim(), \" \") == ws.replace(two.trim(), \" \")"},
				{"src/commonMain/kotlin/io/github/petertrr/diffutils/algorithm/EqualsDiffEqualizer.kt", "public class EqualsDiffEqualizer<T> : DiffEqualizer<T> {"},
				{"src/commonMain/kotlin/io/github/petertrr/diffutils/algorithm/EqualsDiffEqualizer.kt", "one == two"},
				{"src/commonMain/kotlin/io/github/petertrr/diffutils/algorithm/myers/MyersDiff.kt", "public class MyersDiff<T>(private val equalizer: DiffEqualizer<T> = EqualsDiffEqualizer())"},
				{"src/commonMain/kotlin/io/github/petertrr/diffutils/algorithm/myers/MyersDiff.kt", "equalizer.test(orig[i], rev[j])"},
				{"src/commonTest/kotlin/io/github/petertrr/diffutils/text/DiffRowGeneratorTest.kt", "fun testGenerator_IgnoreWhitespaces() {"},
				{"src/commonTest/kotlin/io/github/petertrr/diffutils/text/DiffRowGeneratorTest.kt", "ignoreWhiteSpaces = true,"},
				{"src/commonTest/kotlin/io/github/petertrr/diffutils/text/DiffRowGeneratorTest.kt", "assertEquals(rows[3].tag, DiffRow.Tag.CHANGE)"},
			},
			targets: []extraAuditSymbol{
				{"src/commonMain/kotlin/io/github/petertrr/diffutils/text/DiffRowGenerator.kt", "DiffRowGenerator", 81},
				{"src/commonMain/kotlin/io/github/petertrr/diffutils/text/DiffRowGenerator.kt", "defaultAlgorithm", 475},
				{"src/commonMain/kotlin/io/github/petertrr/diffutils/algorithm/IgnoreWsStringDiffEqualizer.kt", "IgnoreWsStringDiffEqualizer", 18},
				{"src/commonMain/kotlin/io/github/petertrr/diffutils/algorithm/IgnoreWsStringDiffEqualizer.kt", "test", 23},
				{"src/commonMain/kotlin/io/github/petertrr/diffutils/algorithm/EqualsDiffEqualizer.kt", "EqualsDiffEqualizer", 18},
				{"src/commonMain/kotlin/io/github/petertrr/diffutils/algorithm/myers/MyersDiff.kt", "MyersDiff", 31},
				{"src/commonTest/kotlin/io/github/petertrr/diffutils/text/DiffRowGeneratorTest.kt", "testGenerator_IgnoreWhitespaces", 74},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
