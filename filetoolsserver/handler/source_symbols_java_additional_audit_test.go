package handler

import (
	"os"
	"testing"
)

// Independent source-first tasks inspect pool timeout configuration and two
// genuinely distinct Java cache implementations without executing them.
func TestSourceSymbolsOriginalJavaAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Java corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "qualification/final-candidate-corpus/project-java-brettwooldridge-hikaricp", language: "java",
			search: "setConnectionTimeout", primary: "src/main/java/com/zaxxer/hikari/HikariConfig.java",
			negativeFile: "src/main/java/com/zaxxer/hikari/pool/HikariPool.java", negativeQuery: "setConnectionTimeout",
			paths: []string{
				"src/main/java/com/zaxxer/hikari/HikariConfig.java",
				"src/main/java/com/zaxxer/hikari/HikariConfigMXBean.java",
				"src/main/java/com/zaxxer/hikari/pool/HikariPool.java",
				"src/test/java/com/zaxxer/hikari/pool/TestConnectionTimeoutRetry.java",
			},
			oracles: []extraAuditEvidence{
				{"src/main/java/com/zaxxer/hikari/HikariConfig.java", "private static final long CONNECTION_TIMEOUT = SECONDS.toMillis(30);"},
				{"src/main/java/com/zaxxer/hikari/HikariConfig.java", "private static final long SOFT_TIMEOUT_FLOOR = Long.getLong("},
				{"src/main/java/com/zaxxer/hikari/HikariConfig.java", "connectionTimeout = CONNECTION_TIMEOUT;"},
				{"src/main/java/com/zaxxer/hikari/HikariConfig.java", "public long getConnectionTimeout()"},
				{"src/main/java/com/zaxxer/hikari/HikariConfig.java", "public void setConnectionTimeout(long connectionTimeoutMs)"},
				{"src/main/java/com/zaxxer/hikari/HikariConfig.java", "this.connectionTimeout = Integer.MAX_VALUE;"},
				{"src/main/java/com/zaxxer/hikari/HikariConfig.java", "throw new IllegalArgumentException("},
				{"src/main/java/com/zaxxer/hikari/HikariConfigMXBean.java", "void setConnectionTimeout(long connectionTimeoutMs);"},
				{"src/main/java/com/zaxxer/hikari/pool/HikariPool.java", "return getConnection(connectionTimeout);"},
				{"src/main/java/com/zaxxer/hikari/pool/HikariPool.java", "var poolEntry = connectionBag.borrow(timeout, MILLISECONDS);"},
				{"src/test/java/com/zaxxer/hikari/pool/TestConnectionTimeoutRetry.java", "public void testConnectionRetries() throws SQLException"},
				{"src/test/java/com/zaxxer/hikari/pool/TestConnectionTimeoutRetry.java", "config.setConnectionTimeout(2800);"},
				{"src/test/java/com/zaxxer/hikari/pool/TestConnectionTimeoutRetry.java", "assertTrue(\"Didn't wait long enough for timeout\", (elapsed >= timeout));"},
			},
			targets: []extraAuditSymbol{
				{"src/main/java/com/zaxxer/hikari/HikariConfig.java", "setConnectionTimeout", 189},
				{"src/main/java/com/zaxxer/hikari/HikariConfig.java", "getConnectionTimeout", 182},
				{"src/main/java/com/zaxxer/hikari/HikariConfigMXBean.java", "setConnectionTimeout", 44},
				{"src/main/java/com/zaxxer/hikari/pool/HikariPool.java", "getConnection", 140},
				{"src/test/java/com/zaxxer/hikari/pool/TestConnectionTimeoutRetry.java", "testConnectionRetries", 53},
			},
		},
		{
			project: "project-java-crossoverjie-jcsprout", language: "java",
			search: "LRUMap", primary: "src/main/java/com/crossoverjie/actual/LRUMap.java",
			negativeFile: "src/main/java/com/crossoverjie/actual/LRULinkedMap.java", negativeQuery: "LRUMap",
			paths: []string{
				"src/main/java/com/crossoverjie/actual/LRUMap.java",
				"src/main/java/com/crossoverjie/actual/LRULinkedMap.java",
				"src/main/java/com/crossoverjie/actual/LRUAbstractMap.java",
				"src/test/java/com/crossoverjie/actual/LRUMapTest.java",
				"src/test/java/com/crossoverjie/actual/LRULinkedMapTest.java",
			},
			oracles: []extraAuditEvidence{
				{"src/main/java/com/crossoverjie/actual/LRUMap.java", "public class LRUMap<K, V> {"},
				{"src/main/java/com/crossoverjie/actual/LRUMap.java", "private final Map<K, V> cacheMap = new HashMap<>();"},
				{"src/main/java/com/crossoverjie/actual/LRUMap.java", "public void put(K key, V value) {"},
				{"src/main/java/com/crossoverjie/actual/LRUMap.java", "addNode(key, value);"},
				{"src/main/java/com/crossoverjie/actual/LRUMap.java", "moveToHead(node) ;"},
				{"src/main/java/com/crossoverjie/actual/LRUMap.java", "if (cacheSize == nodeCount) {"},
				{"src/main/java/com/crossoverjie/actual/LRUMap.java", "delTail();"},
				{"src/main/java/com/crossoverjie/actual/LRULinkedMap.java", "public class LRULinkedMap<K,V> {"},
				{"src/main/java/com/crossoverjie/actual/LRULinkedMap.java", "cacheMap = new LinkedHashMap(16,0.75F,true){"},
				{"src/main/java/com/crossoverjie/actual/LRULinkedMap.java", "protected boolean removeEldestEntry(Map.Entry eldest) {"},
				{"src/main/java/com/crossoverjie/actual/LRUAbstractMap.java", "public class LRUAbstractMap extends java.util.AbstractMap {"},
				{"src/main/java/com/crossoverjie/actual/LRUAbstractMap.java", "private final static Long EXPIRE_TIME = 60 * 60 * 1000L ;"},
				{"src/test/java/com/crossoverjie/actual/LRUMapTest.java", "LRUMap<String,Integer> lruMap = new LRUMap(3) ;"},
				{"src/test/java/com/crossoverjie/actual/LRUMapTest.java", "lru.get(\"2\");"},
				{"src/test/java/com/crossoverjie/actual/LRULinkedMapTest.java", "LRULinkedMap<String,Integer> map = new LRULinkedMap(3) ;"},
			},
			targets: []extraAuditSymbol{
				{"src/main/java/com/crossoverjie/actual/LRUMap.java", "LRUMap", 13},
				{"src/main/java/com/crossoverjie/actual/LRUMap.java", "addNode", 127},
				{"src/main/java/com/crossoverjie/actual/LRUMap.java", "delTail", 164},
				{"src/main/java/com/crossoverjie/actual/LRULinkedMap.java", "LRULinkedMap", 15},
				{"src/main/java/com/crossoverjie/actual/LRUAbstractMap.java", "LRUAbstractMap", 27},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
