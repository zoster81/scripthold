package handler

import (
	"os"
	"testing"
)

// Source-first navigation on further independent original Apex projects.
func TestSourceSymbolsOriginalApexAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Apex corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "apex-certinia-apex-mdapi", language: "apex",
			// Diagnose remote site/API connection warnings and failure
			// display, separating that flow from metadata deployment.
			search:        "checkMetadataAPIConnection",
			primary:       "apex-mdapi/src/classes/RemoteSiteHelperController.cls",
			negativeFile:  "apex-mdapi/src/classes/MetadataDeployController.cls",
			negativeQuery: "checkMetadataAPIConnection",
			paths: []string{
				"apex-mdapi/src/classes/RemoteSiteHelperController.cls",
				"apex-mdapi/src/classes/RemoteSiteHelperTest.cls",
				"apex-mdapi/src/classes/MetadataDeployController.cls",
			},
			oracles: []extraAuditEvidence{
				{"apex-mdapi/src/classes/RemoteSiteHelperController.cls", "global class RemoteSiteHelperController"},
				{"apex-mdapi/src/classes/RemoteSiteHelperController.cls", "if(!RemoteSiteHelperController.checkMetadataAPIConnection())"},
				{"apex-mdapi/src/classes/RemoteSiteHelperController.cls", "MetadataConnectionWarning = true;"},
				{"apex-mdapi/src/classes/RemoteSiteHelperController.cls", "service.listMetadata(queries, 28);"},
				{"apex-mdapi/src/classes/RemoteSiteHelperController.cls", "LastMetadataAPIConnectionException = e;"},
				{"apex-mdapi/src/classes/RemoteSiteHelperTest.cls", "Test.setMock(WebServiceMock.class, new WebServiceMockImpl());"},
				{"apex-mdapi/src/classes/RemoteSiteHelperTest.cls", "System.assertEquals(false, controller.MetadataConnectionWarning);"},
				{"apex-mdapi/src/classes/RemoteSiteHelperTest.cls", "System.assertEquals(true, controller.MetadataConnectionWarning);"},
				{"apex-mdapi/src/classes/MetadataDeployController.cls", "public PageReference deployZip()"},
			},
			targets: []extraAuditSymbol{
				{"apex-mdapi/src/classes/RemoteSiteHelperController.cls", "displayMetadataResponse", 28},
				{"apex-mdapi/src/classes/RemoteSiteHelperController.cls", "checkMetadataAPIConnection", 45},
				{"apex-mdapi/src/classes/RemoteSiteHelperTest.cls", "testCheckMetadataAPIConnection", 5},
			},
		},
		{
			project: "apex-dlrs", language: "apex",
			// Determine why rollup parent updates are skipped or issued:
			// compare records, configuration bypass and dedicated tests.
			search:        "getOnlyRecordsNeedingDml",
			primary:       "dlrs/main/classes/RollupDmlGuard.cls",
			negativeFile:  "dlrs/main/classes/RollupJob.cls",
			negativeQuery: "getOnlyRecordsNeedingDml",
			paths: []string{
				"dlrs/main/classes/RollupDmlGuard.cls",
				"dlrs/main/classes/RollupDmlGuardTest.cls",
				"dlrs/main/classes/RollupJob.cls",
			},
			oracles: []extraAuditEvidence{
				{"dlrs/main/classes/RollupDmlGuard.cls", "public with sharing class RollupDmlGuard"},
				{"dlrs/main/classes/RollupDmlGuard.cls", "if (configSettings.DisableParentDMLCheck__c) {"},
				{"dlrs/main/classes/RollupDmlGuard.cls", "return records.values();"},
				{"dlrs/main/classes/RollupDmlGuard.cls", "filterParentRecords(masterRecordsByType.get(t), parentsByType.get(t))"},
				{"dlrs/main/classes/RollupDmlGuard.cls", "if (expP.get(key) != curP.get(key)) {"},
				{"dlrs/main/classes/RollupDmlGuardTest.cls", "static void testRollupDmlGuardChangeNeeded() {"},
				{"dlrs/main/classes/RollupDmlGuardTest.cls", "static void testRollupDmlGuardChangeNotNeeded() {"},
				{"dlrs/main/classes/RollupDmlGuardTest.cls", "static void testRollupGuardDisableViaSettings() {"},
				{"dlrs/main/classes/RollupDmlGuardTest.cls", "DisableParentDMLCheck__c = true"},
				{"dlrs/main/classes/RollupJob.cls", "RollupService.runJobToProcessScheduledItems();"},
			},
			targets: []extraAuditSymbol{
				{"dlrs/main/classes/RollupDmlGuard.cls", "getOnlyRecordsNeedingDml", 6},
				{"dlrs/main/classes/RollupDmlGuard.cls", "getParentRecords", 68},
				{"dlrs/main/classes/RollupDmlGuard.cls", "filterParentRecords", 93},
				{"dlrs/main/classes/RollupDmlGuardTest.cls", "testRollupDmlGuardChangeNeeded", 4},
				{"dlrs/main/classes/RollupDmlGuardTest.cls", "testRollupDmlGuardChangeNotNeeded", 32},
				{"dlrs/main/classes/RollupDmlGuardTest.cls", "testRollupGuardDisableViaSettings", 62},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
