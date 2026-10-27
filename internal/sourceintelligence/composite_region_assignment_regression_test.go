package sourceintelligence

import (
	"fmt"
	"math/rand"
	"testing"
)

func TestCompositeRegionAssignmentPreservesFirstMatchingRegion(t *testing.T) {
	for _, tc := range []struct {
		name    string
		regions []compositeEmbeddedRegion
		offsets []int
		want    []string
	}{
		{
			name: "ordered disjoint regions and half-open boundaries",
			regions: []compositeEmbeddedRegion{
				{kind: "first", content: OffsetRange{Start: 4, End: 8}},
				{kind: "second", content: OffsetRange{Start: 12, End: 16}},
			},
			offsets: []int{0, 4, 7, 8, 11, 12, 15, 16},
			want:    []string{"original", "first-1", "first-1", "original", "original", "second-2", "second-2", "original"},
		},
		{
			name: "unsorted input preserves input-order precedence",
			regions: []compositeEmbeddedRegion{
				{kind: "later", content: OffsetRange{Start: 20, End: 30}},
				{kind: "earlier", content: OffsetRange{Start: 0, End: 10}},
			},
			offsets: []int{1, 22},
			want:    []string{"earlier-2", "later-1"},
		},
		{
			name: "overlapping regions prefer earliest matching index",
			regions: []compositeEmbeddedRegion{
				{kind: "outer", content: OffsetRange{Start: 2, End: 30}},
				{kind: "inner", content: OffsetRange{Start: 10, End: 15}},
			},
			offsets: []int{11, 29, 30},
			want:    []string{"outer-1", "outer-1", "original"},
		},
		{
			name:    "no regions retains an existing region ID",
			offsets: []int{3, 10},
			want:    []string{"original", "original"},
		},
		{
			name: "zero length region is ignored",
			regions: []compositeEmbeddedRegion{
				{kind: "empty", content: OffsetRange{Start: 9, End: 9}},
				{kind: "valid", content: OffsetRange{Start: 9, End: 10}},
			},
			offsets: []int{9, 10},
			want:    []string{"valid-2", "original"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			symbols := make([]NormalizedSymbol, len(tc.offsets))
			for i, offset := range tc.offsets {
				symbols[i].declarationOffsets.Start = offset
				symbols[i].RegionID = "original"
			}
			assignEmbeddedRegionIDs(symbols, tc.regions)
			for i, symbol := range symbols {
				if symbol.RegionID != tc.want[i] {
					t.Errorf("symbol %d offset %d: got region %q, want %q", i, tc.offsets[i], symbol.RegionID, tc.want[i])
				}
			}
		})
	}
}

// Compare the indexed and original linear paths across arbitrary layouts,
// not just the ordered non-overlapping regions produced by composite parsers.
func TestCompositeRegionAssignmentMatchesLegacyOnArbitraryLayouts(t *testing.T) {
	random := rand.New(rand.NewSource(42))
	for iteration := 0; iteration < 200; iteration++ {
		regionCount := 1 + random.Intn(24)
		regions := make([]compositeEmbeddedRegion, regionCount)
		for index := range regions {
			start := random.Intn(200) - 20
			end := start + random.Intn(32) - 4
			regions[index] = compositeEmbeddedRegion{
				kind:    fmt.Sprintf("region%d", index),
				content: OffsetRange{Start: start, End: end},
			}
		}
		if iteration%2 == 0 {
			// Include sorted, disjoint arrangements where binary search is used.
			for index := range regions {
				start := index*20 + random.Intn(5)
				regions[index].content = OffsetRange{Start: start, End: start + random.Intn(17)}
			}
		}
		want := make([]NormalizedSymbol, 32)
		got := make([]NormalizedSymbol, len(want))
		for index := range want {
			offset := random.Intn(240) - 20
			want[index].declarationOffsets.Start = offset
			want[index].RegionID = "untouched"
		}
		copy(got, want)
		assignEmbeddedRegionIDsLinear(want, regions)
		assignEmbeddedRegionIDs(got, regions)
		for index := range got {
			if got[index].RegionID != want[index].RegionID {
				t.Fatalf("iteration %d region %d symbol %d offset %d: got %q, want %q",
					iteration, regionCount, index, got[index].declarationOffsets.Start,
					got[index].RegionID, want[index].RegionID)
			}
		}
	}
}

func BenchmarkCompositeRegionAssignment(b *testing.B) {
	for _, count := range []int{16, 128, 512} {
		b.Run(fmt.Sprintf("regions_%d", count), func(b *testing.B) {
			regions := make([]compositeEmbeddedRegion, count)
			symbols := make([]NormalizedSymbol, count)
			for i := range regions {
				regions[i] = compositeEmbeddedRegion{
					kind: "embedded", content: OffsetRange{Start: i * 24, End: i*24 + 12},
				}
				symbols[i].declarationOffsets.Start = i*24 + 4
			}
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				assignEmbeddedRegionIDs(symbols, regions)
			}
			for i, symbol := range symbols {
				if symbol.RegionID != fmt.Sprintf("embedded-%d", i+1) {
					b.Fatalf("symbol %d region %q", i, symbol.RegionID)
				}
			}
		})
	}
}
