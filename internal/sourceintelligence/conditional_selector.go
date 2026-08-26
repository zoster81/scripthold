package sourceintelligence

const conditionalVariantLimit = 32

type conditionalBranch struct {
	start int
	end   int
}

type conditionalGroup struct {
	parentGroup    int
	parentBranch   int
	branches       []conditionalBranch
	conditionKey   string
	conditionState []int8
}

type conditionalPlan struct {
	groups     []conditionalGroup
	directives []OffsetRange
	issue      *OffsetRange
	message    string
}

type conditionalFrame struct {
	group    int
	branch   int
	seenElse bool
}

type conditionalChoice struct {
	group  int
	branch int
}

type conditionalVariant struct {
	selection  []int
	states     []int8
	stateValid bool
}

func conditionalSelections(groups []conditionalGroup) ([][]int, bool) {
	if len(groups) == 0 {
		return [][]int{{}}, true
	}

	groupStateIDs := make([]int, len(groups))
	for index := range groupStateIDs {
		groupStateIDs[index] = -1
	}
	conditionIDs := make(map[string]int)
	for groupID, group := range groups {
		if group.conditionKey == "" {
			continue
		}
		stateID, exists := conditionIDs[group.conditionKey]
		if !exists {
			stateID = len(conditionIDs)
			conditionIDs[group.conditionKey] = stateID
		}
		groupStateIDs[groupID] = stateID
	}

	variants := make([]conditionalVariant, 0, 4)
	choices := make([]conditionalChoice, 0, 8)
	touchedStates := make([]int, 0, 8)
	requirementStates := make([]int8, len(conditionIDs))
	requirementMarks := make([]uint32, len(conditionIDs))
	var generation uint32

	for groupID, group := range groups {
		for branchID := range group.branches {
			choices = choices[:0]
			choices = append(choices, conditionalChoice{group: groupID, branch: branchID})
			parentGroup := group.parentGroup
			parentBranch := group.parentBranch
			for parentGroup >= 0 {
				if parentGroup >= len(groups) || parentBranch < 0 || parentBranch >= len(groups[parentGroup].branches) {
					return nil, false
				}
				choices = append(choices, conditionalChoice{group: parentGroup, branch: parentBranch})
				parent := groups[parentGroup]
				parentGroup, parentBranch = parent.parentGroup, parent.parentBranch
			}

			generation++
			if generation == 0 {
				clear(requirementMarks)
				generation = 1
			}
			touchedStates = touchedStates[:0]
			requirementStateValid := true
			for _, choice := range choices {
				stateID := groupStateIDs[choice.group]
				if stateID < 0 {
					continue
				}
				stateGroup := groups[choice.group]
				if choice.branch >= len(stateGroup.conditionState) {
					continue
				}
				state := stateGroup.conditionState[choice.branch]
				if state == 0 {
					continue
				}
				if requirementMarks[stateID] == generation {
					if requirementStates[stateID] != state {
						requirementStateValid = false
					}
					continue
				}
				requirementMarks[stateID] = generation
				requirementStates[stateID] = state
				touchedStates = append(touchedStates, stateID)
			}

			placed := false
			for variantID := range variants {
				variant := &variants[variantID]
				compatible := true
				for _, choice := range choices {
					selected := variant.selection[choice.group]
					if selected >= 0 && selected != choice.branch {
						compatible = false
						break
					}
				}
				if compatible && len(conditionIDs) > 0 {
					if !variant.stateValid || !requirementStateValid {
						compatible = false
					} else {
						for _, stateID := range touchedStates {
							if current := variant.states[stateID]; current != 0 && current != requirementStates[stateID] {
								compatible = false
								break
							}
						}
					}
				}
				if !compatible {
					continue
				}
				for _, choice := range choices {
					variant.selection[choice.group] = choice.branch
				}
				for _, stateID := range touchedStates {
					if variant.states[stateID] == 0 {
						variant.states[stateID] = requirementStates[stateID]
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

			selection := make([]int, len(groups))
			for index := range selection {
				selection[index] = -1
			}
			for _, choice := range choices {
				selection[choice.group] = choice.branch
			}
			states := make([]int8, len(conditionIDs))
			for _, stateID := range touchedStates {
				states[stateID] = requirementStates[stateID]
			}
			variants = append(variants, conditionalVariant{selection: selection, states: states, stateValid: requirementStateValid})
		}
	}

	selections := make([][]int, len(variants))
	for variantID := range variants {
		selection := variants[variantID].selection
		for groupID := range selection {
			if selection[groupID] < 0 {
				selection[groupID] = 0
			}
		}
		selections[variantID] = selection
	}
	return selections, true
}

func maskConditionalVariant(text string, plan conditionalPlan, selection []int) string {
	masked := []byte(text)
	mask := func(value OffsetRange) {
		start, end := max(0, value.Start), min(len(masked), value.End)
		for index := start; index < end; index++ {
			if masked[index] != '\r' && masked[index] != '\n' {
				masked[index] = ' '
			}
		}
	}
	for _, directive := range plan.directives {
		mask(directive)
	}
	for groupID, group := range plan.groups {
		selected := 0
		if groupID < len(selection) && selection[groupID] >= 0 && selection[groupID] < len(group.branches) {
			selected = selection[groupID]
		}
		for branchID, branch := range group.branches {
			if branchID != selected {
				mask(OffsetRange{Start: branch.start, End: branch.end})
			}
		}
	}
	return string(masked)
}
