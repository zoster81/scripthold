package sourceintelligence

import (
	"reflect"
	"testing"
)

func TestConditionalSelectionsMatchLegacyPacking(t *testing.T) {
	independent := make([]conditionalGroup, 40)
	for index := range independent {
		independent[index] = conditionalGroup{parentGroup: -1, parentBranch: -1, branches: []conditionalBranch{{}, {}}}
	}

	cases := []struct {
		name   string
		groups []conditionalGroup
	}{
		{name: "independent", groups: independent},
		{name: "nested", groups: []conditionalGroup{
			{parentGroup: -1, parentBranch: -1, branches: []conditionalBranch{{}, {}}},
			{parentGroup: 0, parentBranch: 0, branches: []conditionalBranch{{}, {}, {}}},
			{parentGroup: 1, parentBranch: 2, branches: []conditionalBranch{{}, {}}},
		}},
		{name: "correlated", groups: []conditionalGroup{
			{parentGroup: -1, parentBranch: -1, branches: []conditionalBranch{{}, {}}, conditionKey: "macro:0:0:FEATURE", conditionState: []int8{1, -1}},
			{parentGroup: -1, parentBranch: -1, branches: []conditionalBranch{{}, {}}, conditionKey: "macro:0:0:FEATURE", conditionState: []int8{1, -1}},
			{parentGroup: -1, parentBranch: -1, branches: []conditionalBranch{{}, {}}, conditionKey: "macro:0:0:OTHER", conditionState: []int8{1, -1}},
		}},
		{name: "internally-conflicting-requirement", groups: []conditionalGroup{
			{parentGroup: -1, parentBranch: -1, branches: []conditionalBranch{{}, {}}, conditionKey: "macro:0:0:X", conditionState: []int8{1, -1}},
			{parentGroup: 0, parentBranch: 0, branches: []conditionalBranch{{}, {}}, conditionKey: "macro:0:0:X", conditionState: []int8{-1, 1}},
		}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, gotOK := conditionalSelections(testCase.groups)
			want, wantOK := legacyConditionalSelections(testCase.groups)
			if gotOK != wantOK || !reflect.DeepEqual(got, want) {
				t.Fatalf("conditionalSelections() = (%v, %t), legacy = (%v, %t)", got, gotOK, want, wantOK)
			}
		})
	}
}

func TestConditionalSelectionsRejectMalformedParentGraphs(t *testing.T) {
	cases := []struct {
		name   string
		groups []conditionalGroup
	}{
		{
			name: "out-of-range-parent",
			groups: []conditionalGroup{{
				parentGroup: 3, parentBranch: 0, branches: []conditionalBranch{{}, {}},
			}},
		},
		{
			name: "cyclic-parent",
			groups: []conditionalGroup{{
				parentGroup: 0, parentBranch: 0, branches: []conditionalBranch{{}, {}},
			}},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if selections, ok := conditionalSelections(testCase.groups); ok || selections != nil {
				t.Fatalf("conditionalSelections() = (%v, %t), want bounded rejection", selections, ok)
			}
		})
	}
}

func legacyConditionalSelections(groups []conditionalGroup) ([][]int, bool) {
	if len(groups) == 0 {
		return [][]int{{}}, true
	}

	hasConditionState := false
	for _, group := range groups {
		if group.conditionKey != "" {
			hasConditionState = true
			break
		}
	}

	variants := make([][]int, 0, 4)
	for groupID, group := range groups {
		for branchID := range group.branches {
			requirement := make([]int, len(groups))
			for index := range requirement {
				requirement[index] = -1
			}
			requirement[groupID] = branchID
			parentGroup := group.parentGroup
			parentBranch := group.parentBranch
			for parentGroup >= 0 {
				if parentGroup >= len(groups) || parentBranch < 0 || parentBranch >= len(groups[parentGroup].branches) {
					return nil, false
				}
				requirement[parentGroup] = parentBranch
				parent := groups[parentGroup]
				parentGroup, parentBranch = parent.parentGroup, parent.parentBranch
			}

			placed := false
			for variantID := range variants {
				compatible := true
				for index, branch := range requirement {
					if branch >= 0 && variants[variantID][index] >= 0 && variants[variantID][index] != branch {
						compatible = false
						break
					}
				}
				if compatible && hasConditionState && !legacyConditionalStateCompatible(groups, variants[variantID], requirement) {
					compatible = false
				}
				if !compatible {
					continue
				}
				for index, branch := range requirement {
					if branch >= 0 {
						variants[variantID][index] = branch
					}
				}
				placed = true
				break
			}
			if placed {
				continue
			}
			if len(variants) >= conditionalVariantLimit {
				return nil, false
			}
			variants = append(variants, requirement)
		}
	}

	selections := make([][]int, len(variants))
	for variantID, variant := range variants {
		selection := append([]int(nil), variant...)
		for groupID := range selection {
			if selection[groupID] < 0 {
				selection[groupID] = 0
			}
		}
		selections[variantID] = selection
	}
	return selections, true
}

func legacyConditionalStateCompatible(groups []conditionalGroup, left, right []int) bool {
	states := make(map[string]int8)
	check := func(selection []int) bool {
		for groupID, branchID := range selection {
			if branchID < 0 || groupID >= len(groups) {
				continue
			}
			group := groups[groupID]
			if group.conditionKey == "" || branchID >= len(group.conditionState) {
				continue
			}
			state := group.conditionState[branchID]
			if state == 0 {
				continue
			}
			if current, exists := states[group.conditionKey]; exists && current != state {
				return false
			}
			states[group.conditionKey] = state
		}
		return true
	}
	return check(left) && check(right)
}
