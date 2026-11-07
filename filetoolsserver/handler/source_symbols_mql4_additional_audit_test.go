package handler

import (
	"os"
	"testing"
)

// Original MQL4 source is read as evidence, never compiled or executed.
func TestSourceSymbolsOriginalMQL4AdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original MQL4 corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "mql4-dingmaotu", language: "mql4",
			search: "HashMap", primary: "Collection/HashMap.mqh",
			negativeFile: "Collection/Map.mqh", negativeQuery: "HashMap",
			paths: []string{"Collection/HashMap.mqh", "Collection/HashEntries.mqh", "Collection/HashSlots.mqh", "Collection/Map.mqh"},
			oracles: []extraAuditEvidence{
				{"Collection/HashMap.mqh", "class HashMap: public Map<Key,Value>"},
				{"Collection/HashMap.mqh", "class HashMapEntries: public HashEntriesBase<Key>"},
				{"Collection/HashMap.mqh", "if(m_entries.shouldCompact())"},
				{"Collection/HashMap.mqh", "m_slots.lookupIndex(key)"},
				{"Collection/HashEntries.mqh", "bool              shouldCompact() const"},
				{"Collection/HashEntries.mqh", "void              compact()"},
				{"Collection/HashSlots.mqh", "class HashSlots"},
				{"Collection/Map.mqh", "interface Map"},
			},
			targets: []extraAuditSymbol{
				{"Collection/HashMap.mqh", "HashMapEntries", 30},
				{"Collection/HashMap.mqh", "HashMap", 85},
				{"Collection/HashEntries.mqh", "HashEntries", 28},
				{"Collection/HashEntries.mqh", "HashEntriesBase", 42},
				{"Collection/HashSlots.mqh", "HashSlots", 31},
				{"Collection/Map.mqh", "Map", 40},
			},
		},
		{
			project: "mql4-dingmaotu-mt4-server", language: "mql4",
			search: "TradeCommandProcessor", primary: "Scripts/CommandProcessor.mqh",
			negativeFile: "Scripts/MqlCommand.mqh", negativeQuery: "TradeCommandProcessor",
			paths: []string{"Scripts/CommandProcessor.mqh", "Scripts/MqlCommand.mqh"},
			oracles: []extraAuditEvidence{
				{"Scripts/CommandProcessor.mqh", "interface CommandProcessor"},
				{"Scripts/CommandProcessor.mqh", "class TradeCommandProcessor: public CommandProcessor"},
				{"Scripts/CommandProcessor.mqh", "m_commands.set(\"BUY\",new BuyCommand);"},
				{"Scripts/CommandProcessor.mqh", "m_commands.set(\"SELL\",new SellCommand);"},
				{"Scripts/MqlCommand.mqh", "class BuyCommand: public MqlCommand"},
				{"Scripts/MqlCommand.mqh", "class SellCommand: public MqlCommand"},
				{"Scripts/MqlCommand.mqh", "command.size()!=3"},
				{"Scripts/Mt4ServerRaw.mq4", "m_processor=new TradeCommandProcessor;"},
				{"Scripts/Mt4ServerZMQ.mq4", "m_processor=new TradeCommandProcessor;"},
			},
			targets: []extraAuditSymbol{
				{"Scripts/CommandProcessor.mqh", "CommandProcessor", 29},
				{"Scripts/CommandProcessor.mqh", "TradeCommandProcessor", 36},
				{"Scripts/MqlCommand.mqh", "MqlCommand", 30},
				{"Scripts/MqlCommand.mqh", "BuyCommand", 74},
				{"Scripts/MqlCommand.mqh", "SellCommand", 102},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
