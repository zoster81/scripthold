package handler

import (
	"os"
	"testing"
)

// These source-first maintenance cases verify navigation, not MATLAB execution.
func TestSourceSymbolsOriginalMATLABAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original MATLAB corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "matlab-agentic-toolkit", language: "matlab",
			// Locate changes to dataset classification characteristics and thresholds.
			search:        "compute_data_flags",
			primary:       "skills-catalog/ai-and-statistics/matlab-classify-tabular-data/scripts/compute_data_flags.m",
			negativeFile:  "skills-catalog/ai-and-statistics/matlab-classify-tabular-data/scripts/train_and_score_cv.m",
			negativeQuery: "compute_data_flags",
			paths: []string{
				"skills-catalog/ai-and-statistics/matlab-classify-tabular-data/scripts/compute_data_flags.m",
				"skills-catalog/ai-and-statistics/matlab-classify-tabular-data/scripts/build_model_definitions.m",
				"skills-catalog/ai-and-statistics/matlab-classify-tabular-data/references/classifier_thresholds.m",
				"skills-catalog/ai-and-statistics/matlab-classify-tabular-data/scripts/train_and_score_cv.m",
			},
			oracles: []extraAuditEvidence{
				{"skills-catalog/ai-and-statistics/matlab-classify-tabular-data/scripts/compute_data_flags.m", "function flags = compute_data_flags(X, Y)"},
				{"skills-catalog/ai-and-statistics/matlab-classify-tabular-data/scripts/compute_data_flags.m", "flags.isImbalanced      = classRatio > 5;"},
				{"skills-catalog/ai-and-statistics/matlab-classify-tabular-data/scripts/compute_data_flags.m", "flags.hasManyMissing    = local_has_many_missing(X, N);"},
				{"skills-catalog/ai-and-statistics/matlab-classify-tabular-data/scripts/compute_data_flags.m", "function tf = local_has_many_missing(X, N)"},
				{"skills-catalog/ai-and-statistics/matlab-classify-tabular-data/scripts/build_model_definitions.m", "function modelDefs = build_model_definitions(flags, skillPath, varargin)"},
				{"skills-catalog/ai-and-statistics/matlab-classify-tabular-data/scripts/build_model_definitions.m", "function b = pick_branch(BRANCHES, flags)"},
				{"skills-catalog/ai-and-statistics/matlab-classify-tabular-data/references/classifier_thresholds.m", "THRESHOLDS.many_missing_frac   = 0.05;"},
				{"skills-catalog/ai-and-statistics/matlab-classify-tabular-data/references/classifier_thresholds.m", "THRESHOLDS.imbalance_ratio           = 5;"},
				{"skills-catalog/ai-and-statistics/matlab-classify-tabular-data/scripts/train_and_score_cv.m", "function [cvModel, trainTime, acc, accCI, foldAcc] = train_and_score_cv(cvFitFcn, X, Y, cv)"},
			},
			targets: []extraAuditSymbol{
				{"skills-catalog/ai-and-statistics/matlab-classify-tabular-data/scripts/compute_data_flags.m", "compute_data_flags", 1},
				{"skills-catalog/ai-and-statistics/matlab-classify-tabular-data/scripts/compute_data_flags.m", "local_has_many_missing", 65},
				{"skills-catalog/ai-and-statistics/matlab-classify-tabular-data/scripts/build_model_definitions.m", "build_model_definitions", 1},
				{"skills-catalog/ai-and-statistics/matlab-classify-tabular-data/scripts/build_model_definitions.m", "pick_branch", 129},
				{"skills-catalog/ai-and-statistics/matlab-classify-tabular-data/scripts/train_and_score_cv.m", "train_and_score_cv", 1},
			},
		},
		{
			project: "matlab-eviltransform", language: "matlab",
			// Locate precision and iteration changes to the exact GCJ-to-WGS transform.
			search: "gcj2wgs_exact", primary: "matlab/gcj2wgs_exact.m",
			negativeFile: "matlab/UTM/deg2utm.m", negativeQuery: "gcj2wgs_exact",
			paths: []string{
				"matlab/gcj2wgs_exact.m", "matlab/wgs2gcj.m", "matlab/gcj2wgs.m",
				"matlab/delta.m", "matlab/outOfChina.m", "matlab/test/testSuite.m",
				"matlab/UTM/deg2utm.m",
			},
			oracles: []extraAuditEvidence{
				{"matlab/gcj2wgs_exact.m", "function [lat, long ] = gcj2wgs_exact(gcjLat, gcjLng)"},
				{"matlab/gcj2wgs_exact.m", "threshold = 0.000001;"},
				{"matlab/gcj2wgs_exact.m", "for i=1:30"},
				{"matlab/gcj2wgs_exact.m", "[tmplat, tmplng] = wgs2gcj(wgsLat, wgsLng);"},
				{"matlab/gcj2wgs.m", "% Not exact"},
				{"matlab/gcj2wgs.m", "[dlat, dlng] = delta(gcjLat(inChina), gcjLng(inChina));"},
				{"matlab/outOfChina.m", "function result = outOfChina(lat,long)"},
				{"matlab/test/testSuite.m", "function gcj2wgsExactTest(testCase)"},
				{"matlab/test/testSuite.m", "[actLat actLong] = gcj2wgs_exact(TESTS(:,3),TESTS(:,4));"},
				{"matlab/test/testSuite.m", "verifyEqual(testCase,actualDistance,measurementDistance, 'AbsTol' , .5);"},
				{"matlab/UTM/deg2utm.m", "function  [x,y,utmzone] = deg2utm(lat,long)"},
			},
			targets: []extraAuditSymbol{
				{"matlab/gcj2wgs_exact.m", "gcj2wgs_exact", 1},
				{"matlab/gcj2wgs.m", "gcj2wgs", 1},
				{"matlab/wgs2gcj.m", "wgs2gcj", 1},
				{"matlab/test/testSuite.m", "gcj2wgsExactTest", 52},
				{"matlab/test/testSuite.m", "gcj2wgsTest", 30},
				{"matlab/UTM/deg2utm.m", "deg2utm", 1},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
