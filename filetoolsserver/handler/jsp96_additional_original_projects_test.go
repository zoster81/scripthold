package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type jsp96Target struct {
	file, language, name, body string
	line                       int
}

// These are new independent upstream applications, not the earlier JSP
// PetClinic, hotel, sales-order or futsal project origins. Source oracles were
// derived by fully reading and byte-pinning original Git objects first.
func TestJSP96AdditionalOriginalProjectNavigation(t *testing.T) {
	in28 := os.Getenv("SCRIPTHOLD_REAL_JSP96_IN28_ROOT")
	webchat := os.Getenv("SCRIPTHOLD_REAL_JSP96_WEBCHAT_ROOT")
	if in28 == "" && webchat == "" {
		t.Skip("original JSP projects are not configured")
	}
	if in28 == "" || webchat == "" {
		t.Fatal("both independent original JSP projects must be configured together")
	}
	for _, tc := range []struct {
		name, root, unrelated, negative string
		files                           []string
		edges                           []struct{ file, text string }
		targets                         []jsp96Target
	}{
		{
			name:  "Todo POST servlet and separate list rendering",
			root:  in28,
			files: []string{"add-todo.jsp", "list-todos.jsp", "AddTodoServlet.java", "ListTodoServlet.java"},
			edges: []struct{ file, text string }{
				{"add-todo.jsp", "action=\"/add-todo.do\""},
				{"add-todo.jsp", "name=\"todo\""},
				{"add-todo.jsp", "name=\"category\""},
				{"AddTodoServlet.java", "request.getParameter(\"todo\")"},
				{"AddTodoServlet.java", "request.getParameter(\"category\")"},
				{"AddTodoServlet.java", "response.sendRedirect(\"/list-todos.do\")"},
				{"ListTodoServlet.java", "request.getRequestDispatcher(\"/WEB-INF/views/list-todos.jsp\")"},
				{"list-todos.jsp", "<c:forEach items="},
			},
			targets: []jsp96Target{
				{"AddTodoServlet.java", "java", "doPost", "response.sendRedirect(\"/list-todos.do\")", 31},
				{"AddTodoServlet.java", "java", "doGet", "request.getRequestDispatcher(\"/WEB-INF/views/add-todo.jsp\")", 25},
				{"ListTodoServlet.java", "java", "doGet", "request.setAttribute(\"todos\", todoService.retrieveTodos())", 16},
			},
			unrelated: "list-todos.jsp", negative: "doPost",
		},
		{
			name:  "Chat JSP form identifiers to distinct JavaScript send and refresh",
			root:  webchat,
			files: []string{"chat.jsp", "user.jsp", "chat.js"},
			edges: []struct{ file, text string }{
				{"chat.jsp", "id=\"message_box\""},
				{"chat.jsp", "id=\"outgoing_id\""},
				{"chat.jsp", "id=\"incoming_id\""},
				{"chat.jsp", "onclick=\"submitForm()\""},
				{"chat.jsp", "src=\"js/chat.js\""},
				{"chat.js", "document.getElementById(\"message_box\")"},
				{"chat.js", "insertChat?outgoing_id="},
				{"chat.js", "get_chat?outgoing_id="},
				{"user.jsp", "id=\"user_list\""},
			},
			targets: []jsp96Target{
				{"chat.jsp", "jsp", "message_box", "id=\"message_box\"", 74},
				{"chat.jsp", "jsp", "outgoing_id", "id=\"outgoing_id\"", 75},
				{"chat.jsp", "jsp", "incoming_id", "id=\"incoming_id\"", 76},
				{"chat.js", "javascript", "submitForm", "insertChat?outgoing_id=", 29},
				{"chat.js", "javascript", "sendGetRequest", "get_chat?outgoing_id=", 64},
			},
			unrelated: "user.jsp", negative: "message_box",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, h := context.Background(), NewHandler([]string{tc.root})
			sources := make(map[string]string, len(tc.files))
			for _, name := range tc.files {
				path := filepath.Join(tc.root, name)
				st, err := os.Lstat(path)
				if err != nil || !st.Mode().IsRegular() || st.Size() > 1<<20 {
					t.Fatalf("missing or oversized original source %s: stat=%v err=%v", name, st, err)
				}
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				sources[name] = string(raw)
				lang := "jsp"
				if strings.HasSuffix(name, ".java") {
					lang = "java"
				}
				if strings.HasSuffix(name, ".js") {
					lang = "javascript"
				}
				_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "digest", Presentation: "compact", Paths: []string{path}, Language: lang, MaxFiles: 1,
				})
				if err != nil || digest.Truncated || digest.FilesParsed != 1 || len(digest.OverviewFiles) != 1 {
					t.Fatalf("source overview failure %s %s: err=%v digest=%+v", name, lang, err, digest)
				}
			}
			for _, edge := range tc.edges {
				if !strings.Contains(sources[edge.file], edge.text) {
					t.Fatalf("cross-file original evidence missing %s %q", edge.file, edge.text)
				}
			}
			for _, tg := range tc.targets {
				lines := strings.Split(sources[tg.file], "\n")
				if tg.line < 1 || tg.line > len(lines) || !strings.Contains(lines[tg.line-1], tg.name) ||
					!strings.Contains(sources[tg.file], tg.body) {
					t.Fatalf("independent source oracle changed %s:%d %s", tg.file, tg.line, tg.name)
				}
				path := filepath.Join(tc.root, tg.file)
				_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Language: tg.language,
					Query: tg.name, Match: "exact", MaxFiles: 1, MaxSymbols: 64, FileDetails: "relevant",
				})
				if err != nil || found.Truncated || len(found.Files) != 1 {
					t.Fatalf("find failed %s:%s err=%v found=%+v", tg.file, tg.name, err, found)
				}
				var id string
				for _, sym := range found.Symbols {
					if sym.Path == path && sym.Name == tg.name && sym.NameRange.Start.Line == tg.line {
						id = sym.ID
					}
				}
				if id == "" {
					t.Fatalf("original declaration missing %s:%d %s result=%+v", tg.file, tg.line, tg.name, found.Symbols)
				}
				f := found.Files[0]
				if f.SourceFingerprint == "" || f.Encoding == "" {
					t.Fatalf("missing safe source identity %+v", f)
				}
				_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "show", Path: path, Language: tg.language, SymbolID: id,
					SourceFingerprint: f.SourceFingerprint, Encoding: f.Encoding, MaxBytes: 8192,
				})
				if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tg.body) {
					t.Fatalf("fingerprint-bound source excerpt missing %s:%s err=%v show=%+v", tg.file, tg.name, err, shown.Show)
				}
				_, q, err := h.SourceQuery(ctx, nil, SourceQueryInput{
					Operation: "search", Mode: "structural", Paths: []string{path}, Language: tg.language,
					Query: tg.name, Match: "exact", MaxFiles: 1, MaxResults: 64,
				})
				linked := false
				if q.Search != nil {
					for _, m := range q.Search.Matches {
						if m.SymbolID == id && m.SourceFingerprint == f.SourceFingerprint &&
							m.Range.Start.Line == tg.line {
							linked = true
						}
					}
				}
				if err != nil || q.Search == nil || q.Coverage.ResultsTruncated || !linked {
					t.Fatalf("structural search missing %s:%d %s err=%v query=%+v", tg.file, tg.line, tg.name, err, q)
				}
			}
			_, other, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{filepath.Join(tc.root, tc.unrelated)},
				Language: "jsp", Query: tc.negative, Match: "exact", MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || other.Truncated || len(other.Symbols) != 0 {
				t.Fatalf("unrelated JSP falsely declares %s err=%v symbols=%+v", tc.negative, err, other.Symbols)
			}
			t.Logf("original JSP task PASS: %d files, %d declaration targets", len(tc.files), len(tc.targets))
		})
	}
}
