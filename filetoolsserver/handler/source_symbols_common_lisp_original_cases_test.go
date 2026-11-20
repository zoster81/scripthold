package handler

// Maintenance questions were derived from original sources before navigation queries.
func originalCommonLispCases() []originalCommonLispCase {
	return []originalCommonLispCase{
		{
			project: "common-lisp-jzon", query: "make-parser", unrelated: "src/ratio-to-double.lisp",
			files: []string{"src/jzon.lisp", "test/jzon-tests.lisp", "src/eisel-lemire.lisp", "src/ratio-to-double.lisp"},
			source: []originalCommonLispText{
				{"src/jzon.lisp", "(defun make-parser (in &key"},
				{"src/jzon.lisp", "(allow-trailing-comma nil)"},
				{"src/jzon.lisp", "(setf %allow-trailing-comma (and allow-trailing-comma t))"},
				{"src/jzon.lisp", "(defun %parse-next ("},
				{"src/jzon.lisp", "Trailing comma when reading array"},
				{"src/jzon.lisp", "Trailing comma when reading object"},
				{"src/jzon.lisp", "(defun parse (in &key"},
				{"test/jzon-tests.lisp", "(test parses-arrays-disallows-trailing-comma"},
				{"test/jzon-tests.lisp", "(test parses-arrays-allows-trailing-comma-when-asked"},
				{"test/jzon-tests.lisp", "(test parses-objects-disallows-trailing-comma"},
				{"test/jzon-tests.lisp", "(test parses-objects-allows-trailing-comma-when-asked"},
			},
			targets: []originalCommonLispTarget{
				{"src/jzon.lisp", "make-parser", 679},
				{"src/jzon.lisp", "%parse-next", 747},
				{"src/jzon.lisp", "parse", 1090},
			},
		},
		{
			project: "common-lisp-hunchentoot", query: "session-too-old-p", unrelated: "acceptor.lisp",
			files: []string{"specials.lisp", "session.lisp", "test/test-handlers.lisp", "acceptor.lisp"},
			source: []originalCommonLispText{
				{"specials.lisp", "(defvar *session-max-time* #.(* 30 60)"},
				{"specials.lisp", "(defvar *session-gc-frequency* 50"},
				{"session.lisp", ":initform *session-max-time*"},
				{"session.lisp", "(defun session-too-old-p (session)"},
				{"session.lisp", "(session-last-click session) (session-max-time session)"},
				{"session.lisp", "(defun session-gc ()"},
				{"session.lisp", "(defun get-stored-session (id)"},
				{"session.lisp", "(defun start-session ()"},
				{"test/test-handlers.lisp", "(defun session-test ()"},
				{"test/test-handlers.lisp", "(session-value 'foo)"},
				{"acceptor.lisp", "acceptor-read-timeout"},
			},
			targets: []originalCommonLispTarget{
				{"specials.lisp", "*session-max-time*", 182},
				{"session.lisp", "session-gc", 158},
				{"session.lisp", "start-session", 247},
				{"session.lisp", "session-too-old-p", 281},
				{"session.lisp", "get-stored-session", 287},
				{"test/test-handlers.lisp", "session-test", 164},
			},
		},
	}
}
