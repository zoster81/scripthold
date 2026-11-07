package handler

import (
	"os"
	"testing"
)

// Independent original Emacs Lisp projects provide source-first maintenance oracles.
// The original projects are read but never executed.
func TestSourceSymbolsOriginalEmacsLispAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Emacs Lisp corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project:       "emacs-lisp-bbatsov-prelude",
			language:      "emacs-lisp",
			search:        "prelude-whitespace-cleanup-style",
			primary:       "core/prelude-custom.el",
			negativeFile:  "core/prelude-global-keybindings.el",
			negativeQuery: "prelude-whitespace-cleanup-style",
			paths: []string{
				"core/prelude-custom.el",
				"core/prelude-editor.el",
				"core/prelude-global-keybindings.el",
				"test/prelude-load-test.el",
			},
			oracles: []extraAuditEvidence{
				{"core/prelude-custom.el", "(defcustom prelude-clean-whitespace-on-save t"},
				{"core/prelude-custom.el", "(defcustom prelude-whitespace-cleanup-style 'whitespace-cleanup"},
				{"core/prelude-custom.el", "leaving the rest of the file untouched (installed on demand)."},
				{"core/prelude-editor.el", "(defun prelude-cleanup-maybe ()"},
				{"core/prelude-editor.el", "(eq prelude-whitespace-cleanup-style 'ws-butler)"},
				{"core/prelude-editor.el", "(add-hook 'before-save-hook 'prelude-cleanup-maybe nil t)"},
				{"test/prelude-load-test.el", "(unless (featurep 'prelude-editor)"},
				{"init.el", "(require 'prelude-custom)  ;; Needs to be loaded before core, editor and ui"},
				{"init.el", "(require 'prelude-editor)"},
				{"core/prelude-global-keybindings.el", "(global-set-key (kbd"},
			},
			targets: []extraAuditSymbol{
				{"core/prelude-custom.el", "prelude-whitespace", 64},
				{"core/prelude-custom.el", "prelude-clean-whitespace-on-save", 74},
				{"core/prelude-custom.el", "prelude-whitespace-cleanup-style", 80},
				{"core/prelude-editor.el", "prelude-cleanup-maybe", 226},
				{"core/prelude-editor.el", "prelude-enable-whitespace", 241},
				{"test/prelude-load-test.el", "prelude-load-test-modules", 17},
			},
		},
		{
			project:       "emacs-lisp-emacs-helm-helm",
			language:      "emacs-lisp",
			search:        "helm-completion-in-region--insert-result",
			primary:       "helm-mode.el",
			negativeFile:  "helm-adaptive.el",
			negativeQuery: "helm-completion-in-region--insert-result",
			paths: []string{
				"helm-mode.el",
				"test/helm-mode-tests.el",
				"helm-adaptive.el",
			},
			oracles: []extraAuditEvidence{
				{"helm-mode.el", "(defvar helm-crm-default-separator \",\""},
				{"helm-mode.el", "(defun helm-completion-in-region--insert-result (result start point end base-size)"},
				{"helm-mode.el", "(get-text-property 0 'separator crm-separator)"},
				{"helm-mode.el", "(defun helm--crm-insert-fn (_start _end choice)"},
				{"test/helm-mode-tests.el", "(ert-deftest helm-mode-test-crm-regexp-separator-is-not-inserted ()"},
				{"test/helm-mode-tests.el", "(ert-deftest helm-mode-test-crm-text-property-separator-is-inserted ()"},
				{"test/helm-mode-tests.el", "(ert-deftest helm-mode-test-crm-regexp-derived-separator-is-inserted ()"},
				{"test/helm-mode-tests.el", "(should (equal (buffer-string) \"some-string...\"))"},
				{"helm-adaptive.el", "(defcustom helm-adaptive-history-length 50"},
			},
			targets: []extraAuditSymbol{
				{"helm-mode.el", "helm-crm-default-separator", 3096},
				{"helm-mode.el", "helm-completion-in-region--insert-result", 3106},
				{"helm-mode.el", "helm--crm-insert-fn", 3160},
				{"test/helm-mode-tests.el", "helm-mode-test-crm-regexp-separator-is-not-inserted", 28},
				{"test/helm-mode-tests.el", "helm-mode-test-crm-text-property-separator-is-inserted", 38},
				{"test/helm-mode-tests.el", "helm-mode-test-crm-regexp-derived-separator-is-inserted", 48},
				{"helm-adaptive.el", "helm-adaptive-history-length", 39},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
