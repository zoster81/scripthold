package handler

import (
	"os"
	"testing"
)

// Additional original Ruby projects: source-first navigation without running third-party code.
func TestSourceSymbolsOriginalRubyAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Ruby corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project:       "qualification/final-candidate-corpus/project-ruby-basecamp-kamal",
			language:      "ruby",
			search:        "custom_ssl_certificate?",
			primary:       "lib/kamal/configuration/proxy.rb",
			negativeFile:  "lib/kamal/configuration/proxy/boot.rb",
			negativeQuery: "custom_ssl_certificate?",
			paths: []string{
				"lib/kamal/configuration/proxy.rb",
				"lib/kamal/configuration/validator/proxy.rb",
				"lib/kamal/configuration/proxy/boot.rb",
				"test/configuration/proxy_test.rb",
			},
			oracles: []extraAuditEvidence{
				{"lib/kamal/configuration/proxy.rb", "proxy_config[\"hosts\"] || proxy_config[\"host\"]&.split(\",\") || []"},
				{"lib/kamal/configuration/proxy.rb", "ssl[\"certificate_pem\"].present? && ssl[\"private_key_pem\"].present?"},
				{"lib/kamal/configuration/proxy.rb", "secrets[ssl[\"certificate_pem\"]]"},
				{"lib/kamal/configuration/validator/proxy.rb", "error \"Must set a host to enable automatic SSL\""},
				{"lib/kamal/configuration/validator/proxy.rb", "error \"Specify one of 'host' or 'hosts', not both\""},
				{"test/configuration/proxy_test.rb", "test \"ssl with multiple hosts passed via hosts\" do"},
				{"test/configuration/proxy_test.rb", "test \"deploy options with custom ssl certificates\" do"},
				{"lib/kamal/configuration/proxy/boot.rb", "def default_boot_options"},
			},
			targets: []extraAuditSymbol{
				{"lib/kamal/configuration/proxy.rb", "hosts", 28},
				{"lib/kamal/configuration/proxy.rb", "custom_ssl_certificate?", 32},
				{"lib/kamal/configuration/proxy.rb", "certificate_pem_content", 38},
				{"lib/kamal/configuration/proxy.rb", "host_tls_cert", 50},
				{"lib/kamal/configuration/validator/proxy.rb", "validate!", 2},
				{"lib/kamal/configuration/proxy/boot.rb", "default_boot_options", 25},
			},
		},
		{
			project:       "qualification/final-candidate-corpus/project-ruby-capistrano-capistrano",
			language:      "ruby",
			search:        "regex_matcher",
			primary:       "lib/capistrano/configuration/host_filter.rb",
			negativeFile:  "spec/lib/capistrano/configuration/role_filter_spec.rb",
			negativeQuery: "regex_matcher",
			paths: []string{
				"lib/capistrano/configuration/host_filter.rb",
				"lib/capistrano/configuration/role_filter.rb",
				"lib/capistrano/configuration/filter.rb",
				"spec/lib/capistrano/configuration/host_filter_spec.rb",
				"spec/lib/capistrano/configuration/role_filter_spec.rb",
			},
			oracles: []extraAuditEvidence{
				{"lib/capistrano/configuration/host_filter.rb", "v.split(\",\")"},
				{"lib/capistrano/configuration/host_filter.rb", "Regexp.union values"},
				{"lib/capistrano/configuration/role_filter.rb", "s.roles.any?"},
				{"lib/capistrano/configuration/filter.rb", "HostFilter.new(values)"},
				{"lib/capistrano/configuration/filter.rb", "RoleFilter.new(values)"},
				{"lib/capistrano/configuration.rb", "@filters << Filter.new(:host, ENV[\"HOSTS\"]) if ENV[\"HOSTS\"]"},
				{"spec/lib/capistrano/configuration/host_filter_spec.rb", "context \"with a comma separated string\" do"},
				{"spec/lib/capistrano/configuration/role_filter_spec.rb", "context \"with a regex string\" do"},
			},
			targets: []extraAuditSymbol{
				{"lib/capistrano/configuration/host_filter.rb", "HostFilter", 3},
				{"lib/capistrano/configuration/host_filter.rb", "filter", 10},
				{"lib/capistrano/configuration/host_filter.rb", "regex_matcher", 16},
				{"lib/capistrano/configuration/role_filter.rb", "RoleFilter", 3},
				{"lib/capistrano/configuration/filter.rb", "Filter", 9},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
