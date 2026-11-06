package handler

import (
	"os"
	"testing"
)

// Each task and its source oracles was independently derived from an original
// TypeScript project before exercising the development MCP source handlers.
func TestSourceSymbolsOriginalTypeScriptAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original TypeScript corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "project-typescript-hexojs-hexo", language: "typescript",
			search: "excerptFilter", primary: "lib/plugins/filter/after_post_render/excerpt.ts",
			negativeFile:  "lib/plugins/filter/after_post_render/external_link.ts",
			negativeQuery: "excerptFilter",
			paths: []string{
				"lib/plugins/filter/after_post_render/excerpt.ts",
				"lib/plugins/filter/after_post_render/index.ts",
				"test/scripts/filters/excerpt.ts",
				"lib/plugins/filter/after_post_render/external_link.ts",
			},
			oracles: []extraAuditEvidence{
				{"lib/plugins/filter/after_post_render/excerpt.ts", "const rExcerpt = /<!-- ?more ?-->/i;"},
				{"lib/plugins/filter/after_post_render/excerpt.ts", "function excerptFilter(data: RenderData): void {"},
				{"lib/plugins/filter/after_post_render/excerpt.ts", "if (typeof data.excerpt !== 'undefined') {"},
				{"lib/plugins/filter/after_post_render/excerpt.ts", "data.content = content.replace(rExcerpt, (match, index) => {"},
				{"lib/plugins/filter/after_post_render/excerpt.ts", "data.more = content.substring(index + match.length).trim();"},
				{"lib/plugins/filter/after_post_render/excerpt.ts", "return '<span id=\"more\"></span>';"},
				{"lib/plugins/filter/after_post_render/index.ts", "filter.register('after_post_render', require('./excerpt'));"},
				{"test/scripts/filters/excerpt.ts", "import excerptFilter from '../../../lib/plugins/filter/after_post_render/excerpt';"},
				{"test/scripts/filters/excerpt.ts", "it('multiple <!-- more -->', () => {"},
				{"test/scripts/filters/excerpt.ts", "it('skip processing if post/page.excerpt is present in the front-matter', () => {"},
				{"test/scripts/filters/excerpt.ts", "data.more!.should.eql(["},
				{"lib/plugins/filter/after_post_render/external_link.ts", "function externalLinkFilter(this: Hexo, data: RenderData): void {"},
				{"lib/plugins/filter/after_post_render/external_link.ts", "rTargetAttr.test(str)"},
			},
			targets: []extraAuditSymbol{
				{"lib/plugins/filter/after_post_render/excerpt.ts", "excerptFilter", 5},
				{"lib/plugins/filter/after_post_render/external_link.ts", "externalLinkFilter", 11},
			},
		},
		{
			project:  "qualification/final-candidate-corpus/project-typescript-honojs-hono",
			language: "typescript", search: "cors",
			primary:      "src/middleware/cors/index.ts",
			negativeFile: "src/middleware/csrf/index.ts", negativeQuery: "cors",
			paths: []string{
				"src/middleware/cors/index.ts",
				"src/middleware/cors/index.test.ts",
				"src/middleware/csrf/index.ts",
			},
			oracles: []extraAuditEvidence{
				{"src/middleware/cors/index.ts", "type CORSOptions = {"},
				{"src/middleware/cors/index.ts", "export const cors = (options?: CORSOptions): MiddlewareHandler => {"},
				{"src/middleware/cors/index.ts", "allowMethods: ['GET', 'HEAD', 'PUT', 'POST', 'DELETE', 'PATCH', 'QUERY']"},
				{"src/middleware/cors/index.ts", "const findAllowOrigin = ((optsOrigin) => {"},
				{"src/middleware/cors/index.ts", "c.req.method === 'OPTIONS'"},
				{"src/middleware/cors/index.ts", "c.res.headers.append('Vary', 'Origin')"},
				{"src/middleware/cors/index.ts", "set('Access-Control-Allow-Headers', headersStr)"},
				{"src/middleware/cors/index.ts", "status: 204"},
				{"src/middleware/cors/index.test.ts", "describe('CORS by Middleware', () => {"},
				{"src/middleware/cors/index.test.ts", "it('Preflight default', async () => {"},
				{"src/middleware/cors/index.test.ts", "it('Preflight handles a large Access-Control-Request-Headers value', async () => {"},
				{"src/middleware/cors/index.test.ts", "it('Disallow an unmatched origin', async () => {"},
				{"src/middleware/cors/index.test.ts", "maxAge: 600,"},
				{"src/middleware/csrf/index.ts", "CSRF Protection Middleware for Hono."},
				{"src/middleware/csrf/index.ts", "const isSafeMethodRe = /^(GET|HEAD|OPTIONS)$/"},
			},
			targets: []extraAuditSymbol{
				{"src/middleware/cors/index.ts", "CORSOptions", 9},
				{"src/middleware/cors/index.ts", "cors", 63},
				{"src/middleware/csrf/index.ts", "csrf", 94},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
