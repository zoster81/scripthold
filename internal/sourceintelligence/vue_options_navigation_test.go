package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
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

// A component may have both a script-setup function and an Options API method
// with the same name. Both authored declarations must remain independently
// addressable in their respective script regions.
func TestVueDualScriptDuplicateMethodNamesRetainRegions(t *testing.T) {
	check := func(t *testing.T, source string, name string) {
		t.Helper()
		result, err := (VueAnalyzer{}).Analyze(context.Background(),
			testSourceDocument("dual-script.vue", source), testAnalyzeOptions(true, 256))
		if err != nil {
			t.Fatal(err)
		}
		found := make([]NormalizedSymbol, 0, 2)
		for _, symbol := range result.Analysis.Symbols {
			if symbol.Name == name {
				found = append(found, symbol)
			}
		}
		if len(found) != 2 {
			t.Fatalf("%q: want two authored declarations in distinct scripts, got %d: %+v", name, len(found), result.Analysis.Symbols)
		}
		if found[0].ID == found[1].ID || found[0].RegionID == found[1].RegionID ||
			found[0].NameRange.Start.Line == found[1].NameRange.Start.Line {
			t.Fatalf("%q declarations lack unique source identity: %+v", name, found)
		}
	}
	const fixture = "<script setup>\nfunction save () { return true }\n</script>\n" +
		"<script>\nexport default { methods: { save () { return false } } }\n</script>\n"
	for _, tc := range []struct{ name, source string }{
		{"LF", fixture}, {"CRLF", strings.ReplaceAll(fixture, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) { check(t, tc.source, "save") })
	}
	if root := os.Getenv("SCRIPTHOLD_REAL_VUE_VUETIFY_OPTIONS_ROOT"); root != "" {
		t.Run("source-original-vuetify-prop-filter", func(t *testing.T) {
			path := filepath.Join(root, "prop-filter.vue")
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("bounded original Vue source unavailable: %v %v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(data)
			if !strings.Contains(source, "function customFilter (itemTitle, queryText, item)") ||
				!strings.Contains(source, "      customFilter (itemTitle, queryText, item) {") {
				t.Fatal("original Vuetify dual-script oracle changed")
			}
			check(t, source, "customFilter")
			check(t, source, "save")
		})
	}
}

// A typed Vue script-setup binding and a nested callback local have distinct
// navigation scopes; template text must not create source declarations.
func TestVueTypedScriptSetupOriginalBindings(t *testing.T) {
	check := func(t *testing.T, source string) {
		t.Helper()
		result, err := (VueAnalyzer{}).Analyze(context.Background(),
			testSourceDocument("ApiTable.vue", source), testAnalyzeOptions(true, 256))
		if err != nil {
			t.Fatal(err)
		}
		byName := make(map[string][]NormalizedSymbol)
		for _, symbol := range result.Analysis.Symbols {
			byName[symbol.Name] = append(byName[symbol.Name], symbol)
		}
		for _, name := range []string{"props", "t", "filtered"} {
			bindings := byName[name]
			if len(bindings) == 0 {
				t.Fatalf("authored Vue script-setup declaration %q missing: %v", name, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		}
		for _, name := range []string{"items", "query", "item"} {
			for _, symbol := range byName[name] {
				if symbol.ParentID == "" {
					t.Fatalf("nested script-setup local %q promoted to top-level: %+v", name, symbol)
				}
			}
		}
	}
	const fixture = "<script setup lang=\"ts\">\n" +
		"import type { PropType } from 'vue'\n" +
		"const props = defineProps({ headers: { type: Array as PropType<string[]> } })\n" +
		"const { t } = useI18n()\n" +
		"const filtered = computed(() => {\n" +
		"  const items = props.headers.filter((item: string) => !!item)\n" +
		"  return items\n" +
		"})\n</script>\n"
	for _, tc := range []struct{ name, source string }{
		{"LF", fixture}, {"CRLF", strings.ReplaceAll(fixture, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) { check(t, tc.source) })
	}
	if root := os.Getenv("SCRIPTHOLD_REAL_VUE_VUETIFY_TS_ROOT"); root != "" {
		t.Run("source-original-vuetify-ApiTable", func(t *testing.T) {
			path := filepath.Join(root, "ApiTable.vue")
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("bounded original Vue TypeScript source unavailable: %v %v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(data)
			if !strings.Contains(source, "const { t } = useI18n()") ||
				!strings.Contains(source, "const filtered = computed(() => {") {
				t.Fatal("original Vuetify script-setup oracle changed")
			}
			check(t, source)
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
