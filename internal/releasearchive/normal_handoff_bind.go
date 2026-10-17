package releasearchive

import (
	"errors"
	"strings"
)

// BindRawNormalHandoffPlan requires the downloaded-file plan to match a
// separately authenticated complete original asset inventory. It does not
// authenticate that inventory, GitHub jobs or artifact IDs, or file bytes.
// The trusted array must be supplied independently of the decoded JSON plan.
// Both inputs are copied into the returned plan, never retained by reference.
func BindRawNormalHandoffPlan(plan []RawNormalFile, trusted []ExpectedFile) ([]RawNormalFile, error) {
	if _, _, err := validateRawNormalEntries(plan); err != nil {
		return nil, err
	}
	if len(trusted) != 15 {
		return nil, errors.New("trusted normal release inventory requires exactly 15 originals")
	}
	byName := make(map[string]ExpectedFile, len(trusted))
	folded := make(map[string]bool, len(trusted))
	for _, original := range trusted {
		if !portableStagingName(original.Name) ||
			original.Size <= 0 || original.Size > maxAssetBytes ||
			!validDigest(original.SHA256) {
			return nil, errors.New("invalid trusted normal release asset")
		}
		key := strings.ToLower(original.Name)
		if folded[key] {
			return nil, errors.New("duplicate trusted normal release asset")
		}
		folded[key] = true
		byName[original.Name] = original
	}
	for _, item := range plan {
		original, ok := byName[item.Original.Name]
		if !ok || original != item.Original {
			return nil, errors.New("raw normal file disagrees with independently trusted original")
		}
	}
	return append([]RawNormalFile(nil), plan...), nil
}
