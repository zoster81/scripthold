package handler

import (
	"os"
	"testing"
)

// Each task starts from separately read original source, not the analyzer's output.
// These projects are inspected but never executed.
func TestSourceSymbolsOriginalLuaAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Lua corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "lua-avante-corp-avante.nvim", language: "lua",
			search: "E.parse_envvar", primary: "lua/avante/providers/init.lua",
			negativeFile: "lua/avante/utils/environment.lua", negativeQuery: "E.parse_envvar",
			paths: []string{
				"lua/avante/providers/init.lua",
				"lua/avante/utils/environment.lua",
			},
			oracles: []extraAuditEvidence{
				{"lua/avante/providers/init.lua", "function E.parse_envvar(Opts)"},
				{"lua/avante/providers/init.lua", "scoped_key_name = \"AVANTE_\" .. Opts.api_key_name"},
				{"lua/avante/providers/init.lua", "local scoped_value = Utils.environment.parse(scoped_key_name, Opts._shellenv)"},
				{"lua/avante/providers/init.lua", "local value = Utils.environment.parse(Opts.api_key_name, Opts._shellenv)"},
				{"lua/avante/providers/init.lua", "function E.require_api_key(provider)"},
				{"lua/avante/providers/init.lua", "if rawget(t[k], \"parse_api_key\") == nil then t[k].parse_api_key = function() return E.parse_envvar(t[k]) end end"},
				{"lua/avante/utils/environment.lua", "function M.parse(key_name, override)"},
				{"lua/avante/utils/environment.lua", "if M.cache[cache_key] ~= nil then return M.cache[cache_key] end"},
				{"lua/avante/utils/environment.lua", "local result = Utils.shell_run(cmd)"},
				{"lua/avante/utils/environment.lua", "value = os.getenv(key_name)"},
				{"lua/avante/utils/environment.lua", "if value ~= nil then M.cache[cache_key] = value end"},
				{"lua/avante/providers/openai.lua", "M.api_key_name = \"OPENAI_API_KEY\""},
				{"lua/avante/providers/openai.lua", "if Providers.env.require_api_key(provider_conf) then"},
			},
			targets: []extraAuditSymbol{
				{"lua/avante/providers/init.lua", "E.parse_envvar", 50},
				{"lua/avante/providers/init.lua", "E.require_api_key", 161},
				{"lua/avante/providers/init.lua", "M.parse_config", 269},
				{"lua/avante/utils/environment.lua", "M.parse", 14},
			},
		},
		{
			project: "lua-idvel-rime-ice", language: "lua",
			search: "filter.func", primary: "lua/cold_word_drop/filter.lua",
			negativeFile: "lua/cold_word_drop/drop_words.lua", negativeQuery: "filter.func",
			paths: []string{
				"lua/cold_word_drop/filter.lua",
				"lua/cold_word_drop/processor.lua",
				"lua/cold_word_drop/drop_words.lua",
			},
			oracles: []extraAuditEvidence{
				{"lua/cold_word_drop/filter.lua", "function filter.init(env)"},
				{"lua/cold_word_drop/filter.lua", "local _sd, drop_words = pcall(require, \"cold_word_drop/drop_words\")"},
				{"lua/cold_word_drop/filter.lua", "env.word_reduce_idx = config:get_int(\"cold_word_reduce/idx\") or 4"},
				{"lua/cold_word_drop/filter.lua", "function filter.func(input, env)"},
				{"lua/cold_word_drop/filter.lua", "table.find_index(drop_words, cand_text)"},
				{"lua/cold_word_drop/processor.lua", "local function get_record_filername(record_type)"},
				{"lua/cold_word_drop/processor.lua", "local function write_word_to_file(env, record_type)"},
				{"lua/cold_word_drop/processor.lua", "function processor.init(env)"},
				{"lua/cold_word_drop/processor.lua", "function processor.func(key, env)"},
				{"lua/cold_word_drop/processor.lua", "local res = append_word_to_droplist(env, ctx_map, action_type)"},
				{"lua/cold_word_drop/processor.lua", "write_word_to_file(env, action_type)"},
				{"lua/cold_word_drop/drop_words.lua", "return drop_words"},
			},
			targets: []extraAuditSymbol{
				{"lua/cold_word_drop/filter.lua", "filter.init", 11},
				{"lua/cold_word_drop/filter.lua", "filter.func", 24},
				{"lua/cold_word_drop/processor.lua", "get_record_filername", 13},
				{"lua/cold_word_drop/processor.lua", "write_word_to_file", 29},
				{"lua/cold_word_drop/processor.lua", "processor.init", 76},
				{"lua/cold_word_drop/processor.lua", "processor.func", 97},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
