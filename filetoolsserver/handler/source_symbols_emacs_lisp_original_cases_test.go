package handler

// Source-first maintenance questions: completion timing and project root discovery.
func originalEmacsLispCases() []originalEmacsLispCase {
	return []originalEmacsLispCase{
		{
			project: "emacs-company", query: "company--idle-delay", unrelated: "company-capf.el",
			files: []string{"company.el", "test/core-tests.el", "company-capf.el", "company-childframe.el"},
			source: []originalEmacsLispText{
				{"company.el", "(defcustom company-idle-delay .2"},
				{"company.el", "(defcustom company-minimum-prefix-length 3"},
				{"company.el", "(defcustom company-tooltip-idle-delay .5"},
				{"company.el", "(defun company--idle-delay ()"},
				{"company.el", "(run-with-timer delay nil"},
				{"company.el", "'company-idle-begin"},
				{"test/core-tests.el", "(ert-deftest company-backspace-into-bad-prefix"},
				{"test/core-tests.el", "(company-idle-delay 'now)"},
			},
			targets: []originalEmacsLispTarget{
				{"company.el", "company-idle-delay", 688},
				{"company.el", "company-minimum-prefix-length", 588},
				{"company.el", "company--idle-delay", 2740},
				{"company.el", "company-idle-begin", 2356},
			},
		},
		{
			project: "emacs-projectile", query: "projectile-project-root-functions", unrelated: "projectile-consult.el",
			files: []string{"projectile.el", "test/projectile-root-test.el", "test/projectile-indexing-test.el", "projectile-consult.el"},
			source: []originalEmacsLispText{
				{"projectile.el", "(defcustom projectile-project-root-functions"},
				{"projectile.el", "projectile-root-local"},
				{"projectile.el", "projectile-root-bottom-up"},
				{"projectile.el", "projectile-root-top-down-recurring"},
				{"projectile.el", "(defun projectile-project-root (&optional dir)"},
				{"projectile.el", "(seq-some"},
				{"projectile.el", "projectile-project-root-cache"},
				{"test/projectile-root-test.el", "(describe"},
				{"test/projectile-root-test.el", "(setq projectile-project-root-functions"},
				{"test/projectile-root-test.el", "projectile-test-should-root-in"},
			},
			targets: []originalEmacsLispTarget{
				{"projectile.el", "projectile-project-root-functions", 614},
				{"projectile.el", "projectile-project-root", 3042},
				{"projectile.el", "projectile-root-local", 2975},
			},
		},
	}
}
