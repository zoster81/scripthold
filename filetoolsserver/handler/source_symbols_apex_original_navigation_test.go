package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These original-project maintenance questions and expected edit locations
// come from direct source/test reads, independently of Source Intelligence.
func TestSourceSymbolsOriginalApexMaintenanceNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Apex corpus not configured")
	}
	type evidence struct{ file, text string }
	type target struct {
		file, name, excerpt string
		line                int
	}
	cases := []struct {
		project, query, unrelated string
		files                     []string
		evidence                  []evidence
		targets                   []target
	}{
		{
			project:   "apex-recipes",
			query:     "incrementCounterInDescription",
			unrelated: "force-app/main/default/classes/Integration Recipes/ApiServiceRecipes.cls",
			files: []string{
				"force-app/main/default/classes/Shared Code/AccountServiceLayer.cls",
				"force-app/tests/Shared Code/AccountServiceLayer_Tests.cls",
				"force-app/main/default/classes/Trigger Recipes/AccountTriggerHandler.cls",
				"force-app/main/default/classes/Integration Recipes/ApiServiceRecipes.cls",
			},
			evidence: []evidence{
				{"force-app/main/default/classes/Shared Code/AccountServiceLayer.cls", "counter = Integer.valueOf(ia.Description);"},
				{"force-app/main/default/classes/Shared Code/AccountServiceLayer.cls", "counter++;"},
				{"force-app/main/default/classes/Shared Code/AccountServiceLayer.cls", "safelySave(incomingAccounts);"},
				{"force-app/main/default/classes/Trigger Recipes/AccountTriggerHandler.cls", "AccountServiceLayer.incrementCounterInDescription("},
				{"force-app/main/default/classes/Trigger Recipes/AccountTriggerHandler.cls", "AccountServiceLayer.changeShippingStreet("},
				{"force-app/tests/Shared Code/AccountServiceLayer_Tests.cls", "static void incrementDescriptionOnBulkAccountsPositiveInvalidInteger()"},
				{"force-app/tests/Shared Code/AccountServiceLayer_Tests.cls", "new Account(description = 'Hello')"},
				{"force-app/tests/Shared Code/AccountServiceLayer_Tests.cls", "static void incrementDescriptionOnBulkAccountsNegativeInvalidIntegerSaves()"},
			},
			targets: []target{
				{"force-app/main/default/classes/Shared Code/AccountServiceLayer.cls", "incrementCounterInDescription", "counter = Integer.valueOf(ia.Description);", 79},
				{"force-app/main/default/classes/Shared Code/AccountServiceLayer.cls", "safelySave", "Security.stripInaccessible(", 120},
				{"force-app/main/default/classes/Trigger Recipes/AccountTriggerHandler.cls", "beforeInsert", "AccountServiceLayer.incrementCounterInDescription(", 47},
				{"force-app/tests/Shared Code/AccountServiceLayer_Tests.cls", "incrementDescriptionOnBulkAccountsPositiveInvalidInteger", "new Account(description = 'Hello')", 72},
			},
		},
		{
			project:   "apex-fflib-common",
			query:     "registerDirty",
			unrelated: "sfdx-source/apex-common/main/classes/fflib_SObjectSelector.cls",
			files: []string{
				"sfdx-source/apex-common/main/classes/fflib_SObjectUnitOfWork.cls",
				"sfdx-source/apex-common/test/classes/fflib_SObjectUnitOfWorkTest.cls",
				"sfdx-source/apex-common/main/classes/fflib_SObjectSelector.cls",
			},
			evidence: []evidence{
				{"sfdx-source/apex-common/main/classes/fflib_SObjectUnitOfWork.cls", "if (!m_dirtyMapByType.get(sObjectType).containsKey(record.Id) || dirtyFields.isEmpty())"},
				{"sfdx-source/apex-common/main/classes/fflib_SObjectUnitOfWork.cls", "registeredRecord.put(dirtyField, record.get(dirtyField));"},
				{"sfdx-source/apex-common/main/classes/fflib_SObjectUnitOfWork.cls", "m_dml.dmlUpdate(m_dirtyMapByType.get(sObjectType.getDescribe().getName()).values());"},
				{"sfdx-source/apex-common/test/classes/fflib_SObjectUnitOfWorkTest.cls", "private static void testRegisterDirty_field()"},
				{"sfdx-source/apex-common/test/classes/fflib_SObjectUnitOfWorkTest.cls", "uow.registerDirty(amountUpdate, new List<SObjectField> { Opportunity.Amount } );"},
				{"sfdx-source/apex-common/test/classes/fflib_SObjectUnitOfWorkTest.cls", "private static void testRegisterDirtyRecordsWithDirtyFields()"},
				{"sfdx-source/apex-common/main/classes/fflib_SObjectSelector.cls", "selectSObjectsById(Set<Id> idSet)"},
			},
			targets: []target{
				{"sfdx-source/apex-common/main/classes/fflib_SObjectUnitOfWork.cls", "registerDirty", "registeredRecord.put(dirtyField, record.get(dirtyField));", 414},
				{"sfdx-source/apex-common/main/classes/fflib_SObjectUnitOfWork.cls", "commitWork", "doCommitWork();", 712},
				{"sfdx-source/apex-common/main/classes/fflib_SObjectUnitOfWork.cls", "updateDmlByType", "m_dml.dmlUpdate(", 798},
				{"sfdx-source/apex-common/test/classes/fflib_SObjectUnitOfWorkTest.cls", "testRegisterDirty_field", "uow.registerDirty(amountUpdate, new List<SObjectField>", 443},
				{"sfdx-source/apex-common/test/classes/fflib_SObjectUnitOfWorkTest.cls", "testRegisterDirtyRecordsWithDirtyFields", "uow.registerDirty(recordsWithStageUpdate, new List<SObjectField>", 468},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			h := NewHandler([]string{root})
			paths := make([]string, 0, len(tc.files))
			for _, rel := range tc.files {
				path := filepath.Join(root, filepath.FromSlash(rel))
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("original project source unavailable %s: %v", rel, err)
				}
				paths = append(paths, path)
			}
			for _, want := range tc.evidence {
				path := filepath.Join(root, filepath.FromSlash(want.file))
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("original oracle file unavailable %s: %v", want.file, err)
				}
				body, err := os.ReadFile(path)
				if err != nil || !strings.Contains(string(body), want.text) {
					t.Fatalf("independent oracle %s %q: %v", want.file, want.text, err)
				}
			}
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: "apex",
				Encoding: "utf-8", Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || failed != nil || digest.Truncated || digest.FilesParsed != len(paths) {
				t.Fatalf("compact map error=%v failed=%+v digest=%+v", err, failed, digest)
			}
			data, err := json.Marshal(digest)
			if err != nil || len(data) > 8192 {
				t.Fatalf("compact map exceeded budget: bytes=%d err=%v", len(data), err)
			}
			t.Logf("originalProject=1 files=%d symbols=%d compactBytes=%d coverageComplete=%v",
				digest.FilesParsed, digest.SymbolCount, len(data), digest.CoverageComplete)
			for _, want := range tc.targets {
				path := filepath.Join(root, filepath.FromSlash(want.file))
				failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Query: want.name,
					Language: "apex", Encoding: "utf-8", Match: "exact",
					MaxFiles: 1, MaxSymbols: 32,
				})
				if err != nil || failed != nil || found.Truncated || len(found.Files) != 1 {
					t.Fatalf("find %s: error=%v failed=%+v result=%+v", want.name, err, failed, found)
				}
				var symbolID string
				for _, sym := range found.Symbols {
					if sym.Name == want.name && string(sym.Kind) == "method" &&
						sym.NameRange.Start.Line == want.line {
						symbolID = sym.ID
						break
					}
				}
				if symbolID == "" {
					t.Fatalf("missing source-confirmed Apex method %s:%d %s; results=%+v",
						want.file, want.line, want.name, found.Symbols)
				}
				failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "show", Path: path, SymbolID: symbolID,
					SourceFingerprint: found.Files[0].SourceFingerprint,
					Language:          "apex", Encoding: "utf-8", MaxBytes: 8192,
				})
				if err != nil || failed != nil || shown.Show == nil ||
					!strings.Contains(shown.Show.Text, want.excerpt) {
					t.Fatalf("fingerprint-bound show %s: error=%v failed=%+v show=%+v",
						want.name, err, failed, shown.Show)
				}
			}
			failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Paths: paths, Query: tc.query,
				Language: "apex", Encoding: "utf-8", Match: "exact",
				Mode: "structural", MaxResults: 24,
			})
			if err != nil || failed != nil || search.Search == nil || len(search.Search.Matches) == 0 {
				t.Fatalf("structural navigation search %s: error=%v failed=%+v search=%+v",
					tc.query, err, failed, search.Search)
			}
			failed, negative, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{filepath.Join(root, filepath.FromSlash(tc.unrelated))},
				Query: tc.query, Language: "apex", Encoding: "utf-8",
				Match: "exact", MaxFiles: 1, MaxSymbols: 16,
			})
			if err != nil || failed != nil || negative.Truncated || len(negative.Symbols) != 0 {
				t.Fatalf("unrelated Apex component falsely declares %s: error=%v failed=%+v symbols=%+v",
					tc.query, err, failed, negative.Symbols)
			}
		})
	}
}
