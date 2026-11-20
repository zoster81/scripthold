package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Source-first tasks follow the original JSP forms to their independent Java
// controller and keep the unrelated view as a negative navigation oracle.
func TestSourceSymbolsOriginalJSPAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original JSP corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "jsp-colinbut-sales-order-system", language: "jsp",
			search: "firstName", primary: "online-application/src/main/webapp/customers/customers-createCustomer.jsp",
			negativeFile:  "online-application/src/main/webapp/orders/orders-createOrder.jsp",
			negativeQuery: "firstName",
			paths: []string{
				"online-application/src/main/webapp/customers/customers-createCustomer.jsp",
				"online-application/src/main/webapp/customers/customers.jsp",
				"online-application/src/main/webapp/orders/orders-createOrder.jsp",
			},
			oracles: []extraAuditEvidence{
				{"online-application/src/main/webapp/customers/customers-createCustomer.jsp", "modelAttribute=\"createCustomerForm\" action=\"createCustomer\""},
				{"online-application/src/main/webapp/customers/customers-createCustomer.jsp", "id=\"firstName\" path=\"firstName\""},
				{"online-application/src/main/webapp/customers/customers-createCustomer.jsp", "id=\"dob\" path=\"dateOfBirth\""},
				{"online-application/src/main/webapp/customers/customers-createCustomer.jsp", "id=\"customerReference\" path=\"customerPaymentDetail.customerReference\""},
				{"online-application/src/main/webapp/customers/customers.jsp", "id=\"createCustomerButton\""},
				{"online-application/src/main/webapp/customers/customers.jsp", "id=\"customerList\""},
				{"online-application/src/main/webapp/orders/orders-createOrder.jsp", "modelAttribute=\"createOrderForm\""},
				{"online-application/src/main/java/com/mycompany/sos/web/CustomerController.java", "public String showCreateCustomerFormPage(ModelMap modelMap)"},
				{"online-application/src/main/java/com/mycompany/sos/web/CustomerController.java", "modelMap.addAttribute(\"createCustomerForm\", new Customer());"},
				{"online-application/src/main/java/com/mycompany/sos/web/CustomerController.java", "public ModelAndView createCustomer("},
				{"online-application/src/main/java/com/mycompany/sos/web/CustomerController.java", "if(result.hasErrors()) {"},
			},
			targets: []extraAuditSymbol{
				{"online-application/src/main/webapp/customers/customers-createCustomer.jsp", "firstName", 15},
				{"online-application/src/main/webapp/customers/customers-createCustomer.jsp", "dob", 25},
				{"online-application/src/main/webapp/customers/customers-createCustomer.jsp", "customerReference", 74},
				{"online-application/src/main/webapp/customers/customers.jsp", "customerList", 12},
			},
		},
		{
			project: "jsp-futsal", language: "jsp",
			search: "bookingForm", primary: "src/main/webapp/WEB-INF/pages/booking_form.jsp",
			negativeFile:  "src/main/webapp/WEB-INF/pages/court_form.jsp",
			negativeQuery: "bookingForm",
			paths: []string{
				"src/main/webapp/WEB-INF/pages/booking_form.jsp",
				"src/main/webapp/WEB-INF/pages/booking_list.jsp",
				"src/main/webapp/WEB-INF/pages/court_form.jsp",
			},
			oracles: []extraAuditEvidence{
				{"src/main/webapp/WEB-INF/pages/booking_form.jsp", "id=\"bookingForm\""},
				{"src/main/webapp/WEB-INF/pages/booking_form.jsp", "id=\"courtId\" name=\"courtId\""},
				{"src/main/webapp/WEB-INF/pages/booking_form.jsp", "id=\"bookingDate\" name=\"bookingDate\""},
				{"src/main/webapp/WEB-INF/pages/booking_form.jsp", "id=\"pricePreview\""},
				{"src/main/webapp/WEB-INF/pages/booking_form.jsp", "function updatePrice() {"},
				{"src/main/webapp/WEB-INF/pages/booking_form.jsp", "const hours = (eh * 60 + em - sh * 60 - sm) / 60;"},
				{"src/main/webapp/WEB-INF/pages/booking_list.jsp", "booking?action=cancel&id=<%= b.getId() %>"},
				{"src/main/webapp/WEB-INF/pages/court_form.jsp", "id=\"pricePerHour\""},
				{"src/main/java/com/futsal/controllers/BookingServlet.java", "req.getRequestDispatcher(\"/WEB-INF/pages/booking_form.jsp\").forward(req, res);"},
				{"src/main/java/com/futsal/controllers/BookingServlet.java", "Integer.parseInt(req.getParameter(\"courtId\"))"},
				{"src/main/java/com/futsal/controllers/BookingServlet.java", "Date.valueOf(req.getParameter(\"bookingDate\"))"},
				{"src/main/java/com/futsal/controllers/BookingServlet.java", "if (hours <= 0) {"},
				{"src/main/java/com/futsal/controllers/BookingServlet.java", "bookingService.createBooking(userId, courtId, bookDate,"},
			},
			targets: []extraAuditSymbol{
				{"src/main/webapp/WEB-INF/pages/booking_form.jsp", "bookingForm", 14},
				{"src/main/webapp/WEB-INF/pages/booking_form.jsp", "courtId", 23},
				{"src/main/webapp/WEB-INF/pages/booking_form.jsp", "bookingDate", 40},
				{"src/main/webapp/WEB-INF/pages/booking_form.jsp", "pricePreview", 62},
				{"src/main/webapp/WEB-INF/pages/court_form.jsp", "pricePerHour", 51},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}

// The malformed "id" query arguments are real original examples. A complete
// digest must not invent their names as HTML declaration symbols.
func TestSourceSymbolsOriginalJSPURLQueriesAreNotEntities(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original JSP corpus unavailable")
	}
	for _, sc := range []struct {
		project, file, evidence, invalidName string
	}{
		{
			"jsp-final-app", "UserManagement/WebContent/user-list.jsp",
			"edit?id=<c:out", "<c:out",
		},
		{
			"jsp-futsal", "src/main/webapp/WEB-INF/pages/booking_list.jsp",
			"booking?action=cancel&id=<%=", "class=",
		},
	} {
		t.Run(sc.project, func(t *testing.T) {
			root := filepath.Join(corpus, sc.project)
			path := filepath.Join(root, filepath.FromSlash(sc.file))
			requireAdditionalOriginalFile(t, path)
			content, err := os.ReadFile(path)
			if err != nil || !strings.Contains(string(content), sc.evidence) {
				t.Fatalf("original URL-query oracle missing: %s %v", sc.file, err)
			}
			h := NewHandler([]string{root})
			failed, result, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "jsp", Encoding: "utf-8",
				Query: sc.invalidName, Match: "exact", MaxFiles: 1, MaxSymbols: 12,
			})
			if err != nil || failed != nil || len(result.Symbols) != 0 {
				t.Fatalf("JSP URL text emitted as entity %q: err=%v failure=%+v symbols=%+v", sc.invalidName, err, failed, result.Symbols)
			}
		})
	}
}
