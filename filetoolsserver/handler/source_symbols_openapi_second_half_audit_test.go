package handler

import (
	"os"
	"testing"
)

// Provider 77 checks two further independent full OpenAPI documents, rather
// than the previously qualified Connexion and Swagger Petstore documents.
// Original bytes were read completely and verified against pinned Git blobs.
func TestSourceSymbolsOpenAPISecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("additional original OpenAPI project snapshots not configured")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "openapi77-intra42-audit-selected", language: "openapi",
			search: "getUserById", primary: "openapi.yaml",
			negativeFile: "openapi.yaml", negativeQuery: "createVersion",
			paths: []string{"openapi.yaml"},
			oracles: []extraAuditEvidence{
				{"openapi.yaml", "openapi: 3.0.0"},
				{"openapi.yaml", "/users/{id}:"},
				{"openapi.yaml", "operationId: getUsers"},
				{"openapi.yaml", "operationId: getUserById"},
				{"openapi.yaml", "operationId: getUserCandidatureById"},
				{"openapi.yaml", "UserResponse:"},
				{"openapi.yaml", "$ref: '#/components/schemas/UserResponse'"},
			},
			targets: []extraAuditSymbol{
				{"openapi.yaml", "getUsers", 190},
				{"openapi.yaml", "getUserById", 257},
				{"openapi.yaml", "getUserCandidatureById", 280},
				{"openapi.yaml", "UserResponse", 1310},
			},
		},
		{
			project: "openapi77-modrinth-audit-selected", language: "openapi",
			search: "createVersion", primary: "static/openapi.yaml",
			negativeFile: "static/openapi.yaml", negativeQuery: "getUserCandidatureById",
			paths: []string{"static/openapi.yaml"},
			oracles: []extraAuditEvidence{
				{"static/openapi.yaml", "openapi: '3.0.0'"},
				{"static/openapi.yaml", "CreateVersionBody:"},
				{"static/openapi.yaml", "operationId: createVersion"},
				{"static/openapi.yaml", "operationId: scheduleVersion"},
				{"static/openapi.yaml", "operationId: addFilesToVersion"},
				{"static/openapi.yaml", "$ref: '#/components/schemas/CreateVersionBody'"},
			},
			targets: []extraAuditSymbol{
				{"static/openapi.yaml", "CreateVersionBody", 448},
				{"static/openapi.yaml", "Version", 454},
				{"static/openapi.yaml", "createVersion", 2581},
				{"static/openapi.yaml", "scheduleVersion", 2616},
				{"static/openapi.yaml", "addFilesToVersion", 2671},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
