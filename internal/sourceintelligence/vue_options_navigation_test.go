package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

func TestVueOptionsAPIMethodNavigation(t *testing.T) {
	cases := []struct {
		name, text      string
		want, forbidden []string
	}{
		{
			name: "options-methods",
			text: `<template><button @click="handleSizeChange">Save</button></template>
<script>
export default {
  name: 'Pagination',
  props: { limit: { type: Number, default: 20 } },
  methods: {
    handleSizeChange(val) { return val },
    handleCurrentChange(val) { return val + 1 }
  }
}
</script>`,
			want:      []string{"handleSizeChange", "handleCurrentChange"},
			forbidden: []string{"default", "limit"},
		},
		{
			name: "crlf-options-methods",
			text: strings.ReplaceAll(`<script>
export default {
  methods: {
    updatePage(value) { return value }
  }
}
</script>`, "\n", "\r\n"),
			want: []string{"updatePage"},
		},
		{
			name: "nested-and-fake-methods-excluded",
			text: `<script>
const standalone = { methods: { unrelated() {} } };
const prose = "export default { methods: { fake() {} } }";
export default {
  data() { return { methods: { nested() {} } } },
  methods: { real() { return 1 } }
}
</script>`,
			want:      []string{"real"},
			forbidden: []string{"unrelated", "fake", "nested"},
		},
		{
			name: "async-and-unicode-methods-with-nested-decoy",
			text: `<script>
export default {
  props: { config: { methods: { decoy() {} } } },
  methods: {
    async café(value) { return /ab+c/.test(value) },
    regular() { return 42 },
    other: { nested() {} }
  }
}
</script>`,
			want:      []string{"café", "regular"},
			forbidden: []string{"decoy", "nested"},
		},
		{
			name: "script-setup-has-no-options-methods",
			text: `<script setup lang="ts">
function handleChange(val: number) { return val }
const text = "methods: { fake() {} }"
</script>`,
			want:      []string{"handleChange"},
			forbidden: []string{"fake"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (VueAnalyzer{}).Analyze(context.Background(), testSourceDocument("fixture.vue", tc.text), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, name := range tc.want {
				got, ok := byName[name]
				if !ok {
					t.Fatalf("missing %q: %v", name, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
				if (tc.name == "options-methods" || tc.name == "crlf-options-methods" || tc.name == "async-and-unicode-methods-with-nested-decoy") && (got.Kind != SymbolKindMethod || got.BodyRange == nil || got.RegionID == "") {
					t.Fatalf("method %q missing kind/body/region: %+v", name, got)
				}
			}
			for _, name := range tc.forbidden {
				if got, ok := byName[name]; ok {
					t.Fatalf("phantom symbol %q: %+v", name, got)
				}
			}
		})
	}
}

func TestVueOptionsComputedGetterBoundaries(t *testing.T) {
	source := strings.Join([]string{
		"<script>",
		"const decoy = { computed: { notExported() { return 0 } } }",
		"const prose = \"export default { computed: { fake() {} } }\"",
		"export default {",
		"  props: { config: { computed: { nestedProp() {} } } },",
		"  data() { return { computed: { nestedData() {} } } },",
		"  computed: {",
		"    displayName() { return this.user.name },",
		"    nestedObject: { get() { return 'not-indexed' }, set(value) { this.name = value } },",
		"    functionValue: function() { return 1 },",
		"  },",
		"  methods: {",
		"    save() { return this.displayName },",
		"    complex: { nestedMethod() {} },",
		"  },",
		"}",
		"</script>",
	}, "\n")
	for _, tc := range []struct {
		name, text string
	}{
		{"lf", source},
		{"crlf", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (VueAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("options.vue", tc.text), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			symbols := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, expected := range []struct {
				name, native string
				kind         SymbolKind
			}{
				{"displayName", "vue-options-computed", SymbolKindProperty},
				{"save", "vue-options-method", SymbolKindMethod},
			} {
				got, ok := symbols[expected.name]
				if !ok || got.Kind != expected.kind || got.NativeKind != expected.native ||
					got.BodyRange == nil || got.RegionID == "" {
					t.Fatalf("expected supported Vue Options declaration %q: %+v", expected.name, got)
				}
			}
			for _, forbidden := range []string{
				"notExported", "fake", "nestedProp", "nestedData",
				"nestedObject", "get", "set", "functionValue", "complex", "nestedMethod",
			} {
				if got, ok := symbols[forbidden]; ok {
					t.Fatalf("nested/unsupported Vue Options declaration invented: %q %+v", forbidden, got)
				}
			}
		})
	}
}
