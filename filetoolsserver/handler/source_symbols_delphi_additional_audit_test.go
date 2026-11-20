package handler

import (
	"os"
	"testing"
)

// Read-only project tasks were derived from the original Delphi sources.
func TestSourceSymbolsOriginalDelphiAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Delphi corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "delphi-wave10c-badunius-myjson", language: "delphi",
			search: "getJSON", primary: "uJSON.pas",
			negativeFile: "uJSON.pas", negativeQuery: "CallQuery",
			paths: []string{"uJSON.pas"},
			oracles: []extraAuditEvidence{
				{"README.md", "JSON parser for delphi 7 and earlier"},
				{"README.md", "when you try to clone children this way"},
				{"README.md", "returns value as an encoded JSON string"},
				{"uJSON.pas", "property Code: string read getCode write setCode;"},
				{"uJSON.pas", "function myJSONItem.getCode: string;"},
				{"uJSON.pas", "if fKey <> ''"},
				{"uJSON.pas", "function myJSONItem.getJSON: string;"},
				{"uJSON.pas", "this is the same code as in getCode"},
				{"uJSON.pas", "procedure myJSONItem.setCode(aCode: string);"},
				{"uJSON.pas", "Self.parse(aCode);"},
				{"uJSON.pas", "function myJSONItem.parse(aCode: string): string;"},
			},
			targets: []extraAuditSymbol{
				{"uJSON.pas", "getCode", 199},
				{"uJSON.pas", "getJSON", 341},
				{"uJSON.pas", "parse", 488},
				{"uJSON.pas", "setCode", 741},
			},
		},
		{
			project: "delphi-wave10c-bogdanpolak-graphql-delphi", language: "delphi",
			search: "CallQuery", primary: "src/GraphQL.Base.pas",
			negativeFile: "demo01-Github/Form.Main.pas", negativeQuery: "CallQuery",
			paths: []string{"src/GraphQL.Base.pas", "demo01-Github/Form.Main.pas"},
			oracles: []extraAuditEvidence{
				{"src/GraphQL.Base.pas", "function CallQuery(const Query: string; Variables: TJSONObject = nil)"},
				{"src/GraphQL.Base.pas", "function TGraphQL.CallQuery(const Query: string;"},
				{"src/GraphQL.Base.pas", "if Variables = nil then"},
				{"src/GraphQL.Base.pas", "Variables.ToString + ' }';"},
				{"src/GraphQL.Base.pas", "HttpClient.ContentType := 'application/json';"},
				{"src/GraphQL.Base.pas", "ST := TStringStream.Create(ARequest);"},
				{"src/GraphQL.Base.pas", "Response := HttpClient.Post(URL, ST, nil, nil)"},
				{"src/GraphQL.Base.pas", "raise Exception.Create('Not implemented yet');"},
				{"src/GraphQL.Base.pas", "procedure TGraphQL.SetAuthorization(const Value: string);"},
				{"demo01-Github/Form.Main.pas", "GraphQL.Base;"},
				{"demo01-Github/Form.Main.pas", "procedure TForm1.DoCallGitHubAPI(const GraphQLQuery: string;"},
				{"demo01-Github/Form.Main.pas", "AResponse := GraphQL.CallQuery(GraphQLQuery)"},
				{"demo01-Github/Form.Main.pas", "jsVariables := TJSONObject.ParseJSONValue(GraphQLVariables) as TJSONObject;"},
				{"demo01-Github/Form.Main.pas", "AResponse := GraphQL.CallQuery(GraphQLQuery, jsVariables);"},
				{"demo01-Github/Form.Main.pas", "procedure TForm1.actQueryRepoListExecute(Sender: TObject);"},
			},
			targets: []extraAuditSymbol{
				{"src/GraphQL.Base.pas", "CallQuery", 36},
				{"src/GraphQL.Base.pas", "CallQuery", 72},
				{"src/GraphQL.Base.pas", "SetAuthorization", 78},
				{"demo01-Github/Form.Main.pas", "DoCallGitHubAPI", 59},
				{"demo01-Github/Form.Main.pas", "actQueryRepoListExecute", 95},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
