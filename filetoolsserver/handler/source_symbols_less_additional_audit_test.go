package handler

import (
	"os"
	"testing"
)

// Real Less stylesheets are inspected without running Dumi, Vue or the browser.
func TestSourceSymbolsOriginalLessAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Less corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "less-antdesign", language: "less",
			search: ".rc-footer", primary: ".dumi/global.less",
			negativeFile: ".dumi/layer-import.less", negativeQuery: ".rc-footer",
			paths: []string{".dumi/global.less", ".dumi/layer-import.less"},
			oracles: []extraAuditEvidence{
				{".dumi/global.less", "@import 'dumi/theme-default/slots/SearchBar/index.less';"},
				{".dumi/global.less", ".demo-logo {"},
				{".dumi/global.less", ".rc-footer a:hover {"},
				{".dumi/global.less", ".rc-footer-light {"},
				{".dumi/global.less", "@media only screen and (max-width: 767.99px) {"},
				{".dumi/layer-import.less", "@layer antd {"},
				{".dumi/layer-import.less", "@import '~../components/style/antd.css';"},
			},
			targets: []extraAuditSymbol{
				{".dumi/global.less", ".demo-logo", 4},
				{".dumi/global.less", ".rc-footer", 29},
				{".dumi/global.less", ".rc-footer-light", 111},
				{".dumi/global.less", ".rc-footer", 127},
			},
		},
		{
			project: "less-antdv", language: "less",
			search: "#header", primary: "site/src/theme/static/header.less",
			negativeFile: "site/src/theme/static/var.less", negativeQuery: "#header",
			paths: []string{
				"site/src/theme/static/header.less",
				"site/src/layouts/header/index.less",
				"site/src/theme/static/var.less",
				"site/src/theme/static/index.less",
				"site/src/index.less",
			},
			oracles: []extraAuditEvidence{
				{"site/src/index.less", "@import './theme/static/index.less';"},
				{"site/src/theme/static/index.less", "@import './var';"},
				{"site/src/theme/static/index.less", "@import './header';"},
				{"site/src/theme/static/header.less", "&.home-header {"},
				{"site/src/layouts/header/index.less", "#header {"},
				{"site/src/layouts/header/index.less", "@import './SearchBox.less';"},
				{"site/src/theme/static/var.less", "--header-height: 64px;"},
				{"site/src/layouts/header/index.vue", "'home-header': isHome.value"},
				{"site/src/hooks/useSiteToken.ts", "--header-height:"},
				{"site/src/layouts/header/Logo.vue", "height: var(--header-height);"},
			},
			targets: []extraAuditSymbol{
				{"site/src/theme/static/header.less", "#header", 1},
				{"site/src/layouts/header/index.less", "#header", 3},
				{"site/src/layouts/header/index.less", "#header", 45},
				{"site/src/theme/static/var.less", ":root", 1},
				{"site/src/layouts/header/index.less", ".popover-menu", 51},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
