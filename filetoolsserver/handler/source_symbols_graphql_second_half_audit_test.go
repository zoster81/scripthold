package handler

import (
	"os"
	"testing"
)

// Provider 57: two new independent original Git repositories, distinct from
// the initial gqlgen and Apollo SpaceX scenarios. These are selected source
// maintenance tasks, not executable GraphQL server or API tests.
func TestSourceSymbolsGraphQLSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("independent original GraphQL projects not configured")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "graphql57-go-audit-selected", language: "graphql",
			search: "seven", primary: "schema-kitchen-sink.graphql",
			negativeFile: "schema-all-descriptions.graphql", negativeQuery: "seven",
			paths: []string{
				"schema-kitchen-sink.graphql",
				"schema-all-descriptions.graphql",
				"kitchen-sink.graphql",
			},
			oracles: []extraAuditEvidence{
				{"schema-kitchen-sink.graphql", "type Foo implements Bar & Baz {"},
				{"schema-kitchen-sink.graphql", "two(argument: InputType!): Type"},
				{"schema-kitchen-sink.graphql", "input InputType {"},
				{"schema-kitchen-sink.graphql", "extend type Foo {"},
				{"schema-kitchen-sink.graphql", "seven(argument: [String]): Type"},
				{"schema-all-descriptions.graphql", "type ObjectSingleLine {"},
				{"kitchen-sink.graphql", "query namedQuery($foo: ComplexFooType, $bar: Bar = DefaultBarValue) {"},
				{"kitchen-sink.graphql", "mutation favPost {"},
			},
			targets: []extraAuditSymbol{
				{"schema-kitchen-sink.graphql", "Foo", 8},
				{"schema-kitchen-sink.graphql", "seven", 58},
				{"schema-kitchen-sink.graphql", "InputType", 48},
				{"schema-kitchen-sink.graphql", "Site", 38},
				{"schema-all-descriptions.graphql", "ObjectSingleLine", 14},
				{"kitchen-sink.graphql", "namedQuery", 3},
				{"kitchen-sink.graphql", "favPost", 24},
			},
		},
		{
			project: "graphql57-hasura-audit-selected", language: "graphql",
			search:        "actionName",
			primary:       "cli/commands/testdata/config-v3-test-project/metadata/actions.graphql",
			negativeFile:  "v3/crates/engine/tests/execute/aggregates/root_field/simple_select/request.gql",
			negativeQuery: "actionName",
			paths: []string{
				"cli/commands/testdata/config-v3-test-project/metadata/actions.graphql",
				"v3/crates/engine/tests/execute/aggregates/root_field/simple_select/request.gql",
			},
			oracles: []extraAuditEvidence{
				{"cli/commands/testdata/config-v3-test-project/metadata/actions.graphql", "type Mutation {"},
				{"cli/commands/testdata/config-v3-test-project/metadata/actions.graphql", "actionName("},
				{"cli/commands/testdata/config-v3-test-project/metadata/actions.graphql", "arg1: SampleInput!"},
				{"cli/commands/testdata/config-v3-test-project/metadata/actions.graphql", "input SampleInput {"},
				{"cli/commands/testdata/config-v3-test-project/metadata/actions.graphql", "username: String!"},
				{"cli/commands/testdata/config-v3-test-project/metadata/actions.graphql", "type SampleOutput {"},
				{"cli/commands/testdata/config-v3-test-project/metadata/actions.yaml", "name: actionName"},
				{"cli/commands/testdata/config-v3-test-project/metadata/actions.yaml", "name: SampleInput"},
				{"cli/commands/testdata/config-v3-test-project/metadata/actions.yaml", "name: SampleOutput"},
				{"v3/crates/engine/tests/execute/aggregates/root_field/simple_select/request.gql", "Invoice_aggregate {"},
			},
			targets: []extraAuditSymbol{
				{"cli/commands/testdata/config-v3-test-project/metadata/actions.graphql", "Mutation", 1},
				{"cli/commands/testdata/config-v3-test-project/metadata/actions.graphql", "actionName", 2},
				{"cli/commands/testdata/config-v3-test-project/metadata/actions.graphql", "SampleInput", 7},
				{"cli/commands/testdata/config-v3-test-project/metadata/actions.graphql", "username", 8},
				{"cli/commands/testdata/config-v3-test-project/metadata/actions.graphql", "SampleOutput", 12},
				{"cli/commands/testdata/config-v3-test-project/metadata/actions.graphql", "accessToken", 13},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
