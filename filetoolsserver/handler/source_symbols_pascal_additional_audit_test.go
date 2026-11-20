package handler

import (
	"os"
	"testing"
)

// Independent source-oracle tasks locate project maintenance code and exclude
// plausible but unrelated Pascal components. Original projects are not run.
func TestSourceSymbolsOriginalPascalAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Pascal corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "pascal-bittorrent-tracker-editor", language: "pascal",
			search: "DownloadTracker", primary: "source/code/newtrackon.pas",
			negativeFile:  "source/code/torrent_miscellaneous.pas",
			negativeQuery: "DownloadTracker",
			paths: []string{
				"source/code/newtrackon.pas",
				"source/code/controler_trackerlist_online.pas",
				"source/code/torrent_miscellaneous.pas",
				"source/test/test_newtrackon.pas",
			},
			oracles: []extraAuditEvidence{
				{"README.md", "Download stable trackers from newTrackon or ngosang."},
				{"source/code/newtrackon.pas", "function TNewTrackon.DownloadTracker(NewTrackon_List: TNewTrackon_List): boolean;"},
				{"source/code/newtrackon.pas", "TFPCustomHTTPClient.SimpleGet(URL[NewTrackon_List])"},
				{"source/code/newtrackon.pas", "SanitizeTrackerList(FTRackerList[NewTrackon_List]);"},
				{"source/code/newtrackon.pas", "procedure TNewTrackon.CreateTrackerList_Dead;"},
				{"source/code/newtrackon.pas", "RemoveTrackersFromList(TrackerList_Live, TrackerList_Dead);"},
				{"source/code/newtrackon.pas", "function TNewTrackon.Download_All_Live_Stable: boolean;"},
				{"source/code/controler_trackerlist_online.pas", "function TControlerTrackerListOnline.DownloadTrackers_All_Live_Stable: boolean;"},
				{"source/code/controler_trackerlist_online.pas", "Result := FNewTrackon.Download_All_Live_Stable;"},
				{"source/code/controler_trackerlist_online.pas", "UpdateView;"},
				{"source/test/test_newtrackon.pas", "procedure TTestNewTrackon.Test_API_Download;"},
				{"source/test/test_newtrackon.pas", "Check(FNewTrackon.DownloadEverything, 'Download the newtrackon API');"},
				{"source/code/torrent_miscellaneous.pas", "procedure RandomizeTrackerList(StringList: TStringList);"},
			},
			targets: []extraAuditSymbol{
				{"source/code/newtrackon.pas", "DownloadTracker", 97},
				{"source/code/newtrackon.pas", "CreateTrackerList_Dead", 90},
				{"source/code/newtrackon.pas", "Download_All_Live_Stable", 135},
				{"source/code/controler_trackerlist_online.pas", "DownloadTrackers_All_Live_Stable", 79},
				{"source/test/test_newtrackon.pas", "Test_API_Download", 28},
			},
		},
		{
			project: "pascal-darkcodersc-win-brute-logon", language: "pascal",
			search: "DisplayHelpBanner", primary: "WinBruteLogon.dpr",
			negativeFile: "Units/UntTypeDefs.pas", negativeQuery: "DisplayHelpBanner",
			paths: []string{
				"WinBruteLogon.dpr", "Units/UntFunctions.pas",
				"Units/UntStringDefs.pas", "Units/UntTypeDefs.pas",
				"Units/UntGlobalDefs.pas",
			},
			oracles: []extraAuditEvidence{
				{"WinBruteLogon.dpr", "procedure DisplayHelpBanner(AErrorMsg : String = ''; AFull : Boolean = False);"},
				{"WinBruteLogon.dpr", "WriteColoredWord('WinBruteLogon', FOREGROUND_GREEN);"},
				{"WinBruteLogon.dpr", "Debug(AErrorMsg, dlError);"},
				{"WinBruteLogon.dpr", "raise Exception.Create(Format(SD_FILE_NOT_FOUND, [AWordlist]));"},
				{"WinBruteLogon.dpr", "on E : EOptionException do"},
				{"WinBruteLogon.dpr", "UntStringDefs in 'Units\\UntStringDefs.pas'"},
				{"Units/UntFunctions.pas", "procedure WriteColoredWord(AString : String; AColor : Word = FOREGROUND_GREEN);"},
				{"Units/UntFunctions.pas", "procedure Debug(AMessage : String; ADebugLevel : TDebugLevel = dlInfo; AForce : Boolean = False);"},
				{"Units/UntStringDefs.pas", "const SD_FILE_NOT_FOUND = 'File \"%s\" not found.';"},
				{"Units/UntTypeDefs.pas", "EOptionException = class(Exception);"},
				{"Units/UntGlobalDefs.pas", "var G_DEBUG : Boolean = False;"},
			},
			targets: []extraAuditSymbol{
				{"WinBruteLogon.dpr", "DisplayHelpBanner", 29},
				{"Units/UntFunctions.pas", "WriteColoredWord", 46},
				{"Units/UntFunctions.pas", "Debug", 208},
				{"Units/UntStringDefs.pas", "SD_FILE_NOT_FOUND", 18},
				{"Units/UntTypeDefs.pas", "EOptionException", 24},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
