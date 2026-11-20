package handler

import (
	"os"
	"testing"
)

// Real-project tasks and expectations are derived from read-only inspection
// of the independent PureBasic projects before using source_*.
func TestSourceSymbolsOriginalPureBasicAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original PureBasic corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "purebasic-chrisrfr-icedesign", language: "purebasic",
			search: "GridMatch", primary: "IceDesign_Old_beta03.pb",
			negativeFile: "PBForm2IceDesign.pbi", negativeQuery: "GridMatch",
			paths: []string{"IceDesign_Old_beta03.pb", "PBForm2IceDesign.pbi"},
			oracles: []extraAuditEvidence{
				{"IceDesign_Old_beta03.pb", "Global DragSpace.i = 10 , GridSize.i = 10, SnapGrid.b = #True, ShowGrid.b = #True"},
				{"IceDesign_Old_beta03.pb", "Declare.i GridMatch(Value.i, Grid.i=1, Min.i=0, Max.i=4096)"},
				{"IceDesign_Old_beta03.pb", "Procedure.i GridMatch(Value.i, Grid.i=1, Min.i=0, Max.i=4096)"},
				{"IceDesign_Old_beta03.pb", "Value = Round(Value/Grid, #PB_Round_Nearest)*Grid"},
				{"IceDesign_Old_beta03.pb", "Procedure CreateGadget(Model.s"},
				{"IceDesign_Old_beta03.pb", "X = GridMatch(X, DragSpace, 0, MaxSizeX-#MinSize)"},
				{"IceDesign_Old_beta03.pb", "Width = GridMatch(Width, DragSpace, #MinSize, MaxSizeX-X)"},
				{"IceDesign_Old_beta03.pb", "Procedure DrawGrid(GridArea)"},
				{"IceDesign_Old_beta03.pb", "If ShowGrid = #True"},
				{"IceDesign_Old_beta03.pb", "DrawGrid(GridArea)"},
				{"PBForm2IceDesign.pbi", "Procedure PBForm2IceDesign(Window = #PB_All)"},
				{"PBForm2IceDesign.pbi", "LoadModelObject()"},
				{"PBForm2IceDesign.pbi", "LoadObjectPB()"},
			},
			targets: []extraAuditSymbol{
				{"IceDesign_Old_beta03.pb", "GridMatch", 162},
				{"IceDesign_Old_beta03.pb", "CreateGadget", 559},
				{"IceDesign_Old_beta03.pb", "DrawGrid", 757},
				{"PBForm2IceDesign.pbi", "PBForm2IceDesign", 1781},
			},
		},
		{
			project: "purebasic-danilo-raylib", language: "purebasic",
			search: "UpdateCameraCenterSmoothFollow", primary: "examples/core/core_2d_camera_platformer.pb",
			negativeFile: "examples/core/core_2d_camera.pb", negativeQuery: "UpdateCameraCenterSmoothFollow",
			paths: []string{
				"examples/core/core_2d_camera_platformer.pb",
				"examples/core/core_2d_camera.pb",
				"raylib.pbi",
			},
			oracles: []extraAuditEvidence{
				{"examples/core/core_2d_camera_platformer.pb", "Declare UpdateCameraCenterSmoothFollow("},
				{"examples/core/core_2d_camera_platformer.pb", "cameraUpdaters(2) = @UpdateCameraCenterSmoothFollow()"},
				{"examples/core/core_2d_camera_platformer.pb", "cameraOption = (cameraOption + 1) % ArraySize( cameraUpdaters() )"},
				{"examples/core/core_2d_camera_platformer.pb", "Procedure UpdateCameraCenterSmoothFollow("},
				{"examples/core/core_2d_camera_platformer.pb", "Static.f minSpeed = 30, minEffectLength = 10, fractionSpeed = 0.8"},
				{"examples/core/core_2d_camera_platformer.pb", "Procedure UpdateCameraCenterInsideMap("},
				{"examples/core/core_2d_camera_platformer.pb", "Procedure UpdateCameraEvenOutOnLanding("},
				{"examples/core/core_2d_camera.pb", "camera\\zoom = 1.0"},
				{"examples/core/core_2d_camera.pb", "camera\\rotation = 0.0"},
				{"raylib.pbi", "DeclareModule ray"},
			},
			targets: []extraAuditSymbol{
				{"examples/core/core_2d_camera_platformer.pb", "UpdatePlayer", 197},
				{"examples/core/core_2d_camera_platformer.pb", "UpdateCameraCenterInsideMap", 244},
				{"examples/core/core_2d_camera_platformer.pb", "UpdateCameraCenterSmoothFollow", 280},
				{"examples/core/core_2d_camera_platformer.pb", "UpdateCameraEvenOutOnLanding", 300},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
