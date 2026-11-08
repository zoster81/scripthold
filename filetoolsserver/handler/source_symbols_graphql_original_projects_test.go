package handler

import (
	"os"
	"testing"
)

// These scenarios verify source navigation using independently inspected
// GraphQL schemas and adjacent source. Original applications are not executed.
func TestSourceSymbolsOriginalGraphQLInitialQualification(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original GraphQL corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "graphql-gqlgen", language: "graphql",
			search: "createTodo", primary: "_examples/config/schema.graphql",
			negativeFile: "_examples/config/user.graphql", negativeQuery: "createTodo",
			paths: []string{
				"_examples/config/schema.graphql",
				"_examples/config/todo.graphql",
				"_examples/config/user.graphql",
			},
			oracles: []extraAuditEvidence{
				{"_examples/config/schema.graphql", "type Query {"},
				{"_examples/config/schema.graphql", "type Mutation {"},
				{"_examples/config/schema.graphql", "createTodo(input: NewTodo!): Todo!"},
				{"_examples/config/todo.graphql", "type Todo {"},
				{"_examples/config/todo.graphql", "id: ID! @goField(forceResolver: true)"},
				{"_examples/config/todo.graphql", "input NewTodo {"},
				{"_examples/config/todo.graphql", "userId: String!"},
				{"_examples/config/user.graphql", "type User"},
				{"_examples/config/user.graphql", "name: String! @goField(name:\"FullName\")"},
				{"_examples/config/.gqlgen.yml", "schema: \"*.graphql\""},
				{"_examples/config/.gqlgen.yml", "fieldName: Description"},
				{"_examples/config/.gqlgen.yml", "fieldName: UserID"},
				{"_examples/config/schema.resolvers.go", "func (r *mutationResolver) CreateTodo(ctx context.Context, input NewTodo) (*Todo, error) {"},
				{"_examples/config/schema.resolvers.go", "Description: input.Text,"},
				{"_examples/config/schema.resolvers.go", "r.todos = append(r.todos, newTodo)"},
				{"_examples/config/todo.resolvers.go", "func (r *todoResolver) ID(ctx context.Context, obj *Todo) (string, error) {"},
				{"_examples/config/user.resolvers.go", "func (r *roleResolver) Name(ctx context.Context, obj *UserRole) (string, error) {"},
				{"_examples/config/generated.go", "CreateTodo(ctx context.Context, input NewTodo) (*Todo, error)"},
			},
			targets: []extraAuditSymbol{
				{"_examples/config/schema.graphql", "Query", 12},
				{"_examples/config/schema.graphql", "Mutation", 16},
				{"_examples/config/schema.graphql", "createTodo", 17},
				{"_examples/config/todo.graphql", "Todo", 1},
				{"_examples/config/todo.graphql", "NewTodo", 11},
				{"_examples/config/user.graphql", "User", 1},
			},
		},
		{
			project: "graphql-apollographql-spacex", language: "graphql",
			search: "capsulesUpcoming", primary: "schema.graphql",
			negativeFile: "schema.graphql", negativeQuery: "createTodo",
			paths: []string{"schema.graphql"},
			oracles: []extraAuditEvidence{
				{"schema.graphql", "type Capsule {"},
				{"schema.graphql", "input CapsulesFind {"},
				{"schema.graphql", "type Query {"},
				{"schema.graphql", "capsulesUpcoming("},
				{"schema.graphql", "capsulesPast("},
				{"schema.graphql", "find: CapsulesFind"},
				{"schema.graphql", "status: String"},
				{"src/resolvers/capsule.ts", "capsulesUpcoming: async ("},
				{"src/resolvers/capsule.ts", "return [];"},
				{"src/resolvers/capsule.ts", "const capsules = await context.api.getCapsules();"},
				{"src/resolvers/capsule.ts", "return applyLimitOffset({ data: capsules, limit, offset });"},
				{"src/api.ts", "getCapsules() {"},
				{"src/api.ts", "return this.get<Array<Capsule>>(\"capsules\");"},
				{"src/limit-offset-service.ts", "return data.slice(offset, endIndex);"},
				{"src/resolvers/rocket.ts", "rocketsResult: (obj, { limit, offset }, context) => {"},
				{"src/__tests__/repo.test.ts", "capsules {"},
				{"src/__tests__/repo.test.ts", "readFileSync(\"schema.graphql\""},
				{"README.md", "The original project used a MongoDB that was deprecated"},
			},
			targets: []extraAuditSymbol{
				{"schema.graphql", "Capsule", 26},
				{"schema.graphql", "CapsulesFind", 45},
				{"schema.graphql", "Query", 515},
				{"schema.graphql", "capsules", 517},
				{"schema.graphql", "capsulesPast", 524},
				{"schema.graphql", "capsulesUpcoming", 531},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
