package handler

import (
	"os"
	"testing"
)

// These original projects are never executed. CSS lookup, exact source and
// directly inspected PHP/TSX consumers establish selected edit-site navigation.
func TestSourceSymbolsOriginalCSSAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original CSS corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "classic-asp-zynk-samples", language: "css",
			search: "#title", primary: "PHP/CS-Cart/install/setup.css",
			negativeFile:  "PHP/Oscommerce-Zencart/install/setup.css",
			negativeQuery: "#title",
			paths: []string{
				"PHP/CS-Cart/install/setup.css",
				"PHP/Oscommerce-Zencart/install/setup.css",
			},
			oracles: []extraAuditEvidence{
				{"PHP/CS-Cart/install/setup.css", "#title\r\n{"},
				{"PHP/CS-Cart/install/setup.css", "#pageBody\r\n{"},
				{"PHP/Oscommerce-Zencart/install/setup.css", "#pageBody"},
				{"PHP/CS-Cart/install/index.php", "href='setup.css'"},
				{"PHP/CS-Cart/install/index.php", "<p id='title'>"},
				{"PHP/Oscommerce-Zencart/install/index.php", "href=\"setup.css\""},
				{"PHP/Oscommerce-Zencart/install/index.php", "<p id=\"title\">"},
				{"PHP/Virtuemart/administrator/components/com_vmconnect/vmconnect.css", "div#security {"},
			},
			targets: []extraAuditSymbol{
				{"PHP/CS-Cart/install/setup.css", "#title", 10},
				{"PHP/CS-Cart/install/setup.css", "#header", 19},
				{"PHP/CS-Cart/install/setup.css", "#pageBody", 28},
				{"PHP/Oscommerce-Zencart/install/setup.css", "#header", 2},
				{"PHP/Oscommerce-Zencart/install/setup.css", "#pageBody", 3},
			},
		},
		{
			project: "csharp-humanizer", language: "css",
			search: ".hero", primary: "website/src/pages/index.module.css",
			negativeFile: "website/src/css/custom.css", negativeQuery: ".hero",
			paths: []string{
				"website/src/pages/index.module.css",
				"website/src/css/custom.css",
			},
			oracles: []extraAuditEvidence{
				{"website/src/pages/index.module.css", ".hero,\n.tasks,"},
				{"website/src/pages/index.module.css", ".heroCopy {"},
				{"website/src/pages/index.module.css", ".heroActions,\n.referenceLinks"},
				{"website/src/css/custom.css", "--humanizer-paper: #f4fbff;"},
				{"website/src/css/custom.css", "[data-theme='dark'] {"},
				{"website/src/pages/index.tsx", "import styles from './index.module.css';"},
				{"website/src/pages/index.tsx", "className={styles.hero}"},
				{"website/src/pages/index.tsx", "className={styles.heroActions}"},
				{"website/docusaurus.config.ts", "customCss: './src/css/custom.css'"},
			},
			targets: []extraAuditSymbol{
				{"website/src/pages/index.module.css", ".home", 1},
				{"website/src/pages/index.module.css", ".hero", 5},
				{"website/src/pages/index.module.css", ".hero", 14},
				{"website/src/pages/index.module.css", ".heroCopy", 20},
				{"website/src/pages/index.module.css", ".heroActions", 58},
				{"website/src/css/custom.css", ":root", 1},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
