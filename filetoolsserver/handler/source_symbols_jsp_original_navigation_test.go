package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Original JSP views and their Java controllers are inspected without execution.
func TestSourceSymbolsOriginalJSPTaskNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("original JSP corpus unavailable")
	}
	for _, tc := range []struct {
		project, primary, neighbor, distractor, controller, entity, method string
		line, javaLine                                                     int
		evidence                                                           []string
	}{
		{"jsp-spring-petclinic-spring-framework-petclinic",
			"src/main/webapp/WEB-INF/jsp/owners/findOwners.jsp",
			"src/main/webapp/WEB-INF/jsp/owners/ownersList.jsp",
			"src/main/webapp/WEB-INF/jsp/vets/vetList.jsp",
			"src/main/java/org/springframework/samples/petclinic/web/OwnerController.java",
			"search-owner-form", "processFindForm", 14, 77,
			[]string{"path=\"lastName\"", "id=\"ownersTable\"", "findOwnerByLastName(owner.getLastName())"}},
		{"jsp-houarizegai-hotelreservationsystem",
			"HotelReservation-war/web/views/reservation.jsp",
			"HotelReservation-war/web/views/available_rooms.jsp",
			"HotelReservation-war/web/views/home.jsp",
			"HotelReservation-war/src/java/servlets/AvailableRooms.java",
			"dayNumbers", "doGet", 41, 15,
			[]string{"id='totalPrice'", "dateOutDate = checkOutDate.split(\"-\")", "checkOutDateDays = Number.parseInt(checkOutDateArray[0])", "getRequestDispatcher(\"/views/reservation.jsp\")"}},
	} {
		t.Run(tc.project, func(t *testing.T) {
			base := filepath.Join(root, tc.project)
			h := NewHandler([]string{base})
			rels := []string{tc.primary, tc.neighbor, tc.distractor, tc.controller}
			paths := []string{}
			all := ""
			for i, rel := range rels {
				p := filepath.Join(base, filepath.FromSlash(rel))
				fi, err := os.Lstat(p)
				if err != nil || !fi.Mode().IsRegular() {
					t.Fatalf("missing original %s: %v", rel, err)
				}
				raw, err := os.ReadFile(p)
				if err != nil {
					t.Fatal(err)
				}
				all += string(raw)
				if i < 3 {
					paths = append(paths, p)
				}
			}
			for _, e := range tc.evidence {
				if !strings.Contains(all, e) {
					t.Fatalf("original evidence changed: %q", e)
				}
			}
			fail, d, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: "jsp", Encoding: "utf-8", Presentation: "compact", MaxFiles: 3,
			})
			if err != nil || fail != nil || d.Truncated || !d.CoverageComplete || d.FilesParsed != 3 {
				t.Fatalf("JSP digest %v %+v", err, fail)
			}
			t.Logf("views=%d symbols=%d", d.FilesParsed, d.SymbolCount)
			p := paths[0]
			fail, f, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{p}, Language: "jsp", Encoding: "utf-8", Query: tc.entity, Match: "exact", MaxFiles: 1, MaxSymbols: 10,
			})
			if err != nil || fail != nil || len(f.Files) != 1 {
				t.Fatalf("find %s: %v", tc.entity, err)
			}
			id := ""
			for _, s := range f.Symbols {
				if s.Name == tc.entity && string(s.Kind) == "entity" && s.NameRange.Start.Line == tc.line {
					id = s.ID
					break
				}
			}
			if id == "" {
				t.Fatalf("missing JSP entity %s:%d: %+v", tc.entity, tc.line, f.Symbols)
			}
			fail, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: p, SymbolID: id, SourceFingerprint: f.Files[0].SourceFingerprint, Language: "jsp", Encoding: "utf-8", MaxBytes: 8192,
			})
			if err != nil || fail != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.entity) {
				t.Fatalf("verified JSP show %s: %v", tc.entity, err)
			}
			fail, q, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Paths: paths, Language: "jsp", Encoding: "utf-8", Query: tc.entity, Match: "exact", Mode: "structural", MaxResults: 10,
			})
			if err != nil || fail != nil || q.Search == nil || len(q.Search.Matches) == 0 {
				t.Fatalf("structural JSP search %s: %v", tc.entity, err)
			}
			fail, other, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{paths[2]}, Language: "jsp", Encoding: "utf-8", Query: tc.entity, Match: "exact", MaxFiles: 1, MaxSymbols: 5,
			})
			if err != nil || fail != nil || len(other.Symbols) != 0 {
				t.Fatalf("false distractor %s: %v", tc.entity, err)
			}
			fail, java, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{filepath.Join(base, filepath.FromSlash(tc.controller))}, Language: "java", Encoding: "utf-8", Query: tc.method, Match: "exact", MaxFiles: 1, MaxSymbols: 10,
			})
			if err != nil || fail != nil {
				t.Fatalf("Java controller %s: %v", tc.method, err)
			}
			found := false
			for _, s := range java.Symbols {
				if s.Name == tc.method && s.NameRange.Start.Line == tc.javaLine {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing Java neighbor %s:%d: %+v", tc.method, tc.javaLine, java.Symbols)
			}
		})
	}
}
