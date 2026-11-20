package handler

import (
	"os"
	"testing"
)

// Independent Vue 2 Options API and Vue 3 script-setup maintenance scenarios.
// Only source navigation is checked; components are not built or rendered.
func TestSourceSymbolsOriginalVueAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Vue corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "vue-bailicangdu-vue2-elm", language: "vue",
			search: ".head_login", primary: "src/components/header/head.vue",
			negativeFile: "src/components/footer/footGuide.vue", negativeQuery: ".head_login",
			paths: []string{
				"src/components/header/head.vue",
				"src/page/home/home.vue",
				"src/components/footer/footGuide.vue",
				"src/App.vue",
			},
			oracles: []extraAuditEvidence{
				{"src/components/header/head.vue", "class=\"head_login\""},
				{"src/components/header/head.vue", ":to=\"userInfo? '/profile':'/login'\""},
				{"src/components/header/head.vue", ".head_login{"},
				{"src/components/header/head.vue", "props: ['signinUp', 'headTitle', 'goBack']"},
				{"src/page/home/home.vue", "import headTop from '../../components/header/head'"},
				{"src/page/home/home.vue", "<head-top signin-up='home'>"},
				{"src/page/home/home.vue", ".head_logo{"},
				{"src/components/footer/footGuide.vue", "id='foot_guide'"},
				{"src/App.vue", "router-view v-if"},
			},
			targets: []extraAuditSymbol{
				{"src/components/header/head.vue", ".head_login", 72},
				{"src/components/header/head.vue", ".head_goback", 66},
				{"src/page/home/home.vue", ".head_logo", 104},
				{"src/App.vue", ".router-fade-enter", 30},
			},
		},
		{
			project: "vue-core", language: "vue",
			search: "copyLink", primary: "packages-private/sfc-playground/src/Header.vue",
			negativeFile:  "packages-private/sfc-playground/src/VersionSelect.vue",
			negativeQuery: "copyLink",
			paths: []string{
				"packages-private/sfc-playground/src/Header.vue",
				"packages-private/sfc-playground/src/VersionSelect.vue",
				"packages-private/sfc-playground/src/App.vue",
				"packages-private/vite-debug/App.vue",
			},
			oracles: []extraAuditEvidence{
				{"packages-private/sfc-playground/src/App.vue", "import Header from './Header.vue'"},
				{"packages-private/sfc-playground/src/Header.vue", "import VersionSelect from './VersionSelect.vue'"},
				{"packages-private/sfc-playground/src/Header.vue", "async function copyLink(e: MouseEvent)"},
				{"packages-private/sfc-playground/src/Header.vue", "navigator.clipboard.writeText(location.href)"},
				{"packages-private/sfc-playground/src/Header.vue", "@click=\"copyLink\""},
				{"packages-private/sfc-playground/src/VersionSelect.vue", "async function fetchVersions(): Promise<string[]>"},
				{"packages-private/sfc-playground/src/VersionSelect.vue", "function copyVersion(v: string)"},
				{"packages-private/vite-debug/App.vue", "const count = ref(0)"},
			},
			targets: []extraAuditSymbol{
				{"packages-private/sfc-playground/src/Header.vue", "copyLink", 47},
				{"packages-private/sfc-playground/src/Header.vue", "setVueVersion", 39},
				{"packages-private/sfc-playground/src/VersionSelect.vue", "fetchVersions", 21},
				{"packages-private/sfc-playground/src/VersionSelect.vue", "copyVersion", 60},
				{"packages-private/sfc-playground/src/VersionSelect.vue", "setVersion", 55},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
