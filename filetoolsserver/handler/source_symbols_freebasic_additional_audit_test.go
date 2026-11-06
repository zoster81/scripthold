package handler

import (
	"os"
	"testing"
)

// Independent FreeBASIC original-project task oracles. No upstream code runs.
func TestSourceSymbolsOriginalFreeBasicAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original FreeBASIC corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "freebasic-chessbrainfb", language: "freebasic",
			search: "AllocateTime", primary: "time.bi",
			negativeFile: "uci.bi", negativeQuery: "AllocateTime",
			paths: []string{"ChessbrainFB.bas", "time.bi", "search.bi", "uci.bi"},
			oracles: []extraAuditEvidence{
				{"ChessbrainFB.bas", "#include \"time.bi\""},
				{"ChessbrainFB.bas", "#include \"search.bi\""},
				{"ChessbrainFB.bas", "#include \"uci.bi\""},
				{"time.bi", "Sub AllocateTime ()"},
				{"time.bi", "OptimalTime = CalcTime!(MovesToTC, TimeIncrement, TimeLeft, MoveOverhead, -1)"},
				{"time.bi", "MaximumTime = CalcTime!(MovesToTC, TimeIncrement, TimeLeft, MoveOverhead, 0)"},
				{"time.bi", "If OptimalTime > MaximumTime Then OptimalTime = MaximumTime"},
				{"time.bi", "Function CheckTime% ()"},
				{"search.bi", "Sub Think (RetMove As TMOVE)"},
				{"search.bi", "Call AllocateTime()"},
				{"search.bi", "If CheckTime% = 0 Then Exit For"},
				{"uci.bi", "Sub PollUCIInput ()"},
				{"uci.bi", "Sub UCILoop ()"},
			},
			targets: []extraAuditSymbol{
				{"time.bi", "AllocateTime", 25},
				{"time.bi", "CheckTime%", 90},
				{"search.bi", "Think", 107},
				{"uci.bi", "PollUCIInput", 90},
			},
		},
		{
			project: "freebasic-fb-collections", language: "freebasic",
			search: "PriorityOrder", primary: "inc/collections.bi",
			negativeFile: "examples/dictionary-example-1.bas", negativeQuery: "PriorityOrder",
			paths: []string{
				"inc/collections.bi", "inc/collections/TPriorityQueue.bi",
				"examples/priorityqueue-example.bas", "examples/dictionary-example-1.bas",
			},
			oracles: []extraAuditEvidence{
				{"inc/collections.bi", "enum PriorityOrder"},
				{"inc/collections.bi", "Ascending"},
				{"inc/collections.bi", "Descending"},
				{"inc/collections.bi", "__include__( __FBFW_COLLECTIONS_FOLDER__, TPriorityQueue.bi )"},
				{"inc/collections/TPriorityQueue.bi", "type QueueElement( of TType )"},
				{"inc/collections/TPriorityQueue.bi", "destructor QueueElement( of TType )()"},
				{"inc/collections/TPriorityQueue.bi", "if( _needsDisposing andAlso _value <> 0 ) then"},
				{"inc/collections/TPriorityQueue.bi", "function QueueElement( of TType ).detach() as TType ptr"},
				{"inc/collections/TPriorityQueue.bi", "elementValue = _elements( 1 )->detach()"},
				{"inc/collections/TPriorityQueue.bi", "type PriorityQueue( of TType ) extends Collection( of TType )"},
				{"examples/priorityqueue-example.bas", "template( PriorityQueue, of Person )"},
				{"examples/priorityqueue-example.bas", "Collections.PriorityOrder.Ascending"},
				{"examples/priorityqueue-example.bas", "Collections.PriorityOrder.Descending"},
				{"examples/priorityqueue-example.bas", "delete( item )"},
				{"examples/dictionary-example-1.bas", "template( Dictionary, of integer, of Person )"},
				{"examples/dictionary-example-1.bas", "aDictionary.add( items, *aPerson )"},
			},
			targets: []extraAuditSymbol{
				{"inc/collections.bi", "PriorityOrder", 46},
				{"inc/collections/TPriorityQueue.bi", "QueueElement", 21},
				{"inc/collections/TPriorityQueue.bi", "PriorityQueue", 90},
				{"examples/dictionary-example-1.bas", "show", 11},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
