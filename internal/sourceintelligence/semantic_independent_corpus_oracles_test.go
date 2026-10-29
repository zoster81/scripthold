package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These optional independent checks use paths supplied by the operator.
// Corpus data and workstation locations are intentionally not checked in.
func TestSemanticIndependentRealCorpusOracles(t *testing.T) {
	type want struct {
		name string
		kind SymbolKind
		line int
	}
	cases := []struct {
		name, env, language string
		present             []want
		forbidden           []string
	}{
		{"go-chi-context", "SCRIPTHOLD_REAL_GO_CHI", "go", []want{
			{"chi", SymbolKindPackage, 1}, {"chi.URLParam", SymbolKindFunction, 10},
			{"chi.RouteContext", SymbolKindFunction, 27}, {"chi.Context", SymbolKindStruct, 45},
			{"chi.Context.Reset", SymbolKindMethod, 82}, {"chi.Context.URLParam", SymbolKindMethod, 100},
			{"chi.RouteParams", SymbolKindStruct, 147}, {"chi.RouteParams.Add", SymbolKindMethod, 152},
		}, []string{"chi.Ghost", "chi.Context.Ghost", "chi.context.Value", "chi.String"}},
		{"go-cobra-args", "SCRIPTHOLD_REAL_GO_COBRA", "go", []want{
			{"cobra", SymbolKindPackage, 15}, {"cobra.PositionalArgs", SymbolKindType, 22},
			{"cobra.legacyArgs", SymbolKindFunction, 28}, {"cobra.NoArgs", SymbolKindFunction, 42},
			{"cobra.OnlyValidArgs", SymbolKindFunction, 51}, {"cobra.MatchAll", SymbolKindFunction, 127},
			{"cobra.ExactValidArgs", SymbolKindFunction, 142},
		}, []string{"cobra.Ghost", "cobra.CommandPath", "cobra.fmt.Errorf", "cobra.ValidArgs"}},
		{"go-bubbletea-commands", "SCRIPTHOLD_REAL_GO_BUBBLETEA", "go", []want{
			{"tea", SymbolKindPackage, 1}, {"tea.Batch", SymbolKindFunction, 15},
			{"tea.BatchMsg", SymbolKindType, 21}, {"tea.Sequence", SymbolKindFunction, 25},
			{"tea.compactCmds", SymbolKindFunction, 36}, {"tea.Every", SymbolKindFunction, 102},
			{"tea.Tick", SymbolKindFunction, 154}, {"tea.RequestWindowSize", SymbolKindFunction, 173},
		}, []string{"tea.Ghost", "tea.time.Now", "tea.TickMsg", "tea.T"}},
		{"python-fastapi-response-models", "SCRIPTHOLD_REAL_PY_FASTAPI", "python", []want{
			{"Item", SymbolKindClass, 6}, {"Message", SymbolKindClass, 11},
			{"read_item", SymbolKindFunction, 18},
		}, []string{"JSONResponse", "FastAPI", "BaseModel", "Ghost"}},
		{"python-flask-auth", "SCRIPTHOLD_REAL_PY_FLASK", "python", []want{
			{"login_required", SymbolKindFunction, 19},
			{"login_required.wrapped_view", SymbolKindFunction, 22},
			{"load_logged_in_user", SymbolKindFunction, 32},
			{"register", SymbolKindFunction, 46}, {"login", SymbolKindFunction, 84},
			{"logout", SymbolKindFunction, 112},
		}, []string{"url_for", "generate_password_hash", "get_db", "Ghost"}},
		{"python-httpx-auth", "SCRIPTHOLD_REAL_PY_HTTPX", "python", []want{
			{"Auth", SymbolKindClass, 22}, {"Auth.auth_flow", SymbolKindMethod, 38},
			{"Auth.sync_auth_flow", SymbolKindMethod, 62}, {"Auth.async_auth_flow", SymbolKindMethod, 87},
			{"FunctionAuth", SymbolKindClass, 113}, {"BasicAuth", SymbolKindClass, 126},
			{"NetRCAuth", SymbolKindClass, 145}, {"DigestAuth", SymbolKindClass, 175},
			{"DigestAuth._parse_challenge", SymbolKindMethod, 224},
		}, []string{"typing", "ProtocolError", "Auth.Ghost", "DigestAuth.Ghost"}},
		{"csharp-mediatr-pinghandler", "SCRIPTHOLD_REAL_CS_MEDIATR", "csharp", []want{
			{"MediatR.Examples", SymbolKindNamespace, 4},
			{"MediatR.Examples.PingHandler", SymbolKindClass, 8},
			{"MediatR.Examples.PingHandler.Handle", SymbolKindMethod, 17},
		}, []string{"MediatR.Examples.IRequestHandler", "MediatR.Examples.PingHandler.WriteLineAsync", "MediatR.Examples.Ghost"}},
		{"csharp-polly-retry", "SCRIPTHOLD_REAL_CS_POLLY", "csharp", []want{
			{"Polly.Benchmarks", SymbolKindNamespace, 1},
			{"Polly.Benchmarks.Retry", SymbolKindClass, 3},
			{"Polly.Benchmarks.Retry.Retry_Synchronous_Succeeds", SymbolKindMethod, 9},
			{"Polly.Benchmarks.Retry.Retry_Asynchronous_Succeeds", SymbolKindMethod, 13},
			{"Polly.Benchmarks.Retry.Retry_Synchronous_Throws_Then_Succeeds", SymbolKindMethod, 33},
		}, []string{"Polly.Benchmarks.Workloads.Action", "Polly.Benchmarks.Retry.Ghost", "Polly.Benchmarks.Policy"}},
		{"csharp-dapper-belgrade-benchmark", "SCRIPTHOLD_REAL_CS_DAPPER", "csharp", []want{
			{"Dapper.Tests.Performance", SymbolKindNamespace, 7},
			{"Dapper.Tests.Performance.BelgradeBenchmarks", SymbolKindClass, 9},
			{"Dapper.Tests.Performance.BelgradeBenchmarks.Setup", SymbolKindMethod, 14},
			{"Dapper.Tests.Performance.BelgradeBenchmarks.FirstOrDefault", SymbolKindMethod, 21},
		}, []string{"Dapper.Tests.Performance.QueryMapper", "Dapper.Tests.Performance.BelgradeBenchmarks.Step", "Dapper.Tests.Performance.Ghost"}},
		{"go-fsnotify-shared", "SCRIPTHOLD_REAL_GO_FSNOTIFY", "go", []want{
			{"fsnotify", SymbolKindPackage, 1}, {"fsnotify.shared", SymbolKindStruct, 5},
			{"fsnotify.newShared", SymbolKindFunction, 12}, {"fsnotify.shared.sendEvent", SymbolKindMethod, 21},
			{"fsnotify.shared.sendError", SymbolKindMethod, 34},
			{"fsnotify.shared.isClosed", SymbolKindMethod, 46}, {"fsnotify.shared.close", SymbolKindMethod, 56},
		}, []string{"fsnotify.Ghost", "fsnotify.sync.Mutex", "fsnotify.shared.Lock", "fsnotify.shared.Unlock"}},
		{"go-validator-options", "SCRIPTHOLD_REAL_GO_VALIDATOR", "go", []want{
			{"validator", SymbolKindPackage, 1}, {"validator.Option", SymbolKindType, 4},
			{"validator.WithRequiredStructEnabled", SymbolKindFunction, 12},
			{"validator.WithPrivateFieldValidation", SymbolKindFunction, 22},
			{"validator.WithTagNameFuncBlankOmit", SymbolKindFunction, 36},
		}, []string{"validator.Ghost", "validator.Validate", "validator.requiredStructEnabled"}},
		{"python-attrs-filters", "SCRIPTHOLD_REAL_PY_ATTRS", "python", []want{
			{"_split_what", SymbolKindFunction, 10},
			{"include", SymbolKindFunction, 21}, {"include.include_", SymbolKindFunction, 38},
			{"exclude", SymbolKindFunction, 48}, {"exclude.exclude_", SymbolKindFunction, 65},
		}, []string{"frozenset", "Attribute", "include.Ghost", "exclude.Ghost"}},
		{"python-click-colors", "SCRIPTHOLD_REAL_PY_CLICK", "python", []want{
			{"cli", SymbolKindFunction, 24},
		}, []string{"click", "all_colors", "click.echo", "Ghost"}},
		{"csharp-newtonsoft-name-table", "SCRIPTHOLD_REAL_CS_NEWTONSOFT", "csharp", []want{
			{"Newtonsoft.Json", SymbolKindNamespace, 29},
			{"Newtonsoft.Json.DefaultJsonNameTable", SymbolKindClass, 34},
			{"Newtonsoft.Json.DefaultJsonNameTable.Get", SymbolKindMethod, 63},
			{"Newtonsoft.Json.DefaultJsonNameTable.Add", SymbolKindMethod, 107},
			{"Newtonsoft.Json.DefaultJsonNameTable.AddEntry", SymbolKindMethod, 139},
			{"Newtonsoft.Json.DefaultJsonNameTable.Grow", SymbolKindMethod, 151},
			{"Newtonsoft.Json.DefaultJsonNameTable.Entry", SymbolKindClass, 194},
		}, []string{"Newtonsoft.Json.DefaultJsonNameTable.Ghost", "Newtonsoft.Json.Environment.TickCount", "Newtonsoft.Json.Entry"}},
		{"csharp-serilog-logger-extensions", "SCRIPTHOLD_REAL_CS_SERILOG", "csharp", []want{
			{"Serilog", SymbolKindNamespace, 1}, {"Serilog.LoggerExtensions", SymbolKindClass, 6},
			{"Serilog.LoggerExtensions.ForContext", SymbolKindMethod, 20},
		}, []string{"Serilog.Guard.AgainstNull", "Serilog.LoggerExtensions.Ghost", "Serilog.LogEventLevel"}},
		{"go-gorilla-mux-routes", "SCRIPTHOLD_REAL_GO_GORILLA", "go", []want{
			{"mux_test", SymbolKindPackage, 1},
			{"mux_test.ExampleRoute_HeadersRegexp", SymbolKindFunction, 13},
			{"mux_test.ExampleRoute_HeadersRegexp_exactMatch", SymbolKindFunction, 35},
		}, []string{"mux_test.NewRouter", "mux_test.RouteMatch", "mux_test.Header.Set", "mux_test.Ghost"}},
		{"go-testify-assertion-order", "SCRIPTHOLD_REAL_GO_TESTIFY", "go", []want{
			{"assert", SymbolKindPackage, 1}, {"assert.isOrdered", SymbolKindFunction, 9},
			{"assert.IsIncreasing", SymbolKindFunction, 52},
			{"assert.IsNonIncreasing", SymbolKindFunction, 64},
			{"assert.IsDecreasing", SymbolKindFunction, 76},
			{"assert.IsNonDecreasing", SymbolKindFunction, 88},
		}, []string{"assert.reflect.TypeOf", "assert.Fail", "assert.containsValue", "assert.Ghost"}},
		{"go-participle-api-interfaces", "SCRIPTHOLD_REAL_GO_PARTICIPLE", "go", []want{
			{"participle", SymbolKindPackage, 1}, {"participle.Capture", SymbolKindInterface, 8},
			{"participle.Capture.Capture", SymbolKindMethod, 9},
			{"participle.Parseable", SymbolKindInterface, 13},
			{"participle.Parseable.Parse", SymbolKindMethod, 18},
		}, []string{"participle.lexer.PeekingLexer", "participle.NextMatch", "participle.Ghost"}},
		{"python-requests-exceptions", "SCRIPTHOLD_REAL_PY_REQUESTS", "python", []want{
			{"RequestException", SymbolKindClass, 20},
			{"RequestException.__init__", SymbolKindMethod, 28},
			{"InvalidJSONError", SymbolKindClass, 38},
			{"JSONDecodeError", SymbolKindClass, 42},
			{"JSONDecodeError.__init__", SymbolKindMethod, 45},
			{"JSONDecodeError.__reduce__", SymbolKindMethod, 55},
			{"ConnectTimeout", SymbolKindClass, 91},
			{"RequestsDependencyWarning", SymbolKindClass, 161},
		}, []string{"BaseHTTPError", "PreparedRequest", "CompatJSONDecodeError", "kwargs.pop", "Ghost"}},
		{"python-pytest-multipython", "SCRIPTHOLD_REAL_PY_PYTEST", "python", []want{
			{"python1", SymbolKindFunction, 16}, {"python2", SymbolKindFunction, 22},
			{"Python", SymbolKindClass, 27}, {"Python.__init__", SymbolKindMethod, 28},
			{"Python.dumps", SymbolKindMethod, 34},
			{"Python.load_and_is_true", SymbolKindMethod, 48},
			{"test_basic_objects", SymbolKindFunction, 67},
		}, []string{"shutil", "subprocess.run", "textwrap.dedent", "Python.Ghost", "pickle"}},
		{"python-rich-attrs-example", "SCRIPTHOLD_REAL_PY_RICH", "python", []want{
			{"Point3D", SymbolKindClass, 11}, {"Triangle", SymbolKindClass, 18},
			{"Model", SymbolKindClass, 25},
		}, []string{"attr.Factory", "attr.define", "Console", "List", "Ghost"}},
		{"csharp-automapper-features", "SCRIPTHOLD_REAL_CS_AUTOMAPPER", "csharp", []want{
			{"AutoMapper.Features", SymbolKindNamespace, 1},
			{"AutoMapper.Features.IGlobalFeature", SymbolKindInterface, 3},
			{"AutoMapper.Features.IGlobalFeature.Configure", SymbolKindMethod, 5},
			{"AutoMapper.Features.IMappingFeature", SymbolKindInterface, 7},
			{"AutoMapper.Features.IMappingFeature.Reverse", SymbolKindMethod, 10},
			{"AutoMapper.Features.IRuntimeFeature", SymbolKindInterface, 12},
			{"AutoMapper.Features.Features", SymbolKindClass, 16},
			{"AutoMapper.Features.Features.Get", SymbolKindMethod, 25},
			{"AutoMapper.Features.Features.Set", SymbolKindMethod, 34},
			{"AutoMapper.Features.FeatureExtensions", SymbolKindClass, 55},
			{"AutoMapper.Features.FeatureExtensions.SetFeature", SymbolKindMethod, 57},
		}, []string{"AutoMapper.Features.Ghost", "AutoMapper.Features.Features.TFeatureToFind", "AutoMapper.Features.MapperConfiguration", "AutoMapper.Features.Internal"}},
		{"csharp-humanizer-byte-size", "SCRIPTHOLD_REAL_CS_HUMANIZER", "csharp", []want{
			{"Benchmarks", SymbolKindNamespace, 1},
			{"Benchmarks.ByteSizeBenchmarks", SymbolKindClass, 6},
			{"Benchmarks.ByteSizeBenchmarks.ByteSizeToStringDefault", SymbolKindMethod, 12},
			{"Benchmarks.ByteSizeBenchmarks.ByteSizeToStringCulture", SymbolKindMethod, 16},
			{"Benchmarks.ByteSizeBenchmarks.ByteSizeToStringFormat", SymbolKindMethod, 20},
			{"Benchmarks.ByteSizeBenchmarks.ByteSizeToFullWords", SymbolKindMethod, 24},
		}, []string{"Benchmarks.MemoryDiagnoser", "Benchmarks.ByteSize.FromMegabytes", "Benchmarks.ByteSizeBenchmarks.Ghost"}},
		{"csharp-vs-validation-event-extensions", "SCRIPTHOLD_REAL_CS_VS_VALIDATION", "csharp", []want{
			{"Microsoft", SymbolKindNamespace, 6},
			{"Microsoft.EventHandlerExtensions", SymbolKindClass, 11},
			{"Microsoft.EventHandlerExtensions.Raise", SymbolKindMethod, 19},
			{"Microsoft.EventHandlerExtensions.Raise", SymbolKindMethod, 33},
			{"Microsoft.EventHandlerExtensions.Raise", SymbolKindMethod, 48},
		}, []string{"Microsoft.Requires.NotNull", "Microsoft.EventHandlerExtensions.DynamicInvoke", "Microsoft.EventHandlerExtensions.Ghost"}},
		{"solidity-openzeppelin-ownable", "SCRIPTHOLD_REAL_SOL_OPENZEPPELIN", "solidity", []want{
			{"Ownable", SymbolKindClass, 20},
			{"Ownable.OwnableUnauthorizedAccount", SymbolKindType, 26},
			{"Ownable.OwnableInvalidOwner", SymbolKindType, 31},
			{"Ownable.OwnershipTransferred", SymbolKindEvent, 33},
			{"Ownable.Ownable", SymbolKindConstructor, 38},
			{"Ownable.onlyOwner", SymbolKindMethod, 48},
			{"Ownable.owner", SymbolKindMethod, 56},
			{"Ownable._checkOwner", SymbolKindMethod, 63},
		}, []string{"Context", "Ownable.Context", "Ownable._msgSender", "Ownable.Ghost"}},
		{"solidity-aave-datatypes", "SCRIPTHOLD_REAL_SOL_AAVE", "solidity", []want{
			{"DataTypes", SymbolKindModule, 4},
			{"DataTypes.ReserveData", SymbolKindStruct, 5},
			{"DataTypes.ReserveData.liquidityIndex", SymbolKindField, 9},
			{"DataTypes.ReserveConfigurationMap", SymbolKindStruct, 38},
			{"DataTypes.ReserveConfigurationMap.data", SymbolKindField, 60},
			{"DataTypes.UserConfigurationMap", SymbolKindStruct, 63},
		}, []string{"ReserveData", "DataTypes.Ghost", "DataTypes.ReserveData.ReserveConfigurationMap"}},
		{"solidity-compound-unitroller", "SCRIPTHOLD_REAL_SOL_COMPOUND", "solidity", []want{
			{"Unitroller", SymbolKindClass, 11},
			{"Unitroller.NewPendingImplementation", SymbolKindEvent, 16},
			{"Unitroller.Unitroller", SymbolKindConstructor, 33},
			{"Unitroller._setPendingImplementation", SymbolKindMethod, 39},
			{"Unitroller._acceptImplementation", SymbolKindMethod, 59},
		}, []string{"Unitroller.Error", "Unitroller.fail", "Unitroller.msg", "Unitroller.Ghost"}},
		{"solidity-solmate-owned", "SCRIPTHOLD_REAL_SOL_SOLMATE", "solidity", []want{
			{"Owned", SymbolKindClass, 6},
			{"Owned.OwnershipTransferred", SymbolKindEvent, 11},
			{"Owned.owner", SymbolKindField, 17},
			{"Owned.onlyOwner", SymbolKindMethod, 19},
			{"Owned.Owned", SymbolKindConstructor, 29},
			{"Owned.transferOwnership", SymbolKindMethod, 39},
		}, []string{"Owned.require", "Owned.emit", "Owned.msg", "Owned.Ghost"}},
		{"solidity-uniswap-nodelegatecall", "SCRIPTHOLD_REAL_SOL_UNISWAP", "solidity", []want{
			{"NoDelegateCall", SymbolKindClass, 6},
			{"NoDelegateCall.original", SymbolKindField, 8},
			{"NoDelegateCall.NoDelegateCall", SymbolKindConstructor, 10},
			{"NoDelegateCall.checkNotDelegateCall", SymbolKindMethod, 18},
			{"NoDelegateCall.noDelegateCall", SymbolKindMethod, 23},
		}, []string{"NoDelegateCall.require", "NoDelegateCall.address", "NoDelegateCall.Ghost"}},
		{"apex-dlrs-bypass", "SCRIPTHOLD_REAL_APEX_DLRS", "apex", []want{
			{"BypassHandler", SymbolKindClass, 33},
			{"BypassHandler.bypassedRollups", SymbolKindField, 34},
			{"BypassHandler.bypassAll", SymbolKindField, 35},
			{"BypassHandler.isBypassed", SymbolKindMethod, 41},
			{"BypassHandler.setBypassAll", SymbolKindMethod, 51},
			{"BypassHandler.bypass", SymbolKindMethod, 58},
		}, []string{"BypassHandler.DeclarativeLookupRollupSummaries__c", "BypassHandler.System.debug", "BypassHandler.Ghost"}},
		{"apex-ebikes-paged-result", "SCRIPTHOLD_REAL_APEX_EBIKES", "apex", []want{
			{"PagedResult", SymbolKindClass, 1},
			{"PagedResult.pageSize", SymbolKindProperty, 3},
			{"PagedResult.pageNumber", SymbolKindProperty, 6},
			{"PagedResult.totalItemCount", SymbolKindProperty, 9},
			{"PagedResult.records", SymbolKindProperty, 12},
		}, []string{"PagedResult.AuraEnabled", "PagedResult.get", "PagedResult.set", "PagedResult.Ghost"}},
		{"apex-fflib-common-stringbuilder", "SCRIPTHOLD_REAL_APEX_FFLIB_COMMON", "apex", []want{
			{"fflib_StringBuilder", SymbolKindClass, 32},
			{"fflib_StringBuilder.buffer", SymbolKindField, 34},
			{"fflib_StringBuilder.fflib_StringBuilder", SymbolKindConstructor, 39},
			{"fflib_StringBuilder.fflib_StringBuilder", SymbolKindConstructor, 44},
			{"fflib_StringBuilder.add", SymbolKindMethod, 52},
			{"fflib_StringBuilder.add", SymbolKindMethod, 60},
			{"fflib_StringBuilder.toString", SymbolKindMethod, 65},
		}, []string{"fflib_StringBuilder.List", "fflib_StringBuilder.buffer.add", "fflib_StringBuilder.Ghost"}},
		{"apex-fflib-mocks-qualified", "SCRIPTHOLD_REAL_APEX_FFLIB_MOCKS", "apex", []want{
			{"fflib_QualifiedMethodAndArgValues", SymbolKindClass, 10},
			{"fflib_QualifiedMethodAndArgValues.qm", SymbolKindField, 12},
			{"fflib_QualifiedMethodAndArgValues.fflib_QualifiedMethodAndArgValues", SymbolKindConstructor, 17},
			{"fflib_QualifiedMethodAndArgValues.getQualifiedMethod", SymbolKindMethod, 25},
			{"fflib_QualifiedMethodAndArgValues.getMethodArgValues", SymbolKindMethod, 31},
			{"fflib_QualifiedMethodAndArgValues.getMockInstance", SymbolKindMethod, 37},
			{"fflib_QualifiedMethodAndArgValues.toString", SymbolKindMethod, 42},
		}, []string{"fflib_QualifiedMethodAndArgValues.NamespaceAccessible", "fflib_QualifiedMethodAndArgValues.String.join", "fflib_QualifiedMethodAndArgValues.Ghost"}},
		{"apex-lwc-customwrapper", "SCRIPTHOLD_REAL_APEX_LWC", "apex", []want{
			{"CustomWrapper", SymbolKindClass, 4},
			{"CustomWrapper.someInteger", SymbolKindProperty, 6},
			{"CustomWrapper.someString", SymbolKindProperty, 8},
			{"CustomWrapper.someList", SymbolKindProperty, 10},
		}, []string{"CustomWrapper.AuraEnabled", "CustomWrapper.get", "CustomWrapper.set", "CustomWrapper.Ghost"}},
		{"apex-npsp-cascade", "SCRIPTHOLD_REAL_APEX_NPSP", "apex", []want{
			{"ACCT_CascadeDeleteLookups_TDTM", SymbolKindClass, 36},
			{"ACCT_CascadeDeleteLookups_TDTM.getCascadeDeleteLoader", SymbolKindMethod, 42},
			{"ACCT_CascadeDeleteLookups_TDTM.getCascadeUndeleteLoader", SymbolKindMethod, 50},
			{"ACCT_CascadeDeleteLookups_TDTM.CascadeDeleteLoader", SymbolKindClass, 57},
			{"ACCT_CascadeDeleteLookups_TDTM.CascadeDeleteLoader.load", SymbolKindMethod, 65},
		}, []string{"CDL_CascadeDeleteLookups_TDTM", "ACCT_CascadeDeleteLookups_TDTM.CascadeDeleteLoader.CascadeUndeleteLoader", "ACCT_CascadeDeleteLookups_TDTM.Ghost"}},
		{"blazor-antdesign-button", "SCRIPTHOLD_REAL_BLAZOR_ANTDESIGN", "blazor", []want{}, []string{"AntDesign", "Button", "CascadingValue", "HandleOnClick", "Loading", "Ghost"}},
		{"blazor-blazorise-todoitem", "SCRIPTHOLD_REAL_BLAZOR_BLAZORISE", "blazor", []want{
			{"OnCheckedChanged", SymbolKindMethod, 23},
			{"OnRemoveClicked", SymbolKindMethod, 30},
			{"Todo", SymbolKindProperty, 33},
			{"StatusChanged", SymbolKindProperty, 35},
			{"RemoveClicked", SymbolKindProperty, 37},
		}, []string{"ListGroupItem", "OnClickCallback", "Todo.Completed", "Flex", "Ghost"}},
		{"blazor-mudblazor-codeinline", "SCRIPTHOLD_REAL_BLAZOR_MUDBLAZOR", "blazor", []want{
			{"ChildContent", SymbolKindProperty, 8},
			{"Code", SymbolKindProperty, 10},
			{"SecondaryColor", SymbolKindProperty, 12},
			{"Class", SymbolKindProperty, 14},
			{"Tag", SymbolKindProperty, 16},
		}, []string{"code", "docs-code", "RenderFragment", "Ghost"}},
		{"blazor-radzen-appearance", "SCRIPTHOLD_REAL_BLAZOR_RADZEN", "blazor", []want{}, []string{"RadzenToggleButton", "OnChange", "RadzenComponent", "Visible", "Ghost"}},
		{"blazor-samples-child", "SCRIPTHOLD_REAL_BLAZOR_SAMPLES", "blazor", []want{
			{"Title", SymbolKindProperty, 8},
			{"ChildContent", SymbolKindProperty, 11},
			{"OnClickCallback", SymbolKindProperty, 14},
		}, []string{"button", "MouseEventArgs", "Parameter", "Ghost"}},
		{"jinja-exercism-acronym", "SCRIPTHOLD_REAL_JINJA_EXERCISM", "jinja", []want{
			{"test_case", SymbolKindFunction, 5},
		}, []string{"generator_macros", "canonical_ref", "case", "test_case.input", "Ghost"}},
		{"jinja-kde-template", "SCRIPTHOLD_REAL_JINJA_KDE", "jinja", []want{
			{"content", SymbolKindSection, 32},
			{"format_comments", SymbolKindFunction, 40},
			{"join_params", SymbolKindFunction, 47},
		}, []string{"title", "items", "user", "join_params.params", "user.name", "Ghost"}},
		{"jinja-kubespray-containerd", "SCRIPTHOLD_REAL_JINJA_KUBESPRAY", "jinja", []want{}, []string{"containerd_version", "runtime", "key", "value", "containerd_storage_dir", "Ghost"}},
		{"jinja-openstack-network", "SCRIPTHOLD_REAL_JINJA_OPENSTACK", "jinja", []want{}, []string{"bootstrap_host_scenarios_expanded", "bmaas_network", "bootstrap_host_internal_address", "Ghost"}},
		{"jinja-ansible-haproxy", "SCRIPTHOLD_REAL_JINJA_ANSIBLE", "jinja", []want{}, []string{"host", "groups", "listenport", "daemonname", "Ghost"}},
		{"typescript-zustand-shallow", "SCRIPTHOLD_REAL_TS_ZUSTAND", "typescript", []want{
			{"shallow", SymbolKindFunction, 48},
		}, []string{"Map", "Object.is", "isIterable.Map", "shallow.valueA", "shallow.valueB", "Ghost"}},
		{"typescript-quill-selection", "SCRIPTHOLD_REAL_TS_QUILL", "typescript", []want{
			{"NativeRange", SymbolKindAlias, 11},
			{"NormalizedRange", SymbolKindInterface, 13},
			{"Bounds", SymbolKindInterface, 22},
			{"Bounds.bottom", SymbolKindProperty, 23},
			{"Bounds.height", SymbolKindProperty, 24},
			{"Bounds.width", SymbolKindProperty, 28},
			{"Range", SymbolKindClass, 31},
			{"Selection", SymbolKindClass, 38},
		}, []string{"LeafBlot", "Scope", "Scroll", "Emitter", "Ghost"}},
		{"typescript-ink-ansi-tokenizer", "SCRIPTHOLD_REAL_TS_INK", "typescript", []want{
			{"ControlStringType", SymbolKindAlias, 11},
			{"CsiToken", SymbolKindAlias, 13},
			{"EscToken", SymbolKindAlias, 21},
			{"AnsiToken", SymbolKindAlias, 53},
		}, []string{"ControlStringType.osc", "CsiToken.type", "TextToken.readonly", "Ghost"}},
		{"typescript-hexo-types", "SCRIPTHOLD_REAL_TS_HEXO", "typescript", []want{
			{"NodeJSLikeCallback", SymbolKindAlias, 35},
			{"RenderData", SymbolKindInterface, 37},
			{"RenderData.engine", SymbolKindProperty, 38},
			{"TagSchema", SymbolKindInterface, 50},
			{"TagSchema.name", SymbolKindProperty, 53},
			{"TagSchema.slug", SymbolKindProperty, 54},
			{"DataSchema", SymbolKindInterface, 61},
		}, []string{"moment", "Query", "default_config", "RenderData.moment", "Ghost"}},
		{"typescript-openspec-id", "SCRIPTHOLD_REAL_TS_OPENSPEC", "typescript", []want{
			{"KEBAB_ID_REGEX", SymbolKindConstant, 5},
			{"isKebabId", SymbolKindFunction, 7},
			{"KEBAB_ID_DESCRIPTION", SymbolKindConstant, 12},
			{"KEBAB_ID_FIX", SymbolKindConstant, 16},
			{"folderStyleNameProblem", SymbolKindFunction, 24},
		}, []string{"value", "label", "RegExp", "String", "Ghost"}},
		{"typescript-pretext-bidi", "SCRIPTHOLD_REAL_TS_PRETEXT", "typescript", []want{
			{"classifyCodePoint", SymbolKindFunction, 13},
			{"computeBidiLevels", SymbolKindFunction, 35},
		}, []string{"lo", "hi", "codePoint", "GeneratedBidiType", "BidiType", "Ghost"}},
		{"typescript-typechallenges-returntype", "SCRIPTHOLD_REAL_TS_TYPECHALLENGES", "typescript", []want{
			{"MyReturnType", SymbolKindAlias, 1},
		}, []string{"any", "T", "Ghost"}},
		{"typescript-wechat-utils", "SCRIPTHOLD_REAL_TS_WECHAT", "typescript", []want{
			{"cn", SymbolKindFunction, 4},
			{"formatCount", SymbolKindFunction, 8},
		}, []string{"clsx", "twMerge", "NumberFormat", "value", "Ghost"}},
		{"java-eventbus-no-subscriber", "SCRIPTHOLD_REAL_JAVA_EVENTBUS", "java", []want{
			{"org.greenrobot.eventbus", SymbolKindPackage, 16},
			{"org.greenrobot.eventbus.NoSubscriberEvent", SymbolKindClass, 23},
			{"org.greenrobot.eventbus.NoSubscriberEvent.eventBus", SymbolKindField, 25},
			{"org.greenrobot.eventbus.NoSubscriberEvent.originalEvent", SymbolKindField, 28},
			{"org.greenrobot.eventbus.NoSubscriberEvent.NoSubscriberEvent", SymbolKindConstructor, 30},
		}, []string{"EventBus", "org.greenrobot.eventbus.Ghost", "org.greenrobot.eventbus.NoSubscriberEvent.this", "org.greenrobot.eventbus.NoSubscriberEvent.EventBus"}},
		{"java-butterknife-class", "SCRIPTHOLD_REAL_JAVA_BUTTERKNIFE", "java", []want{
			{"butterknife", SymbolKindPackage, 1},
			{"butterknife.ButterKnife", SymbolKindClass, 74},
			{"butterknife.ButterKnife.ButterKnife", SymbolKindConstructor, 75},
			{"butterknife.ButterKnife.TAG", SymbolKindConstant, 79},
			{"butterknife.ButterKnife.debug", SymbolKindField, 80},
			{"butterknife.ButterKnife.setDebug", SymbolKindMethod, 86},
		}, []string{"Activity", "Dialog", "butterknife.View", "butterknife.ButterKnife.AssertionError", "butterknife.ButterKnife.Ghost"}},
		{"java-algorithms-left-rotation", "SCRIPTHOLD_REAL_JAVA_ALGORITHMS", "java", []want{
			{"com.thealgorithms.others", SymbolKindPackage, 1},
			{"com.thealgorithms.others.ArrayLeftRotation", SymbolKindClass, 10},
			{"com.thealgorithms.others.ArrayLeftRotation.ArrayLeftRotation", SymbolKindConstructor, 11},
			{"com.thealgorithms.others.ArrayLeftRotation.rotateLeft", SymbolKindMethod, 21},
		}, []string{"com.thealgorithms.others.ArrayLeftRotation.size", "com.thealgorithms.others.ArrayLeftRotation.rotated", "com.thealgorithms.others.Ghost"}},
		{"java-jcsprout-application", "SCRIPTHOLD_REAL_JAVA_JCSPROUT", "java", []want{
			{"com.crossoverjie", SymbolKindPackage, 1},
			{"com.crossoverjie.Application", SymbolKindClass, 13},
			{"com.crossoverjie.Application.LOGGER", SymbolKindConstant, 15},
			{"com.crossoverjie.Application.main", SymbolKindMethod, 18},
		}, []string{"com.crossoverjie.SpringApplication", "com.crossoverjie.Application.SpringBootApplication", "com.crossoverjie.Application.Ghost"}},
		{"java-termux-bytequeue", "SCRIPTHOLD_REAL_JAVA_TERMUX", "java", []want{
			{"com.termux.terminal", SymbolKindPackage, 1},
			{"com.termux.terminal.ByteQueue", SymbolKindClass, 4},
			{"com.termux.terminal.ByteQueue.mBuffer", SymbolKindField, 6},
			{"com.termux.terminal.ByteQueue.mHead", SymbolKindField, 7},
			{"com.termux.terminal.ByteQueue.ByteQueue", SymbolKindConstructor, 11},
			{"com.termux.terminal.ByteQueue.close", SymbolKindMethod, 15},
			{"com.termux.terminal.ByteQueue.read", SymbolKindMethod, 20},
			{"com.termux.terminal.ByteQueue.write", SymbolKindMethod, 59},
		}, []string{"com.termux.terminal.ByteQueue.mBuffer.length", "com.termux.terminal.ByteQueue.oneRun", "com.termux.terminal.ByteQueue.Ghost"}},
		{"kotlin-moshi-indexed-parameters", "SCRIPTHOLD_REAL_KT_MOSHI", "kotlin", []want{
			{"com.squareup.moshi.kotlin.reflect", SymbolKindPackage, 1},
			{"com.squareup.moshi.kotlin.reflect.IndexedParameterMap", SymbolKindClass, 4},
			{"com.squareup.moshi.kotlin.reflect.IndexedParameterMap.put", SymbolKindMethod, 9},
			{"com.squareup.moshi.kotlin.reflect.IndexedParameterMap.entries", SymbolKindProperty, 11},
			{"com.squareup.moshi.kotlin.reflect.IndexedParameterMap.containsKey", SymbolKindMethod, 19},
			{"com.squareup.moshi.kotlin.reflect.IndexedParameterMap.get", SymbolKindMethod, 21},
		}, []string{"AbstractMutableMap", "SimpleEntry", "IndexedParameterMap.allPossibleEntries", "IndexedParameterMap.Ghost"}},
		{"kotlin-timber-tree", "SCRIPTHOLD_REAL_KT_TIMBER", "kotlin", []want{
			{"timber.log", SymbolKindPackage, 1},
			{"timber.log.Timber", SymbolKindClass, 4},
			{"timber.log.Timber.Tree", SymbolKindClass, 7},
			{"timber.log.Timber.Tree.v", SymbolKindMethod, 10},
			{"timber.log.Timber.Tree.d", SymbolKindMethod, 19},
			{"timber.log.Timber.Tree.isLoggable", SymbolKindMethod, 73},
		}, []string{"timber.log.Timber.Log", "timber.log.Timber.Throwable", "timber.log.Timber.Tree.Ghost"}},
		{"kotlin-okio-forwarding-source", "SCRIPTHOLD_REAL_KT_OKIO", "kotlin", []want{
			{"okio", SymbolKindPackage, 16},
			{"okio.ForwardingSource", SymbolKindClass, 19},
			{"okio.ForwardingSource.delegate", SymbolKindProperty, 24},
			{"okio.ForwardingSource.timeout", SymbolKindMethod, 31},
			{"okio.ForwardingSource.close", SymbolKindMethod, 34},
			{"okio.ForwardingSource.toString", SymbolKindMethod, 36},
		}, []string{"okio.IOException", "okio.ForwardingSource.Source", "okio.ForwardingSource.Thrown", "okio.ForwardingSource.Ghost"}},
		{"kotlin-materialfiles-action-state", "SCRIPTHOLD_REAL_KT_MATERIALFILES", "kotlin", []want{
			{"me.zhanghai.android.files.util", SymbolKindPackage, 6},
			{"me.zhanghai.android.files.util.ActionState", SymbolKindClass, 8},
			{"me.zhanghai.android.files.util.ActionState.Ready", SymbolKindClass, 9},
			{"me.zhanghai.android.files.util.ActionState.Ready.equals", SymbolKindMethod, 10},
			{"me.zhanghai.android.files.util.ActionState.Running", SymbolKindClass, 23},
			{"me.zhanghai.android.files.util.ActionState.Success", SymbolKindClass, 25},
		}, []string{"me.zhanghai.android.files.util.ActionState.javaClass", "me.zhanghai.android.files.util.ActionState.Ghost"}},
		{"kotlin-gkd-load-status", "SCRIPTHOLD_REAL_KT_GKD", "kotlin", []want{
			{"li.songe.gkd.util", SymbolKindPackage, 1},
			{"li.songe.gkd.util.LoadStatus", SymbolKindClass, 3},
			{"li.songe.gkd.util.LoadStatus.Loading", SymbolKindClass, 4},
			{"li.songe.gkd.util.LoadStatus.Failure", SymbolKindClass, 5},
			{"li.songe.gkd.util.LoadStatus.Success", SymbolKindClass, 6},
		}, []string{"li.songe.gkd.util.Exception", "li.songe.gkd.util.LoadStatus.Ghost"}},
		{"kotlin-seal-date-time", "SCRIPTHOLD_REAL_KT_SEAL", "kotlin", []want{
			{"com.junkfood.seal.util", SymbolKindPackage, 1},
			{"com.junkfood.seal.util.toLocalizedString", SymbolKindFunction, 14},
		}, []string{"com.junkfood.seal.util.Locale", "com.junkfood.seal.util.DateFormat", "com.junkfood.seal.util.toLocalizedString.Build", "com.junkfood.seal.util.Ghost"}},
		{"kotlin-nekobox-plugin-utils", "SCRIPTHOLD_REAL_KT_NEKOBOX", "kotlin", []want{
			{"com.github.shadowsocks.plugin", SymbolKindPackage, 3},
			{"com.github.shadowsocks.plugin.Empty", SymbolKindClass, 9},
		}, []string{"com.github.shadowsocks.plugin.Parcelable", "com.github.shadowsocks.plugin.Parcelize", "com.github.shadowsocks.plugin.Ghost"}},
		{"scss-dart-docs-style", "SCRIPTHOLD_REAL_SCSS_DART", "scss", []want{
			{"monospace", SymbolKindVariable, 23},
			{"base-font-size", SymbolKindVariable, 41},
			{"header-font-size", SymbolKindVariable, 47},
			{"a", SymbolKindSelector, 25},
			{"a:visited", SymbolKindSelector, 31},
			{"div.highlight", SymbolKindSelector, 49},
		}, []string{"font-family", "font-weight", "@font-face", "header-font-size * 2", "Ghost"}},
		{"scss-hasura-beta-tag", "SCRIPTHOLD_REAL_SCSS_HASURA", "scss", []want{
			{".beta-tag", SymbolKindSelector, 1},
		}, []string{"font-size", "background-color", "--ifm-color-gray-600", "var", "Ghost"}},
		{"scss-joomla-layout", "SCRIPTHOLD_REAL_SCSS_JOOMLA", "scss", []want{
			{".media-container", SymbolKindSelector, 2},
			{".media-col-main-panel", SymbolKindSelector, 16},
			{".media-col-side-panel", SymbolKindSelector, 21},
			{".media-main", SymbolKindSelector, 41},
			{".media-sidebar", SymbolKindSelector, 47},
		}, []string{"container-bg", "col-box-shadow", "col-gutter-width", "flex-wrap", "Ghost"}},
		{"scss-ocaml-manual", "SCRIPTHOLD_REAL_SCSS_OCAML", "scss", []want{
			{".content", SymbolKindSelector, 13},
			{"#part-title", SymbolKindSelector, 15},
			{".index", SymbolKindSelector, 43},
			{".ocaml", SymbolKindSelector, 81},
		}, []string{"font-sans", "content-frame", "colored-disc-marker", "@charset", "Ghost"}},
		{"scss-wordpress-scheme-config", "SCRIPTHOLD_REAL_SCSS_WORDPRESS", "scss", []want{
			{"highlight-color", SymbolKindVariable, 1},
		}, []string{"scheme-name", "base-color", "icon-color", "notification-color", "menu-highlight-text", "menu-highlight-icon", "menu-bubble-text", "menu-submenu-text", "menu-submenu-focus-text", "dashboard-icon-background", "Ghost"}},
		{"less-antdesign-global", "SCRIPTHOLD_REAL_LESS_ANTDESIGN", "less", []want{
			{".demo-logo", SymbolKindSelector, 4},
			{".demo-logo-vertical", SymbolKindSelector, 13},
			{"html", SymbolKindSelector, 20},
			{".rc-footer", SymbolKindSelector, 29},
			{".rc-footer a", SymbolKindSelector, 37},
			{".rc-footer a:hover", SymbolKindSelector, 42},
		}, []string{"SearchBar", "@import", "scrollbar-width", "background", "Ghost"}},
		{"less-antdv-drawer", "SCRIPTHOLD_REAL_LESS_ANTDV", "less", []want{
			{".drawer > *", SymbolKindSelector, 3},
			{".drawer .drawer-mask", SymbolKindSelector, 20},
			{".drawer-content-wrapper", SymbolKindSelector, 32},
			{".drawer-content", SymbolKindSelector, 37},
			{".drawer-handle", SymbolKindSelector, 43},
		}, []string{"opacity", "transform", "cubic-bezier", "-webkit-transition", "Ghost"}},
		{"less-antmobile-gallery", "SCRIPTHOLD_REAL_LESS_ANTMOBILE", "less", []want{
			{"body", SymbolKindSelector, 1},
			{".gallery", SymbolKindSelector, 5},
			{".header", SymbolKindSelector, 11},
			{".body", SymbolKindSelector, 16},
			{".guide", SymbolKindSelector, 36},
			{".logo", SymbolKindSelector, 42},
			{".search", SymbolKindSelector, 52},
		}, []string{"--adm-color-background", "var", "overflow-y", "Ghost"}},
		{"less-antpro-global", "SCRIPTHOLD_REAL_LESS_ANTPRO", "less", []want{
			{"html", SymbolKindSelector, 42},
			{"body", SymbolKindSelector, 43},
			{"#root", SymbolKindSelector, 44},
			{".colorWeak", SymbolKindSelector, 53},
			{".ant-layout", SymbolKindSelector, 57},
		}, []string{"@font-face", "font-family", "filter", "Ghost"}},
		{"less-core-mixins", "SCRIPTHOLD_REAL_LESS_CORE", "less", []want{
			{"import-base-color", SymbolKindVariable, 2},
			{"import-accent", SymbolKindVariable, 3},
			{"import-spacing", SymbolKindVariable, 4},
			{".imported-base", SymbolKindSelector, 33},
			{".imported-card", SymbolKindSelector, 39},
		}, []string{"size", "weight", "w", "h", "dir", "gap", "Ghost"}},
		{"less-semantic-breadcrumb", "SCRIPTHOLD_REAL_LESS_SEMANTIC", "less", []want{
			{"type", SymbolKindVariable, 15},
			{"element", SymbolKindVariable, 16},
			{".ui.breadcrumb", SymbolKindSelector, 25},
			{".ui.breadcrumb:first-child", SymbolKindSelector, 31},
			{".ui.breadcrumb:last-child", SymbolKindSelector, 34},
			{".ui.breadcrumb .divider", SymbolKindSelector, 43},
		}, []string{"display", "verticalMargin", "dividerOpacity", "@import", "Ghost"}},
		{"openapi-connexion-auth-example", "SCRIPTHOLD_REAL_OAPI_CONNEXION", "openapi", []want{
			{"app.get_secret", SymbolKindOperation, 11},
		}, []string{"secret", "api_key", "securitySchemes", "X-Auth", "app.apikey_auth", "Ghost"}},
		{"openapi-oai-callback-example", "SCRIPTHOLD_REAL_OAPI_OAI", "openapi", []want{}, []string{"callbackUrl", "subscriptionId", "onData", "timestamp", "userData", "post", "Ghost"}},
		{"openapi-typescript-pets-example", "SCRIPTHOLD_REAL_OAPI_TYPESCRIPT", "openapi", []want{
			{"getTest", SymbolKindOperation, 27},
			{"listPets", SymbolKindOperation, 44},
			{"listPetsModel", SymbolKindOperation, 65},
			{"Pets", SymbolKindType, 244},
		}, []string{"/pets/items", "/pets", "limit", "response", "Ghost"}},
		{"openapi-redocly-health-example", "SCRIPTHOLD_REAL_OAPI_REDOCLY", "openapi", []want{
			{"Health", SymbolKindOperation, 10},
		}, []string{"status", "pass", "warn", "fail", "200", "400", "Ghost"}},
		{"openapi-swagger-petstore-example", "SCRIPTHOLD_REAL_OAPI_SWAGGER", "openapi", []want{
			{"updatePet", SymbolKindOperation, 45},
			{"Pet", SymbolKindType, 758},
		}, []string{"pet", "/pet", "tags", "requestBody", "Ghost"}},
		{"matlab-lewos-app", "SCRIPTHOLD_REAL_MATLAB_LEWOS", "matlab", []want{
			{"LeWoS_RS", SymbolKindClass, 1},
			{"LeWoS_RS.RunSegmentationButtonPushed", SymbolKindMethod, 36},
			{"LeWoS_RS.ShowButtonPushed", SymbolKindMethod, 55},
		}, []string{"matlab.apps.AppBase", "RunSegmentationButton", "LeWoSUIFigure", "ProgressMonitorLabel", "Ghost"}},
		{"matlab-nasa-enum", "SCRIPTHOLD_REAL_MATLAB_NASA", "matlab", []want{
			{"SensorsEnum", SymbolKindClass, 1},
		}, []string{"Simulink.IntEnumType", "None", "ZOH", "Ghost"}},
		{"matlab-platemo-problem", "SCRIPTHOLD_REAL_MATLAB_PLATEMO", "matlab", []want{
			{"CitySegMOP1", SymbolKindClass, 1},
			{"CitySegMOP1.Setting", SymbolKindMethod, 13},
		}, []string{"EvoXBenchProblem", "config", "config.args", "Ghost"}},
		{"matlab-robotics-enum", "SCRIPTHOLD_REAL_MATLAB_ROBOTICS", "matlab", []want{
			{"UIReadSubCodes", SymbolKindClass, 7},
		}, []string{"GetVbatt", "GetLbatt", "uint8", "Ghost"}},
		{"matlab-spatialmomentum", "SCRIPTHOLD_REAL_MATLAB_SPATIAL", "matlab", []want{
			{"SpatialMomentum", SymbolKindClass, 68},
			{"SpatialMomentum.new", SymbolKindMethod, 70},
		}, []string{"SpatialF6", "SpatialVec6", "SpatialForce", "Ghost"}},
		{"mql4-dingmaotu-text", "SCRIPTHOLD_REAL_MQL4_DINGMAOTU", "mql4", []want{
			{"Text", SymbolKindClass, 11},
			{"Text.setPrice", SymbolKindMethod, 14},
			{"Text.setDatetime", SymbolKindMethod, 15},
		}, []string{"FreeFormElement", "OBJPROP_TIME", "Panel", "Ghost"}},
		{"mql4-execution-skeleton", "SCRIPTHOLD_REAL_MQL4_FRAMEWORK", "mql4", []want{
			{"EAUniqueID", SymbolKindVariable, 14},
			{"allowedSlippage", SymbolKindVariable, 17},
			{"init", SymbolKindFunction, 40},
			{"start", SymbolKindFunction, 53},
			{"deinit", SymbolKindFunction, 67},
		}, []string{"TradeLogger", "criterion", "initTradeLogger", "Ghost"}},
		{"mql4-harry-adx", "SCRIPTHOLD_REAL_MQL4_ADX", "mql4", []want{
			{"ADXPeriod", SymbolKindVariable, 14},
			{"ExtBuffer1", SymbolKindVariable, 17},
			{"init", SymbolKindFunction, 22},
			{"start", SymbolKindFunction, 42},
		}, []string{"SetIndexBuffer", "iADX", "ADX0", "Ghost"}},
		{"mql4-hung96-bollinger", "SCRIPTHOLD_REAL_MQL4_BB", "mql4", []want{
			{"Risk_Pct", SymbolKindVariable, 16},
			{"MAGIC", SymbolKindVariable, 17},
			{"MODE_PRICE_STATUS", SymbolKindEnum, 29},
			{"OnInit", SymbolKindFunction, 47},
			{"ExitSignal", SymbolKindFunction, 69},
			{"OnTick", SymbolKindFunction, 73},
		}, []string{"iBands", "NrOfDigits", "PipAdjust", "Ghost"}},
		{"twig-core-markdown-apply", "SCRIPTHOLD_REAL_TWIG_CORE", "twig", []want{}, []string{"markdown_to_html", "apply", "Hello", "Ghost"}},
		{"twig-craftcms-error", "SCRIPTHOLD_REAL_TWIG_CRAFTCMS", "twig", []want{
			{"message", SymbolKindSection, 4},
		}, []string{"title", "extends", "message.title", "app", "Ghost"}},
		{"twig-drupal-announcements", "SCRIPTHOLD_REAL_TWIG_DRUPAL", "twig", []want{}, []string{"count", "featured", "standard", "attach_library", "announcements", "Ghost"}},
		{"twig-easyadmin-actionmenu", "SCRIPTHOLD_REAL_TWIG_EASYADMIN", "twig", []want{}, []string{"content", "Overlay", "ActionList", "dropdown", "Ghost"}},
		{"twig-profiler-ajax", "SCRIPTHOLD_REAL_TWIG_PROFILER", "twig", []want{
			{"toolbar", SymbolKindSection, 3},
		}, []string{"icon", "text", "sf-toolbar-value", "sf-toolbar-header", "Ghost"}},
		{"twig-shopware-invitation", "SCRIPTHOLD_REAL_TWIG_SHOPWARE", "twig", []want{}, []string{"Invitation", "Inter", "path", "fill", "Ghost"}},
		{"twig-symfony-delete-form", "SCRIPTHOLD_REAL_TWIG_SYMFONY", "twig", []want{
			{"delete-form", SymbolKindEntity, 2},
		}, []string{"admin_post_delete", "post.id", "twig:ux:icon", "Ghost"}},
		{"ejs-core-client-example", "SCRIPTHOLD_REAL_EJS_CORE", "ejs", []want{
			{"greeting", SymbolKindEntity, 7},
		}, []string{"helloTemplateFn", "messageTemplateFn", "includeFn", "compiledEjsTemplates", "Ghost"}},
		{"ejs-ecommerce-add-product", "SCRIPTHOLD_REAL_EJS_ECOMMERCE", "ejs", []want{
			{"newProductForm", SymbolKindEntity, 123},
			{"productID", SymbolKindEntity, 131},
			{"productName", SymbolKindEntity, 136},
		}, []string{"locals.product", "product.productName", "form-control", "Ghost"}},
		{"ejs-express-generator", "SCRIPTHOLD_REAL_EJS_EXPRESS", "ejs", []want{}, []string{"app", "createError", "modules", "localModules", "Ghost"}},
		{"ejs-postgresql-login", "SCRIPTHOLD_REAL_EJS_POSTGRES", "ejs", []want{
			{"email", SymbolKindEntity, 15},
			{"password", SymbolKindEntity, 19},
		}, []string{"UserLoginCtrl", "_csrf", "loginForm", "Ghost"}},
		{"ejs-typeorm-404", "SCRIPTHOLD_REAL_EJS_TYPEORM", "ejs", []want{}, []string{"head.ejs", "navigation.ejs", "end.ejs", "Page Not Found", "Ghost"}},
		{"ejs-todo-task", "SCRIPTHOLD_REAL_EJS_TODO", "ejs", []want{
			{"top", SymbolKindEntity, 24},
		}, []string{"newtask", "task", "complete", "check", "Ghost"}},
		{"ejs-upload-form", "SCRIPTHOLD_REAL_EJS_UPLOAD", "ejs", []want{
			{"file", SymbolKindEntity, 24},
		}, []string{"uploadForm", "document.getElementById", "files", "Ghost"}},
		{"cpp-btop-logger", "SCRIPTHOLD_REAL_CPP_BTOP", "cpp", []want{
			{"Logger", SymbolKindNamespace, 15},
			{"Logger.Level", SymbolKindEnum, 16},
			{"Logger.detail", SymbolKindNamespace, 26},
			{"Logger.detail.log_write", SymbolKindFunction, 29},
		}, []string{"fmt", "Level.ERROR", "log_levels.string", "Ghost"}},
		{"cpp-typesense-metrics", "SCRIPTHOLD_REAL_CPP_TYPESENSE", "cpp", []want{
			{"AppMetrics", SymbolKindClass, 14},
			{"AppMetrics.get_instance", SymbolKindMethod, 61},
			{"AppMetrics.increment_count", SymbolKindMethod, 69},
		}, []string{"spp", "Config", "current_counts.identifier", "Ghost"}},
		{"cpp-xiaozhi-state", "SCRIPTHOLD_REAL_CPP_XIAOZHI", "cpp", []want{
			{"DeviceState", SymbolKindEnum, 4},
		}, []string{"DeviceStateUnknown", "_DEVICE_STATE_H_", "Ghost"}},
		{"cpp-trafficmonitor-plugin", "SCRIPTHOLD_REAL_CPP_TRAFFIC", "cpp", []want{
			{"CPluginSystemDate", SymbolKindClass, 4},
			{"CPluginSystemDate.CPluginSystemDate", SymbolKindConstructor, 7},
			{"CPluginSystemDate.GetItemName", SymbolKindMethod, 10},
			{"CPluginSystemDate.GetItemId", SymbolKindMethod, 11},
		}, []string{"IPluginItem", "wchar_t", "Ghost"}},
		{"cpp-trojan-main", "SCRIPTHOLD_REAL_CPP_TROJAN", "cpp", []want{
			{"signal_async_wait", SymbolKindFunction, 39},
			{"main", SymbolKindFunction, 64},
		}, []string{"Service", "sig.async_wait", "config_file", "po", "Ghost"}},
		{"jsp-ecommerce-admin-home", "SCRIPTHOLD_REAL_JSP_ECOMMERCE", "jsp", []want{
			{"navbarSupportedContent", SymbolKindEntity, 33},
		}, []string{"navbar-toggler", "bg-dark", "Home Page", "Ghost"}},
		{"jsp-final-user-form", "SCRIPTHOLD_REAL_JSP_FINAL", "jsp", []want{}, []string{"user", "request.getContextPath", "User Management Application", "Ghost"}},
		{"jsp-in28minutes-add-todo", "SCRIPTHOLD_REAL_JSP_IN28", "jsp", []want{}, []string{"todo", "category", "add", "header.jspf", "Ghost"}},
		{"jsp-javaee7-builtins", "SCRIPTHOLD_REAL_JSP_JAVAEE", "jsp", []want{}, []string{"pageContext", "TestServlet", "Greeting Service", "Ghost"}},
		{"jsp-servlet-book", "SCRIPTHOLD_REAL_JSP_SERVLET", "jsp", []want{
			{"book", SymbolKindEntity, 9},
		}, []string{"com.demo.Book", "bookForm", "java.sql", "Ghost"}},
		{"razor-abp-multitenancy", "SCRIPTHOLD_REAL_RAZOR_ABP", "razor", []want{
			{"Model", SymbolKindProperty, 16},
		}, []string{"AbpCompilationRazorPageBase", "Response.StatusCode", "CultureInfo.CurrentCulture.Name", "Ghost"}},
		{"razor-eshop-basket", "SCRIPTHOLD_REAL_RAZOR_ESHOP", "razor", []want{}, []string{"BasketComponentViewModel", "ItemsCount", "ViewData", "Ghost"}},
		{"razor-nopcommerce-config", "SCRIPTHOLD_REAL_RAZOR_NOP", "razor", []want{}, []string{"savecustomerrolesrequirement@(Model.RequirementId)", "pnl-save-requirement-result@(Model.RequirementId)", "Html.IdFor", "Ghost"}},
		{"razor-orchardcore-dashboard", "SCRIPTHOLD_REAL_RAZOR_ORCHARD", "razor", []want{}, []string{"DisplayAsync", "AdminDashboardContent", "Welcome to Orchard Core", "Ghost"}},
		{"razor-umbraco-login", "SCRIPTHOLD_REAL_RAZOR_UMBRACO", "razor", []want{
			{"login-error", SymbolKindEntity, 128},
			{"username", SymbolKindEntity, 137},
			{"password", SymbolKindEntity, 144},
		}, []string{"Url.Action", "login-container", "externalProviders.Count", "Ghost"}},
		{"arduino-biomurph-mouseeye", "SCRIPTHOLD_REAL_ARDUINO_BIOMURPH", "arduino", []want{
			{"noMove", SymbolKindVariable, 12},
			{"setup", SymbolKindFunction, 14},
			{"loop", SymbolKindFunction, 21},
		}, []string{"Mouse.getMouse", "Mouse.deltaX", "Serial.println", "Ghost"}},
		{"arduino-eggduino-sketch", "SCRIPTHOLD_REAL_ARDUINO_EGGDUINO", "arduino", []want{
			{"penMin", SymbolKindVariable, 69},
			{"penMax", SymbolKindVariable, 70},
			{"setup", SymbolKindFunction, 87},
			{"loop", SymbolKindFunction, 93},
		}, []string{"makeComInterface", "initHardware", "moveOneStep", "rotMotor.step1", "Ghost"}},
		{"arduino-farmbot-controller", "SCRIPTHOLD_REAL_ARDUINO_FARMBOT", "arduino", []want{
			{"setup", SymbolKindFunction, 15},
			{"loop", SymbolKindFunction, 49},
		}, []string{"startSerial", "readParameters", "homeOnBoot", "Ghost"}},
		{"arduino-dccex-station", "SCRIPTHOLD_REAL_ARDUINO_DCC", "arduino", []want{
			{"setup", SymbolKindFunction, 80},
		}, []string{"SerialManager.init", "IODevice.begin", "MAX_LOCOS", "Ghost"}},
		{"arduino-gvret-console", "SCRIPTHOLD_REAL_ARDUINO_GVRET", "arduino", []want{
			{"i", SymbolKindVariable, 47},
			{"serialBufferLength", SymbolKindVariable, 50},
			{"SWCAN_Int", SymbolKindFunction, 68},
			{"loadSettings", SymbolKindFunction, 75},
		}, []string{"SWCAN.intHandler", "settings.version", "EEPROM.read", "Ghost"}},
		{"arduino-stc1000-com", "SCRIPTHOLD_REAL_ARDUINO_STC", "arduino", []want{
			{"write_bit", SymbolKindFunction, 33},
			{"read_bit", SymbolKindFunction, 47},
			{"write_byte", SymbolKindFunction, 62},
			{"read_byte", SymbolKindFunction, 71},
			{"write_eeprom", SymbolKindFunction, 86},
		}, []string{"COM_PIN", "digitalWrite", "pinMode", "Ghost"}},
		{"arduino-fugu-sensors", "SCRIPTHOLD_REAL_ARDUINO_FUGU", "arduino", []want{
			{"ADC_SetGain", SymbolKindFunction, 1},
			{"resetVariables", SymbolKindFunction, 13},
			{"Read_Sensors", SymbolKindFunction, 19},
		}, []string{"ads.setGain", "secondsElapsed", "ADC_BitReso", "Ghost"}},
		{"classic-basic-bluebat-ascii", "SCRIPTHOLD_REAL_CB_BLUEBAT", "classic-basic", []want{
			{"A", SymbolKindVariable, 5},
		}, []string{"CBASE", "ACODE", "GOSUB", "Ghost"}},
		{"classic-basic-dualbrain-lanturn", "SCRIPTHOLD_REAL_CB_DUALBRAIN", "classic-basic", []want{
			{"FNSS4", SymbolKindFunction, 5},
			{"FNCOLLIDE", SymbolKindFunction, 6},
			{"WX", SymbolKindVariable, 7},
			{"BX", SymbolKindVariable, 8},
		}, []string{"KEY", "FN", "GOSUB", "Ghost"}},
		{"classic-basic-gwbasic-3d", "SCRIPTHOLD_REAL_CB_GWBASIC3D", "classic-basic", []want{
			{"M", SymbolKindVariable, 8},
			{"P", SymbolKindVariable, 9},
			{"H", SymbolKindVariable, 10},
		}, []string{"ACTIVE", "POINTS", "GOSUB", "Ghost"}},
		{"classic-basic-jasbales-bmi", "SCRIPTHOLD_REAL_CB_JASBALES", "classic-basic", []want{},
			[]string{"weight", "weightKg", "heightMeters", "bmi", "Ghost"}},
		{"classic-basic-jumpup-game", "SCRIPTHOLD_REAL_CB_JUMPUP", "classic-basic", []want{},
			[]string{"NAM$", "LEVEL", "T", "Ghost"}},
		{"classic-basic-menu", "SCRIPTHOLD_REAL_CB_MENU", "classic-basic", []want{
			{"S$", SymbolKindVariable, 2},
		}, []string{"C1", "X1", "Y1", "Ghost"}},
		{"classic-basic-shravan-budget", "SCRIPTHOLD_REAL_CB_SHRAVAN", "classic-basic", []want{},
			[]string{"A", "C", "F", "Ghost"}},
		{"haskell-highlightjs-udp", "SCRIPTHOLD_REAL_HS_HIGHLIGHTJS", "haskell", []want{
			{"Network.UDP", SymbolKindModule, 2},
			{"Network.UDP.DataPacket", SymbolKindInterface, 19},
			{"Network.UDP.openBoundUDPPort", SymbolKindFunction, 29},
			{"Network.UDP.pingUDPPort", SymbolKindFunction, 37},
		}, []string{"Network", "Network.UDP.Strict", "Network.UDP.bindAddr", "Network.UDP.Ghost"}},
		{"haskell-chroma-numeric", "SCRIPTHOLD_REAL_HS_CHROMA", "haskell", []want{},
			[]string{"Network", "x", "e", "Ghost"}},
		{"objective-c-appdelegate", "SCRIPTHOLD_REAL_OBJC_APPDELEGATE", "objective-c", []want{
			{"RCTAppDelegate@implementation", SymbolKindImplementation, 31},
			{"RCTAppDelegate.init", SymbolKindConstructor, 33},
			{"RCTAppDelegate.rootViewFactory", SymbolKindMethod, 65},
		}, []string{"RCTAppDelegate.alloc", "RCTAppDelegate.Ghost", "RCTReactNativeFactory", "MyCppNSStringWrapper"}},
		{"objective-c-testlib-apple", "SCRIPTHOLD_REAL_OBJC_APPLE", "objective-c", []want{
			{"TestLibraryApple@implementation", SymbolKindImplementation, 16},
		}, []string{"TestLibraryCommon", "RCT_EXPORT_MODULE", "TestLibraryApple.Ghost"}},
		{"objective-c-testlib-common", "SCRIPTHOLD_REAL_OBJC_COMMON", "objective-c", []want{
			{"TestLibraryCommon@implementation", SymbolKindImplementation, 10},
			{"TestLibraryCommon.defaultPrefix", SymbolKindMethod, 14},
		}, []string{"RCTPromiseResolveBlock", "TestLibraryCommon.Ghost", "resolve"}},
		{"objective-c-animation-plugins", "SCRIPTHOLD_REAL_OBJC_ANIMATION", "objective-c", []want{},
			[]string{"RCTAnimationClassProvider", "RCTNativeAnimatedModuleCls", "Class", "Ghost"}},
		{"objective-c-blob-plugins", "SCRIPTHOLD_REAL_OBJC_BLOB", "objective-c", []want{},
			[]string{"RCTBlobClassProvider", "RCTBlobManagerCls", "Class", "Ghost"}},
		{"objective-c-image-plugins", "SCRIPTHOLD_REAL_OBJC_IMAGE", "objective-c", []want{},
			[]string{"RCTImageClassProvider", "RCTImageLoaderCls", "Class", "Ghost"}},
		{"objective-c-gist-cpp-only", "SCRIPTHOLD_REAL_OBJC_GIST", "objective-c", []want{},
			[]string{"MyCppNSStringWrapper", "main", "NSString", "NSAutoreleasePool", "Ghost"}},
		{"objective-cpp-gist-wrapper", "SCRIPTHOLD_REAL_OBJCPP_GIST", "objective-cpp", []want{
			{"MyCppNSStringWrapper", SymbolKindClass, 10},
			{"MyCppNSStringWrapper.length", SymbolKindMethod, 15},
			{"MyCppNSStringWrapper.characterAtIndex", SymbolKindMethod, 16},
			{"main", SymbolKindFunction, 49},
		}, []string{"NSString", "NSAutoreleasePool", "alloc", "initWithCString", "Ghost"}},
		{"objective-cpp-react-appdelegate", "SCRIPTHOLD_REAL_OBJCPP_APPDELEGATE", "objective-cpp", []want{
			{"RCTAppDelegate@implementation", SymbolKindImplementation, 31},
			{"RCTAppDelegate.init", SymbolKindConstructor, 33},
			{"RCTAppDelegate.rootViewFactory", SymbolKindMethod, 65},
		}, []string{"RCTReactNativeFactory", "RCTAppDelegate.Ghost", "RCTAppDelegate.alloc", "RCTAppDelegate.self"}},
		{"objective-cpp-react-animation-utils", "SCRIPTHOLD_REAL_OBJCPP_ANIMATION", "objective-cpp", []want{
			{"RCTFindIndexOfNearestValue", SymbolKindFunction, 16},
			{"RCTInterpolateValue", SymbolKindFunction, 32},
			{"RCTInterpolateValueInRange", SymbolKindFunction, 71},
			{"RCTInterpolateColorInRange", SymbolKindFunction, 87},
		}, []string{"CGFloat", "RCTLogError", "getRed", "index", "Ghost"}},
		{"objective-cpp-react-linking", "SCRIPTHOLD_REAL_OBJCPP_LINKING", "objective-cpp", []want{
			{"RCTLinkingClassProvider", SymbolKindFunction, 20},
		}, []string{"RCTLinkingManagerCls", "Class", "std", "Ghost"}},
		{"objective-cpp-testlib-apple", "SCRIPTHOLD_REAL_OBJCPP_TESTAPPLE", "objective-cpp", []want{
			{"TestLibraryApple@implementation", SymbolKindImplementation, 16},
		}, []string{"RCTPromiseResolveBlock", "TestLibraryCommon", "stringWithFormat", "Ghost"}},
		{"objective-cpp-testlib-common", "SCRIPTHOLD_REAL_OBJCPP_TESTCOMMON", "objective-cpp", []want{
			{"TestLibraryCommon@implementation", SymbolKindImplementation, 10},
			{"TestLibraryCommon.defaultPrefix", SymbolKindMethod, 14},
		}, []string{"RCT_EXPORT_MODULE", "RCTPromiseResolveBlock", "resolve", "Ghost"}},
		{"dart-sdk-core-bool", "SCRIPTHOLD_REAL_DART_SDK", "dart", []want{
			{"bool", SymbolKindClass, 16},
			{"bool.parse", SymbolKindMethod, 159},
			{"bool.fromEnvironment", SymbolKindConstructor, 67},
		}, []string{"Logger", "bool.loggingEnabled", "bool.indentLines", "Ghost"}},
		{"dart-chroma-monte-carlo", "SCRIPTHOLD_REAL_DART_CHROMA", "dart", []want{
			{"main", SymbolKindFunction, 10},
			{"computePi", SymbolKindFunction, 21},
			{"generateRandom", SymbolKindFunction, 39},
			{"Point", SymbolKindClass, 46},
			{"Point.Point", SymbolKindConstructor, 48},
		}, []string{"querySelector", "estimate", "inside", "SomeMissing", "Ghost"}},
		{"dart-highlightjs-sample", "SCRIPTHOLD_REAL_DART_HIGHLIGHTJS", "dart", []want{
			{"SomeClass", SymbolKindClass, 10},
			{"SomeClass.SomeClass", SymbolKindConstructor, 11},
			{"SomeClass._internal", SymbolKindConstructor, 12},
			{"SomeClass.sum", SymbolKindMethod, 18},
			{"SomeClass.els", SymbolKindMethod, 20},
			{"checkVersion", SymbolKindFunction, 35},
		}, []string{"Awesome", "lookUpVersion", "SomeClass.z", "Ghost"}},
		{"dart-pygments-greeter", "SCRIPTHOLD_REAL_DART_PYGMENTS", "dart", []want{
			{"Greeter", SymbolKindClass, 3},
			{"Greeter.Greeter", SymbolKindConstructor, 5},
			{"Greeter.withPrefix", SymbolKindConstructor, 6},
			{"Greeter.greet", SymbolKindMethod, 7},
			{"Greeter.compareTo", SymbolKindMethod, 9},
			{"main", SymbolKindFunction, 12},
		}, []string{"Comparable", "Greeter.greeter", "Greeter.Ghost", "print"}},
		{"bash-ansible-dns-config", "SCRIPTHOLD_REAL_SH_ANSIBLE", "bash", []want{
			{"print_help", SymbolKindFunction, 21},
			{"checkForProgram", SymbolKindFunction, 111},
			{"checkDomain", SymbolKindFunction, 126},
			{"checkRecord", SymbolKindFunction, 144},
			{"deleteRecord", SymbolKindFunction, 159},
			{"writeDNS", SymbolKindFunction, 165},
		}, []string{"DO_PAT", "PARAMS", "record_type", "domain", "force_add", "Ghost"}},
		{"shell-ansible-dns-config", "SCRIPTHOLD_REAL_SH_ANSIBLE", "shell", []want{
			{"print_help", SymbolKindFunction, 21},
			{"checkForProgram", SymbolKindFunction, 111},
			{"checkDomain", SymbolKindFunction, 126},
			{"checkRecord", SymbolKindFunction, 144},
			{"deleteRecord", SymbolKindFunction, 159},
			{"writeDNS", SymbolKindFunction, 165},
		}, []string{"DO_PAT", "record_name", "Ghost"}},
		{"shell-arduino-wifi-upgrade", "SCRIPTHOLD_REAL_SH_ARDUINO", "shell", []want{
			{"usage", SymbolKindFunction, 11},
			{"upgradeHDmodule", SymbolKindFunction, 23},
			{"upgradeShield", SymbolKindFunction, 39},
		}, []string{"WIFI_FW_PATH", "progname", "cat", "dfu-programmer", "Ghost"}},
		{"shell-curl-codespell", "SCRIPTHOLD_REAL_SH_CURL", "shell", []want{},
			[]string{"codespell", "git", "xargs", "skip", "Ghost"}},
		{"shell-automapper-prebuild", "SCRIPTHOLD_REAL_SH_AUTOMAPPER", "shell", []want{},
			[]string{"version", "major", "minor", "patch", "oldVersion", "curl", "unzip", "Ghost"}},
		{"shell-testify-gofmt", "SCRIPTHOLD_REAL_SH_TESTIFY", "shell", []want{},
			[]string{"gofmt", "git", "go", "Ghost"}},
		{"shell-kotlin-runtest", "SCRIPTHOLD_REAL_SH_KOTLIN", "shell", []want{},
			[]string{"FILTERED_FILE", "cat", "sed", "konanc", "Ghost"}},
		{"shell-fastapi-format", "SCRIPTHOLD_REAL_SH_FASTAPI", "shell", []want{},
			[]string{"ruff", "check", "format", "Ghost"}},
		{"shell-rust-clean-all", "SCRIPTHOLD_REAL_SH_RUST", "shell", []want{},
			[]string{"rm", "target", "Ghost"}},
		{"purebasic-aliustaoglu-main", "SCRIPTHOLD_REAL_PB_ALIUSTAOGLU", "purebasic", []want{
			{"Windows", SymbolKindEnum, 6},
			{"MenuBar", SymbolKindEnum, 10},
			{"MenuItems", SymbolKindEnum, 14},
			{"StatusBar", SymbolKindEnum, 23},
			{"UpdateWindow", SymbolKindFunction, 27},
			{"Main", SymbolKindFunction, 34},
		}, []string{"dx", "dy", "WindowWidth", "ProgramTitle", "Ghost"}},
		{"purebasic-falsam-wan-ip", "SCRIPTHOLD_REAL_PB_FALSAM", "purebasic", []want{
			{"GetWanIp", SymbolKindFunction, 1},
		}, []string{"IP", "n", "Buffer", "ReceiveHTTPMemory", "MakeIPAddress", "Ghost"}},
		{"purebasic-kenmo-7zip", "SCRIPTHOLD_REAL_PB_KENMO", "purebasic", []want{
			{"__7ZIPSTRUCT", SymbolKindStruct, 33},
			{"__7ZIPSTRUCT.Init", SymbolKindField, 34},
			{"__7ZIPSTRUCT.Output", SymbolKindField, 37},
			{"__7Zip_Run", SymbolKindFunction, 59},
			{"Reset7Zip", SymbolKindFunction, 87},
			{"Set7ZipPassword", SymbolKindFunction, 94},
			{"Set7ZipFlags", SymbolKindFunction, 100},
		}, []string{"PID", "Executable", "RunProgram", "SevenZip_IncludeVersion", "Ghost"}},
		{"cpp-cli-iocompression-assembly", "SCRIPTHOLD_REAL_CPPCLI_IO", "cpp-cli", []want{},
			[]string{"System", "System.Reflection", "AssemblyTitleAttribute", "Ghost"}},
		{"cpp-cli-libyara-meta", "SCRIPTHOLD_REAL_CPPCLI_LIBYARA", "cpp-cli", []want{
			{"libyaraNET", SymbolKindNamespace, 7},
			{"libyaraNET.Meta", SymbolKindClass, 9},
			{"libyaraNET.Meta.Meta", SymbolKindConstructor, 15},
		}, []string{"System", "libyaraNET.Identifier", "libyaraNET.Ghost"}},
		{"cpp-cli-managedfbx-pose", "SCRIPTHOLD_REAL_CPPCLI_FBX", "cpp-cli", []want{
			{"ArcManagedFBX", SymbolKindNamespace, 3},
			{"ArcManagedFBX.FBXPose", SymbolKindClass, 5},
		}, []string{"ArcManagedFBX.ref", "ArcManagedFBX.private", "ArcManagedFBX.Ghost"}},
		{"cpp-cli-managedffmpeg-constants", "SCRIPTHOLD_REAL_CPPCLI_FFMPEG", "cpp-cli", []want{
			{"FFMpegNet", SymbolKindNamespace, 5},
			{"FFMpegNet.AVFormat", SymbolKindNamespace, 7},
			{"FFMpegNet.AVFormat.AVConstants", SymbolKindClass, 9},
			{"FFMpegNet.AVFormat.AVConstants.AVConstants", SymbolKindConstructor, 12},
		}, []string{"System", "FFMpegNet.AVFormat.Ghost", "FFMpegNet.AVFormat.AVConstants.__int64"}},
		{"cpp-cli-ntaglib-assembly", "SCRIPTHOLD_REAL_CPPCLI_NTAGLIB", "cpp-cli", []want{},
			[]string{"System", "System.Reflection", "AssemblyTitle", "AssemblyVersion", "Ghost"}},
		{"cpp-cli-physx-constraint", "SCRIPTHOLD_REAL_CPPCLI_PHYSX", "cpp-cli", []want{
			{"PhysX", SymbolKindNamespace, 3},
			{"PhysX.ConstraintShaderTable", SymbolKindClass, 5},
		}, []string{"PhysX.ref", "PhysX.Ghost", "ConstraintShaderTable.Ghost"}},
		{"vba-commonlib-arrayutils", "SCRIPTHOLD_REAL_VBA_COMMONLIB", "vba", []want{
			{"VBALib_ArrayUtils", SymbolKindModule, 1},
			{"VBALib_ArrayUtils.NORMALIZE_LBOUND", SymbolKindConstant, 11},
			{"VBALib_ArrayUtils.NormalizeArray", SymbolKindMethod, 17},
			{"VBALib_ArrayUtils.CopyMemory", SymbolKindFunction, 8},
			{"VBALib_ArrayUtils.ArrayLen", SymbolKindMethod, 118},
		}, []string{"VBALib_ArrayUtils.IsEmpty", "VBALib_ArrayUtils.UBound", "VBALib_ArrayUtils.Ghost"}},
		{"vba-macrolibrary-main", "SCRIPTHOLD_REAL_VBA_MACROLIB", "vba", []want{
			{"ModuleX", SymbolKindModule, 1},
			{"ModuleX.WorksheetExists", SymbolKindMethod, 10},
			{"ModuleX.CopyRowAboveAndInsert", SymbolKindMethod, 16},
			{"ModuleX.ProperX", SymbolKindMethod, 31},
			{"ModuleX.Email_part", SymbolKindMethod, 37},
		}, []string{"ModuleX.Evaluate", "ModuleX.ActiveSheet", "ModuleX.Ghost"}},
		{"vba-modernjson-common", "SCRIPTHOLD_REAL_VBA_MODERNJSON", "vba", []want{
			{"Json_Common", SymbolKindModule, 1},
			{"Json_Common.JSON_TAG_OBJECT", SymbolKindConstant, 26},
			{"Json_Common.JsonTextBuilder", SymbolKindType, 33},
			{"Json_Common.JsonStringIndex", SymbolKindType, 51},
			{"Json_Common.JsonSB_Init", SymbolKindMethod, 64},
		}, []string{"Json_Common.Ghost", "Json_Common.Space", "Json_Common.Len"}},
		{"vba-productivity-convert", "SCRIPTHOLD_REAL_VBA_PRODUCTIVITY", "vba", []want{
			{"ConvertTextToNumber", SymbolKindModule, 1},
			{"ConvertTextToNumber.run", SymbolKindMethod, 3},
			{"ConvertTextToNumber.run_WithDecimals", SymbolKindMethod, 11},
			{"ConvertTextToNumber.ConvertTextToNumber", SymbolKindMethod, 20},
			{"ConvertTextToNumber.convertTextToNumber_CountDecimals", SymbolKindMethod, 43},
		}, []string{"ConvertTextToNumber.Selection", "ConvertTextToNumber.WorksheetFunction", "ConvertTextToNumber.Ghost"}},
		{"vba-xlframe-ezarray", "SCRIPTHOLD_REAL_VBA_XLFRAME", "vba", []want{
			{"EZArray.Resize", SymbolKindMethod, 13},
			{"EZArray.NumRows", SymbolKindMethod, 54},
		}, []string{"EZArray.WorksheetFunction", "EZArray.UBound", "EZArray.Ghost"}},
		{"vba-xplus-math", "SCRIPTHOLD_REAL_VBA_XPLUS", "vba", []want{
			{"xpMath", SymbolKindModule, 1},
			{"xpMath.INTERPOLATE_NUMBER", SymbolKindMethod, 7},
			{"xpMath.INTERPOLATE_PERCENT", SymbolKindMethod, 29},
		}, []string{"xpMath.startingNumber", "xpMath.endingNumber", "xpMath.Ghost"}},
		{"php-html-wordpress-404", "SCRIPTHOLD_REAL_PHPHTML_WORDPRESS", "php-html", []want{
			{"primary", SymbolKindEntity, 12},
			{"content", SymbolKindEntity, 13},
			{"post-0", SymbolKindEntity, 15},
		}, []string{"WP_Widget_Recent_Posts", "get_search_form", "the_widget", "Ghost"}},
		{"php-html-joomla-category", "SCRIPTHOLD_REAL_PHPHTML_JOOMLA", "php-html", []want{},
			[]string{"LayoutHelper", "subtemplatename", "joomla.content.category_default", "Ghost"}},
		{"php-html-storefront-content", "SCRIPTHOLD_REAL_PHPHTML_STOREFRONT", "php-html", []want{},
			[]string{"post-<?php the_ID(); ?>", "post-", "storefront_page", "Ghost"}},
		{"php-html-lexilla-comments", "SCRIPTHOLD_REAL_PHPHTML_LEXILLA", "php-html", []want{},
			[]string{"foo", "comment", "Ghost"}},
		{"php-html-chroma-sample", "SCRIPTHOLD_REAL_PHPHTML_CHROMA", "php-html", []want{},
			[]string{"modx", "docs", "doc", "Some value", "Ghost"}},
		{"php-html-highlightjs-echo", "SCRIPTHOLD_REAL_PHPHTML_HIGHLIGHTJS", "php-html", []want{},
			[]string{"Hello, world!", "echo", "Ghost"}},
		{"php-html-pygments-incomplete", "SCRIPTHOLD_REAL_PHPHTML_PYGMENTS", "php-html", []want{},
			[]string{"php", "Ghost"}},
		{"cil-asmresolver-forwarders", "SCRIPTHOLD_REAL_CIL_ASMRESOLVER", "cil", []want{
			{"ForwarderLibrary", SymbolKindModule, 1},
			{"ForwarderLibrary.IMyModel", SymbolKindClass, 7},
			{"ForwarderLibrary.MyClass", SymbolKindClass, 12},
		}, []string{"ActualLibrary", "ForwarderLibrary.ActualLibrary", "Ghost"}},
		{"cil-cecil-hello", "SCRIPTHOLD_REAL_CIL_CECIL", "cil", []want{
			{"Hello", SymbolKindModule, 7},
			{"Hello.Foo", SymbolKindClass, 11},
			{"Hello.Foo.Bar", SymbolKindMethod, 20},
			{"Hello.Foo.Baz", SymbolKindMethod, 32},
			{"Hello.Foo.PrintEmpty", SymbolKindMethod, 44},
		}, []string{"mscorlib", "Hello.Foo.res", "Hello.Foo.Ghost"}},
		{"cil-ilspy-bitnot", "SCRIPTHOLD_REAL_CIL_ILSPY", "cil", []want{
			{"ConvTest", SymbolKindModule, 7},
			{"ConvTest.Program", SymbolKindClass, 15},
			{"ConvTest.Program.Main", SymbolKindMethod, 18},
			{"ConvTest.Program.BitwiseComplementInNativeSize", SymbolKindMethod, 42},
			{"ConvTest.Program.PrintHex", SymbolKindMethod, 73},
			{"ConvTest.Enums.EUInt16", SymbolKindClass, 92},
			{"ConvTest.Enums.EUInt16.__value", SymbolKindField, 95},
		}, []string{"ConvTest.Program.not", "ConvTest.Program.Ghost", "mscorlib"}},
		{"cil-mono-generic", "SCRIPTHOLD_REAL_CIL_MONO", "cil", []want{
			{"CS0012-10-lib", SymbolKindModule, 9},
			{"CS0012-10-lib.I", SymbolKindClass, 45},
		}, []string{"CS0012-lib-missing", "mscorlib", "Ghost"}},
		{"cil-runtime-arithmetic", "SCRIPTHOLD_REAL_CIL_RUNTIME", "cil", []want{
			{"BasicArithmeticTests", SymbolKindModule, 8},
			{"BasicArithmeticTests.BasicArithmeticTestsType", SymbolKindClass, 12},
			{"BasicArithmeticTests.BasicArithmeticTestsType.SimpleAdd_Invalid_ExpectedNumericType", SymbolKindMethod, 25},
		}, []string{"System.Runtime", "BasicArithmeticTests.BasicArithmeticTestsType.ldstr", "Ghost"}},
		{"cil-roslyn-class-layout", "SCRIPTHOLD_REAL_CIL_ROSLYN", "cil", []want{
			{"b", SymbolKindModule, 12},
			{"b.S1", SymbolKindClass, 32},
			{"b.S2", SymbolKindClass, 39},
			{"b.S3", SymbolKindClass, 46},
			{"b.S4", SymbolKindClass, 53},
			{"b.S5", SymbolKindClass, 60},
		}, []string{"mscorlib", "b.Ghost", ".pack"}},
		{"luau-dekkonot-sha512", "SCRIPTHOLD_REAL_LUAU_DEKKONOT", "luau", []want{
			{"lil_sig0", SymbolKindFunction, 51},
			{"lil_sig1", SymbolKindFunction, 73},
			{"big_sig0", SymbolKindFunction, 94},
		}, []string{"bit32.bxor", "bit32.rshift", "BLOCK_FRONT", "Ghost"}},
		{"luau-evercyan-remotes", "SCRIPTHOLD_REAL_LUAU_EVERCYAN", "luau", []want{
			{"ServerRequestBlob", SymbolKindType, 42},
			{"ServerInvRequestBlob", SymbolKindType, 46},
			{"ClientRequestBlob", SymbolKindType, 50},
			{"Remote", SymbolKindType, 58},
			{"ConvertToUnion", SymbolKindFunction, 74},
			{"ProcessHasProperTypes", SymbolKindFunction, 85},
		}, []string{"Remote.FireServer", "HttpService.GetService", "ReferenceParams", "Ghost"}},
		{"luau-rbxutil-util", "SCRIPTHOLD_REAL_LUAU_RBXUTIL", "luau", []want{
			{"AnyTable", SymbolKindType, 7},
			{"Util.DeepFreeze", SymbolKindFunction, 14},
			{"Util.DeepCopy", SymbolKindFunction, 25},
			{"Util.Extend", SymbolKindFunction, 37},
		}, []string{"Util.None", "table.freeze", "table.clone", "Ghost"}},
		{"luau-sleitnick-find", "SCRIPTHOLD_REAL_LUAU_SLEITNICK", "luau", []want{
			{"find", SymbolKindFunction, 3},
		}, []string{"instance.FindFirstChild", "instance.GetFullName", "select", "Ghost"}},
		{"luau-pvz-wave", "SCRIPTHOLD_REAL_LUAU_PVZ", "luau", []want{},
			[]string{"Name", "Aliases", "Description", "Group", "Ghost"}},
		{"fsharp-fsharpplus-arrow", "SCRIPTHOLD_REAL_FS_FSHARPPLUS", "fsharp", []want{
			{"FSharpPlus.Control", SymbolKindNamespace, 1},
			{"FSharpPlus.Control.Arr", SymbolKindType, 13},
			{"FSharpPlus.Control.Arr.Arr", SymbolKindMethod, 15},
			{"FSharpPlus.Control.Arr.Arr", SymbolKindMethod, 16},
			{"FSharpPlus.Control.Arr.Invoke", SymbolKindMethod, 18},
			{"FSharpPlus.Control.Arr.InvokeOnInstance", SymbolKindMethod, 22},
			{"FSharpPlus.Control.Arr.Arr", SymbolKindMethod, 25},
			{"FSharpPlus.Control.ArrFirst", SymbolKindType, 29},
			{"FSharpPlus.Control.ArrFirst.First", SymbolKindMethod, 31},
			{"FSharpPlus.Control.ArrFirst.Invoke", SymbolKindMethod, 34},
			{"FSharpPlus.Control.ArrFirst.First", SymbolKindMethod, 41},
			{"FSharpPlus.Control.ArrSecond", SymbolKindType, 45},
			{"FSharpPlus.Control.ArrSecond.Second", SymbolKindMethod, 47},
			{"FSharpPlus.Control.ArrSecond.Invoke", SymbolKindMethod, 50},
			{"FSharpPlus.Control.ArrCombine", SymbolKindType, 65},
			{"FSharpPlus.Control.ArrCombine.***", SymbolKindMethod, 67},
			{"FSharpPlus.Control.ArrCombine.***", SymbolKindMethod, 77},
			{"FSharpPlus.Control.Fanout", SymbolKindType, 83},
			{"FSharpPlus.Control.Fanout.&&&", SymbolKindMethod, 85},
			{"FSharpPlus.Control.Fanout.&&&", SymbolKindMethod, 95},
		}, []string{"FSharpPlus.Control.Arr.static", "FSharpPlus.Control.Arr.inline",
			"FSharpPlus.Control.Arr.call", "FSharpPlus.Control.ArrFirst.call",
			"FSharpPlus.Control.Arr.Second", "FSharpPlus.Control.Ghost"}},
		{"fsharp-fsharpdata-runtime", "SCRIPTHOLD_REAL_FS_DATA", "fsharp", []want{
			{"FSharp.Data", SymbolKindNamespace, 14},
			{"FSharp.Data.JsonSchemaValidationResult", SymbolKindType, 20},
			{"FSharp.Data.JsonSchemaExtensions", SymbolKindModule, 25},
			{"FSharp.Data.JsonSchemaExtensions.ParseJsonSchema", SymbolKindFunction, 27},
			{"FSharp.Data.JsonSchemaExtensions.LoadJsonSchema", SymbolKindFunction, 31},
			{"FSharp.Data.JsonSchemaExtensions.ValidateJsonAgainstSchema", SymbolKindFunction, 36},
		}, []string{"FSharp.Data.JsonSchemaExtensions.reader", "FSharp.Data.JsonSchemaExtensions.JsonSchema",
			"FSharp.Data.JsonSchemaExtensions.Ghost"}},
		{"fsharp-feliz-build", "SCRIPTHOLD_REAL_FS_FELIZ", "fsharp", []want{
			{"Build", SymbolKindModule, 1},
			{"Build.Constants", SymbolKindModule, 7},
			{"Build.Constants.TestRoot", SymbolKindVariable, 10},
			{"Build.Utility", SymbolKindModule, 28},
			{"Build.Utility.printGreenfn", SymbolKindFunction, 32},
			{"Build.Setup", SymbolKindModule, 49},
			{"Build.Setup.dotnetToolRestore", SymbolKindFunction, 51},
			{"Build.Setup.npmInstall", SymbolKindFunction, 63},
			{"Build.Setup.dotnetRestore", SymbolKindFunction, 74},
		}, []string{"Build.Constants.Utility", "Build.Utility.Setup",
			"Build.Setup.args", "Build.Utility.fmt", "Build.Setup.Ghost"}},
		{"fsharp-fsharpx-option", "SCRIPTHOLD_REAL_FS_FSHARPX", "fsharp", []want{
			{"FSharpx", SymbolKindNamespace, 1},
			{"FSharpx.Option", SymbolKindModule, 3},
			{"FSharpx.Option.MaybeBuilder", SymbolKindType, 10},
			{"FSharpx.Option.MaybeBuilder.Return", SymbolKindMethod, 11},
			{"FSharpx.Option.MaybeBuilder.Bind", SymbolKindMethod, 15},
			{"FSharpx.Option.MaybeBuilder.For", SymbolKindMethod, 41},
			{"FSharpx.Option.maybe", SymbolKindVariable, 44},
			{"FSharpx.Option.monoid", SymbolKindFunction, 47},
			{"FSharpx.Option.returnM", SymbolKindFunction, 60},
		}, []string{"FSharpx.Option.MaybeBuilder.maybe",
			"FSharpx.Option.MaybeBuilder.ReturnFrom.Ghost", "FSharpx.Option.this", "FSharpx.Option.Ghost"}},
		{"fsharp-rezoom-config", "SCRIPTHOLD_REAL_FS_REZOOM", "fsharp", []want{
			{"Rezoom.SQL.Compiler.Config", SymbolKindModule, 1},
			{"Rezoom.SQL.Compiler.Config.toIBackend", SymbolKindFunction, 7},
			{"Rezoom.SQL.Compiler.Config.ConfigOptionalStyle", SymbolKindType, 14},
			{"Rezoom.SQL.Compiler.Config.Config", SymbolKindType, 18},
			{"Rezoom.SQL.Compiler.Config.defaultConfig", SymbolKindVariable, 31},
			{"Rezoom.SQL.Compiler.Config.Parser", SymbolKindModule, 39},
			{"Rezoom.SQL.Compiler.Config.Parser.tws", SymbolKindFunction, 42},
			{"Rezoom.SQL.Compiler.Config.Parser.backend", SymbolKindVariable, 44},
		}, []string{"Rezoom.Parser", "Rezoom.SQL.Compiler.Config.private",
			"Rezoom.SQL.Compiler.Config.Parser.private", "Rezoom.SQL.Compiler.Config.Parser.inline",
			"Rezoom.SQL.Compiler.Config.Ghost"}},
		{"fsharp-websharper-program", "SCRIPTHOLD_REAL_FS_WEBSHARPER", "fsharp", []want{
			{"WebSharper.CSharp.Program", SymbolKindModule, 21},
			{"WebSharper.CSharp.Program.formatArgv", SymbolKindFunction, 31},
			{"WebSharper.CSharp.Program.main", SymbolKindFunction, 37},
		}, []string{"WebSharper.CSharp.Program.C.formatArgv",
			"WebSharper.CSharp.Program.C.main", "WebSharper.CSharp.Program.tryGetMetadata",
			"WebSharper.CSharp.Program.Ghost"}},
		{"fsharp-elmish-ring", "SCRIPTHOLD_REAL_FS_ELMISH", "fsharp", []want{
			{"Elmish", SymbolKindNamespace, 1},
			{"Elmish.RingState", SymbolKindType, 5},
			{"Elmish.RingBuffer", SymbolKindType, 9},
			{"Elmish.RingBuffer.Pop", SymbolKindMethod, 20},
			{"Elmish.RingBuffer.Push", SymbolKindMethod, 33},
		}, []string{"Elmish.internal", "Elmish.RingBuffer.doubleSize",
			"Elmish.RingBuffer.state", "Elmish.RingBuffer.items", "Elmish.RingBuffer.Ghost"}},
		{"fsharp-falco-core", "SCRIPTHOLD_REAL_FS_FALCO", "fsharp", []want{
			{"Falco", SymbolKindNamespace, 1},
			{"Falco.HttpHandler", SymbolKindType, 8},
			{"Falco.HttpHandler", SymbolKindModule, 10},
			{"Falco.HttpHandler.toRequestDelegate", SymbolKindFunction, 12},
			{"Falco.HttpResponseModifier", SymbolKindType, 16},
			{"Falco.HttpVerb", SymbolKindType, 19},
			{"Falco.HttpVerb.ToString", SymbolKindMethod, 30},
		}, []string{"Falco.HttpHandler.HttpResponseModifier", "Falco.HttpHandler.HttpVerb",
			"Falco.RequestDelegate", "Falco.HttpMethods", "Falco.HttpHandler.handler", "Falco.Ghost"}},
		{"autohotkey-windowspy-v2", "SCRIPTHOLD_REAL_AHK_WINDOWSPY", "autohotkey", []want{
			{"WinSpyGui", SymbolKindFunction, 16}, {"WinSpySize", SymbolKindFunction, 60},
			{"WinSpyClose", SymbolKindFunction, 74}, {"Update", SymbolKindFunction, 78},
			{"TryUpdate", SymbolKindFunction, 82}, {"WinGetTextFast", SymbolKindFunction, 191},
			{"UpdateText", SymbolKindFunction, 217}, {"textMangle", SymbolKindFunction, 228},
			{"suspend_timer", SymbolKindFunction, 239},
		}, []string{"TraySetIcon", "Gui", "WinGetPos", "SetTimer", "oGui", "Ghost"}},
		{"autohotkey-mouse-move-v2", "SCRIPTHOLD_REAL_AHK_MOUSE", "autohotkey", []want{
			{"move_5px", SymbolKindFunction, 6},
		}, []string{"MouseGetPos", "MouseMove", "Sleep", "SetTimer", "Ghost"}},
		{"autohotkey-calendar-v2-beta", "SCRIPTHOLD_REAL_AHK_CALENDAR", "autohotkey", []want{
			{"DestroyCalendar", SymbolKindFunction, 27}, {"CreateCalendar", SymbolKindFunction, 39},
			{"GetTitle", SymbolKindFunction, 56}, {"UpdateTitle", SymbolKindFunction, 67},
		}, []string{"Calendar", "FormatTime", "Gui", "SetTimer", "Ghost"}},
		{"autohotkey-ahk2exe-v1-directives-only", "SCRIPTHOLD_REAL_AHK_AHK2EXE", "autohotkey", nil,
			[]string{"Ahk2Exe64", "b", "Ahk2Exe", "Ghost"}},
		{"autohotkey-uia-v2-calls-only", "SCRIPTHOLD_REAL_AHK_UIA", "autohotkey", nil,
			[]string{"UIA", "Run", "WinWaitActive", "ControlGetHwnd", "editEl", "Ghost"}},
		{"blade-bagisto-template-id", "SCRIPTHOLD_REAL_BLADE_BAGISTO", "blade", []want{{"v-two-factor-setup-template", SymbolKindEntity, 10}}, []string{"admin.account.update", "scripts", "v-two-factor-setup", "route", "Ghost"}},
		{"blade-framework-error-sections", "SCRIPTHOLD_REAL_BLADE_FRAMEWORK", "blade", []want{{"title", SymbolKindSection, 3}, {"code", SymbolKindSection, 4}, {"message", SymbolKindSection, 5}}, []string{"minimal", "Unauthorized", "__", "Ghost"}},
		{"blade-laravel-svg-mask-ids", "SCRIPTHOLD_REAL_BLADE_LARAVEL", "blade", []want{{"path-1-mask", SymbolKindEntity, 155}, {"path-2-mask", SymbolKindEntity, 167}, {"path-3-mask", SymbolKindEntity, 179}}, []string{"tailwindcss", "font-sans", "maskUnits", "Ghost"}},
		{"astro-ink-callout", "SCRIPTHOLD_REAL_ASTRO_INK", "astro", []want{{"CalloutType", SymbolKindAlias, 3}, {"Props", SymbolKindInterface, 4}, {"ICON_MAP", SymbolKindConstant, 9}, {"COLOR_MAP", SymbolKindConstant, 16}}, []string{"Icon", "note", "astro-icon", "Astro.props", "callout", "Ghost"}},
		{"astro-paper-breadcrumb", "SCRIPTHOLD_REAL_ASTRO_PAPER", "astro", []want{{"locale", SymbolKindConstant, 7}, {"t", SymbolKindConstant, 8}, {"pathWithoutBase", SymbolKindConstant, 12}, {"currentUrlPath", SymbolKindConstant, 13}, {"breadcrumbList", SymbolKindConstant, 17}, {"decodeSegment", SymbolKindFunction, 19}, {"navLabels", SymbolKindConstant, 27}}, []string{"getRelativeLocaleUrl", "stripBase", "stripLocale", "Number", "Astro.url", "Ghost"}},
		{"astro-starlight-about", "SCRIPTHOLD_REAL_ASTRO_STARLIGHT", "astro", []want{{"Props", SymbolKindInterface, 2}, {"about-astro-heading", SymbolKindEntity, 10}, {"article", SymbolKindSelector, 29}}, []string{"slot", "Astro.props", "max-width", "Ghost"}},
		{"freebasic-framework-sqlite", "SCRIPTHOLD_REAL_FREEBASIC_FRAMEWORK", "freebasic", []want{{"SQLITE_VERSION_NUMBER", SymbolKindConstant, 52}, {"sqlite3_libversion", SymbolKindFunction, 57}, {"sqlite3_sourceid", SymbolKindFunction, 58}, {"sqlite_int64", SymbolKindType, 64}, {"SQLITE_OK", SymbolKindConstant, 74}}, []string{"sqlite3_version", "zOptName", "Ghost"}},
		{"freebasic-llama2-model", "SCRIPTHOLD_REAL_FREEBASIC_LLAMA", "freebasic", []want{{"CONFIG_DIM_4B", SymbolKindVariable, 24}, {"CONFIG_HIDDEN_DIM_4B", SymbolKindVariable, 25}, {"TransformerWeights_WQ_f4B", SymbolKindVariable, 40}}, []string{"crt/stdio.bi", "DIM", "TransformerWeights_Ghost"}},
		{"freebasic-raylib-binding", "SCRIPTHOLD_REAL_FREEBASIC_RAYLIB", "freebasic", []want{{"RAYLIB_VERSION_MAJOR", SymbolKindConstant, 37}, {"Vector2", SymbolKindType, 88}, {"Vector2.x", SymbolKindField, 89}, {"Vector3", SymbolKindType, 105}, {"RLColor", SymbolKindType, 169}}, []string{"RL_MALLOC", "Vector2.Ghost", "RLColor.Ghost"}},
		{"freebasic-tiko-editor", "SCRIPTHOLD_REAL_FREEBASIC_TIKO", "freebasic", []want{{"clsApp", SymbolKindType, 16}, {"clsApp.pDocList", SymbolKindField, 20}, {"clsApp.IsValidDocumentPointer", SymbolKindFunction, 68}, {"clsApp.CreateEmptyDocument", SymbolKindFunction, 70}}, []string{"clsDocument", "clsApp.Ghost", "clsApp.False"}},
		{"freebasic-visualfbeditor-mariadb", "SCRIPTHOLD_REAL_FREEBASIC_VISUAL", "freebasic", []want{{"MariaDBBox", SymbolKindType, 13}, {"MariaDBBox.ErrStr", SymbolKindField, 15}, {"MariaDBBox.Event_Send", SymbolKindFunction, 23}, {"MariaDBBox.Open", SymbolKindFunction, 28}}, []string{"Component", "MariaDBBox.Ghost", "MARIADB_PORT"}},
		{"al-bcapps-intrastat", "SCRIPTHOLD_REAL_AL_BCAPPS", "al", []want{{"Microsoft.Inventory.Intrastat", SymbolKindNamespace, 5}, {"Microsoft.Inventory.Intrastat.IntrastatReportATUpgrade", SymbolKindModule, 11}, {"Microsoft.Inventory.Intrastat.IntrastatReportATUpgrade.OnUpgradePerCompany", SymbolKindMethod, 15}, {"Microsoft.Inventory.Intrastat.IntrastatReportATUpgrade.UpdateDefaultDataExchangeDef", SymbolKindMethod, 20}}, []string{"UpgradeTag.HasUpgradeTag", "DataExchDef.Get", "IntrastatReportATUpgrade.Ghost"}},
		{"al-powersearch-engine", "SCRIPTHOLD_REAL_AL_POWER", "al", []want{{"PowerSearch.BusinessCentral", SymbolKindNamespace, 1}, {"PowerSearch.BusinessCentral.PowerSearch Engine", SymbolKindModule, 110}, {"PowerSearch.BusinessCentral.PowerSearch Engine.Search", SymbolKindMethod, 115}, {"PowerSearch.BusinessCentral.PowerSearch Engine.SearchShortQueryRecords", SymbolKindMethod, 196}, {"PowerSearch.BusinessCentral.PowerSearch Engine.TryGetSetup", SymbolKindMethod, 209}, {"PowerSearch.BusinessCentral.PowerSearch Engine.SearchCustomers", SymbolKindMethod, 301}}, []string{"PowerSearch.BusinessCentral.Ghost", "PowerSearch.BusinessCentral.PowerSearch Engine.Query", "Microsoft.Assembly.Document"}},
		{"al-sticky-note-manager", "SCRIPTHOLD_REAL_AL_STICKY", "al", []want{{"DefaultPublisher.StickyNoteNotes", SymbolKindNamespace, 1}, {"DefaultPublisher.StickyNoteNotes.SN Note Manager", SymbolKindModule, 19}, {"DefaultPublisher.StickyNoteNotes.SN Note Manager.GetActiveNotesJson", SymbolKindMethod, 25}}, []string{"Note.SetRange", "DefaultPublisher.StickyNoteNotes.Ghost", "DefaultPublisher.StickyNoteNotes.SN Note Manager.NowDT"}},
		{"al-thingsstack-api-key", "SCRIPTHOLD_REAL_AL_THINGS", "al", []want{{"obc TTS Demo API Key", SymbolKindType, 1}, {"obc TTS Demo API Key.No.", SymbolKindField, 9}, {"obc TTS Demo API Key.Description", SymbolKindField, 14}, {"obc TTS Demo API Key.API Key", SymbolKindField, 20}, {"obc TTS Demo API Key.OnDelete", SymbolKindMethod, 34}}, []string{"obc TTS Demo API Key.LookupPageId", "RemoveAPIKey", "obc TTS Demo API Key.Ghost"}},
		{"al-whatsapp-install", "SCRIPTHOLD_REAL_AL_WHATSAPP", "al", []want{{"obc WA Test Install", SymbolKindModule, 1}, {"obc WA Test Install.OnInstallAppPerCompany", SymbolKindMethod, 5}, {"obc WA Test Install.OnInstallAppPerDatabase", SymbolKindMethod, 10}}, []string{"obc WA Test Install.Subtype", "OnRun", "Install"}},
		{"delphi-delphimvc", "SCRIPTHOLD_REAL_DELPHI_DMVC", "delphi", []want{{"renders_spring4d_nullables", SymbolKindModule, 1}, {"renders_spring4d_nullables.RunServer", SymbolKindFunction, 21}}, []string{"Writeln", "ParamStr", "TIdHTTPWebBrokerBridge.Create"}},
		{"delphi-horse", "SCRIPTHOLD_REAL_DELPHI_HORSE", "delphi", []want{{"Apache", SymbolKindModule, 1}, {"Apache.ApacheModuleData", SymbolKindVariable, 26}}, []string{"THorse.Listen", "THorse.Get", "Ghost"}},
		{"delphi-mormot2", "SCRIPTHOLD_REAL_DELPHI_MORMOT", "delphi", []want{{"BasicdemoServer", SymbolKindModule, 1}, {"BasicdemoServer.srv", SymbolKindVariable, 22}}, []string{"TBasicDemoServer.Create", "TSynLog.Add", "WriteLn"}},
		{"delphi-neon", "SCRIPTHOLD_REAL_DELPHI_NEON", "delphi", []want{{"Benchmarks", SymbolKindModule, 10}}, []string{"Application.Run", "Application.Initialize", "TfrmBenchmarks"}},
		{"delphi-pasclaw", "SCRIPTHOLD_REAL_DELPHI_PASCLAW", "delphi", []want{{"PasclawDesktop", SymbolKindModule, 1}}, []string{"Application.CreateForm", "FindStyleDir", "FMX.RetroSkins"}},
		{"delphi-tdlib", "SCRIPTHOLD_REAL_DELPHI_TDLIB", "delphi", []want{{"testando", SymbolKindModule, 1}}, []string{"Application.MainFormOnTaskbar", "XSuperJSON", "Ghost"}},
		{"delphi-thread-demo", "SCRIPTHOLD_REAL_DELPHI_THREAD", "delphi", []want{{"ThreadDemo", SymbolKindModule, 1}}, []string{"Application.Run", "Vcl.Themes", "TStyleManager.TrySetStyle"}},
		{"plsql-alexandria", "SCRIPTHOLD_REAL_PLSQL_ALEX", "plsql", []want{{"hash_util_pkg", SymbolKindPackage, 1}, {"hash_util_pkg.sha1", SymbolKindFunction, 29}, {"hash_util_pkg.unittest", SymbolKindFunction, 46}}, []string{"hash_util_pkg.p_buffer", "DBMS_CRYPTO", "Ghost"}},
		{"plsql-oracle-examples", "SCRIPTHOLD_REAL_PLSQL_EXAMPLE", "plsql", []want{{"sectionutils", SymbolKindPackage, 1}, {"sectionutils.cleanup_sections", SymbolKindFunction, 2}}, []string{"sectionutils.sections", "sectionutils.replace", "show"}},
		{"plsql-oxar", "SCRIPTHOLD_REAL_PLSQL_OXAR", "plsql", []want{{"aop_sample3_pkg", SymbolKindPackage, 3}, {"aop_sample3_pkg.aop_store_document", SymbolKindFunction, 14}, {"aop_sample3_pkg.get_file", SymbolKindFunction, 46}}, []string{"aop_sample3_pkg.p_output_blob", "Ghost", "aop_sample3_pkg.REST"}},
		{"plsql-utplsql", "SCRIPTHOLD_REAL_PLSQL_UT", "plsql", []want{{"utplsql", SymbolKindPackage, 1}, {"utplsql.global_setup", SymbolKindFunction, 6}}, []string{"utplsql.beforeall", "Ghost", "utplsql.suite"}},
		{"plsql-wrapping-gist", "SCRIPTHOLD_REAL_PLSQL_WRAP", "plsql", []want{{"HELLO_PKG", SymbolKindPackage, 10}, {"HELLO_PKG.hello", SymbolKindFunction, 12}}, []string{"HELLO_PKG.Nate", "dbms_ddl.create_wrapped", "Ghost"}},
		{"webforms-vcsamples-upload", "SCRIPTHOLD_REAL_WEBFORMS_A", "aspnet-webforms", []want{
			{"Page_Load", SymbolKindMethod, 6},
		}, []string{"Request.Headers", "fileName", "Request.InputStream", "Page_Load.Ghost"}},
		{"classic-asp-logger", "SCRIPTHOLD_REAL_CLASSICASP_A", "classic-asp", []want{
			{"logger", SymbolKindClass, 11},
			{"logger.log", SymbolKindMethod, 33}, {"logger.setLogFile", SymbolKindMethod, 44},
		}, []string{"logger.Ghost", "server.createObject", "logger.p_string"}},
		{"jscript-net-snu-hello", "SCRIPTHOLD_REAL_JSNET_A", "jscript-net", []want{
			{"_name", SymbolKindVariable, 6},
		}, []string{"Console.ReadLine", "Console.WriteLine", "Hello"}},
		{"vbnet-svb-arrow-bom", "SCRIPTHOLD_REAL_VBNET_A", "vbnet", []want{
			{"Arrow", SymbolKindClass, 1}, {"Arrow.Angle", SymbolKindProperty, 3},
			{"Arrow.AngleProperty", SymbolKindField, 15},
		}, []string{"Arrow.RotateTransform", "Angle.GetValue", "Arrow.Ghost"}},
		{"c-curl-altsvc", "SCRIPTHOLD_REAL_C_A", "c", []want{
			{"main", SymbolKindFunction, 32},
		}, []string{"curl_easy_init", "curl_easy_setopt", "curl_global_cleanup"}},
		{"groovy-benchmark-ackermann", "SCRIPTHOLD_REAL_GROOVY_A", "groovy", []want{
			{"A", SymbolKindFunction, 8},
		}, []string{"println", "args", "Ghost"}},
		{"groovy-exercism-hello", "SCRIPTHOLD_REAL_GROOVY_B", "groovy", []want{
			{"HelloWorld", SymbolKindClass, 1}, {"HelloWorld.hello", SymbolKindMethod, 3},
		}, []string{"HelloWorld.World", "String"}},
		{"gleam-echo-original", "SCRIPTHOLD_REAL_GLEAM_A", "gleam", []want{
			{"main", SymbolKindFunction, 4},
		}, []string{"echo", "Gleam", "ghost"}},
		{"common-lisp-clog-demo", "SCRIPTHOLD_REAL_COMMONLISP_A", "common-lisp", []want{
			{"clog-demo-1", SymbolKindPackage, 1},
			{"clog-demo-1.display-width", SymbolKindConstant, 11},
			{"clog-demo-1.new-food", SymbolKindFunction, 20},
		}, []string{"create-div", "defpackage", "clog-demo-1.ghost"}},
		{"vbscript-reactos-wscript", "SCRIPTHOLD_REAL_VBSCRIPT_A", "vbscript", []want{
			{"winetest", SymbolKindVariable, 21}, {"ok", SymbolKindFunction, 24},
		}, []string{"CreateObject", "winetest.ok", "reportSuccess"}},
		{"fortran-stdlib-meshgrid", "SCRIPTHOLD_REAL_FORTRAN_A", "fortran", []want{
			{"example_meshgrid", SymbolKindModule, 1},
			{"example_meshgrid.print_2d_array", SymbolKindFunction, 29},
		}, []string{"meshgrid", "linspace", "example_meshgrid.print"}},
		{"emacs-mal-core", "SCRIPTHOLD_REAL_EMACS_A", "emacs-lisp", []want{
			{"mal-boolean", SymbolKindFunction, 4}, {"mal-=", SymbolKindFunction, 6},
			{"mal-map-=", SymbolKindFunction, 29}, {"mal-conj", SymbolKindFunction, 44},
			{"elisp-to-mal", SymbolKindFunction, 55},
		}, []string{"seq", "mal-true", "mal/types", "mal.Ghost"}},
		{"flow-project-ambient-declaration", "SCRIPTHOLD_REAL_FLOW_A", "flow", []want{
			{"ccc", SymbolKindConstant, 3},
		}, []string{"Array", "string", "haste_pkg"}},
		{"zig-stdlib-wasm-object", "SCRIPTHOLD_REAL_ZIG_A", "zig", []want{
			{"Object", SymbolKindConstant, 1},
			{"RelativeSlice", SymbolKindStruct, 53}, {"RelativeSlice.off", SymbolKindField, 54},
			{"RelativeSlice.len", SymbolKindField, 55},
			{"SegmentInfo", SymbolKindStruct, 58},
		}, []string{"Wasm.Ghost", "std.Ghost", "pub", "SegmentInfo.Ghost"}},
		{"ocaml-stdlib-option", "SCRIPTHOLD_REAL_OCAML_A", "ocaml", []want{
			{"none", SymbolKindVariable, 18}, {"some", SymbolKindFunction, 19},
			{"Syntax", SymbolKindModule, 64},
		}, []string{"Option.Ghost", "None"}},
		{"ocaml-exercism-connect", "SCRIPTHOLD_REAL_OCAML_B", "ocaml", []want{
			{"eq_cell", SymbolKindFunction, 6}, {"IntTuple", SymbolKindModule, 11},
			{"to_matrix", SymbolKindFunction, 28},
		}, []string{"Phantom", "IntTuple.Ghost"}},
		{"erlang-recon-docsite", "SCRIPTHOLD_REAL_ERLANG_A", "erlang", []want{
			{"docsite", SymbolKindModule, 9}, {"docsite.main", SymbolKindFunction, 15},
			{"docsite.content", SymbolKindFunction, 39}, {"docsite.write", SymbolKindFunction, 109},
		}, []string{"docsite.rebar3", "docsite.filename"}},
		{"erlang-yaws-negotiation", "SCRIPTHOLD_REAL_ERLANG_B", "erlang", []want{
			{"yaws_content_negotiation", SymbolKindModule, 1},
			{"yaws_content_negotiation.compute_rsva", SymbolKindFunction, 9},
			{"yaws_content_negotiation.get_other_header", SymbolKindFunction, 30},
		}, []string{"yaws_content_negotiation.ghost", "yaws_content_negotiation.Headers"}},
		{"octave-polynomial-double", "SCRIPTHOLD_REAL_OCTAVE_A", "octave", []want{
			{"double", SymbolKindFunction, 1},
		}, []string{"poly", "p"}},
		{"qbasic-qb64pe-acalc", "SCRIPTHOLD_REAL_QBASIC_A", "qbasic", []want{
			{"DrawCursor", SymbolKindFunction, 2}, {"DrawCursor", SymbolKindFunction, 52},
			{"DrawInner", SymbolKindFunction, 69}, {"DrawOuter", SymbolKindFunction, 124},
			{"OldX", SymbolKindVariable, 7}, {"OldY", SymbolKindVariable, 7},
		}, []string{"SIN", "LOG", "OldAngle.Ghost"}},
		{"systemverilog-yosys-example", "SCRIPTHOLD_REAL_SYSTEMVERILOG_A", "systemverilog", []want{
			{"top", SymbolKindModule, 1},
		}, []string{"assume", "assert", "top.initial"}},
		{"systemverilog-pygments-flipflop", "SCRIPTHOLD_REAL_SYSTEMVERILOG_B", "systemverilog", []want{
			{"toplevel", SymbolKindModule, 1}, {"toplevel.flop1", SymbolKindSignal, 5},
			{"toplevel.flop2", SymbolKindSignal, 6},
		}, []string{"toplevel.reset", "posedge"}},
		{"nim-stdlib-atomic", "SCRIPTHOLD_REAL_NIM_A", "nim", []want{
			{"AtomicWithGeneric", SymbolKindClass, 4}, {"AtomicWithGeneric.value", SymbolKindField, 5},
			{"initAtomicWithGeneric", SymbolKindFunction, 7},
		}, []string{"atomics", "result"}},
		{"nim-exercism-hello", "SCRIPTHOLD_REAL_NIM_B", "nim", []want{
			{"hello", SymbolKindFunction, 1},
		}, []string{"World", "string"}},
		{"julia-stdlib-buffer", "SCRIPTHOLD_REAL_JULIA_A", "julia", []want{
			{"Buffer", SymbolKindStruct, 4}, {"read_to_buffer", SymbolKindFunction, 29},
		}, []string{"UInt8", "Buffer.Ghost"}},
		{"julia-exercism-tree", "SCRIPTHOLD_REAL_JULIA_B", "julia", []want{
			{"BinarySearchTree", SymbolKindStruct, 1}, {"traverse", SymbolKindFunction, 44},
		}, []string{"nothing", "vector"}},
		{"css-sphinx-design-overrides", "SCRIPTHOLD_REAL_CSS_A", "css", []want{
			{".sd-card-header", SymbolKindSelector, 2},
		}, []string{"font-size", "Audiowide"}},
		{"css-esp-ebus-common", "SCRIPTHOLD_REAL_CSS_B", "css", []want{
			{"body.centered", SymbolKindSelector, 20}, {".container", SymbolKindSelector, 24},
			{".row", SymbolKindSelector, 28},
		}, []string{"font-family", "padding", "#fafbfc"}},
		{"terraform-core-check", "SCRIPTHOLD_REAL_TERRAFORM_A", "terraform", []want{
			{"test_instance.foo", SymbolKindResource, 1}, {"check.test_instance_exists", SymbolKindSection, 5},
		}, []string{"bar", "check.test_instance_exists.foo"}},
		{"terraform-aws-vpc", "SCRIPTHOLD_REAL_TERRAFORM_B", "terraform", []want{
			{"terraform", SymbolKindSection, 1}, {"terraform.required_providers", SymbolKindSection, 4},
			{"terraform.provider_meta.aws", SymbolKindSection, 11},
		}, []string{"hashicorp/aws", "terraform.source"}},
		{"xaml-abp-app", "SCRIPTHOLD_REAL_XAML_A", "xaml", []want{
			{"MyCompanyName.MyProjectName.App", SymbolKindClass, 5},
		}, []string{"MyCompanyName.MyProjectName.App.Resources", "Resources/Styles/Colors.xaml"}},
		{"xaml-desktop-toasts", "SCRIPTHOLD_REAL_XAML_B", "xaml", []want{
			{"DesktopToastsSample.App", SymbolKindClass, 1},
		}, []string{"DesktopToastsSample.MainWindow", "StartupUri"}},
		{"ansible-windows-ping", "SCRIPTHOLD_REAL_ANSIBLE_A", "ansible-yaml", []want{
			{"Ping", SymbolKindSection, 3}, {"Ping.ping", SymbolKindOperation, 7},
		}, []string{"Ping.win_ping", "all"}},
		{"svelte-custom-method", "SCRIPTHOLD_REAL_SVELTE_A", "svelte", []want{
			{"updateFoo", SymbolKindFunction, 4}, {"foo", SymbolKindVariable, 8},
		}, []string{"value", "custom-element"}},
		{"scala3-compiler-main", "SCRIPTHOLD_REAL_SCALA_A", "scala", []want{
			{"dotty.tools.dotc.Main", SymbolKindModule, 5},
		}, []string{"dotty.tools.dotc.Driver", "dotty.tools.dotc.class"}},
		{"verilog-yosys-key-expansion", "SCRIPTHOLD_REAL_VERILOG_A", "verilog", []want{
			{"aes_key_expand_128", SymbolKindModule, 4}, {"aes_key_expand_128.w", SymbolKindSignal, 9},
		}, []string{"aes_key_expand_128.assign", "aes_key_expand_128.kld_ref"}},
		{"elixir-atom-standard-library", "SCRIPTHOLD_REAL_ELIXIR_A", "elixir", []want{
			{"Atom", SymbolKindModule, 5}, {"Atom.to_string", SymbolKindFunction, 62},
			{"Atom.to_charlist", SymbolKindFunction, 78}, {"Atom.to_char_list", SymbolKindFunction, 85},
		}, []string{"Atom.apple", "Atom.ghost"}},
		{"graphql-gqlgen", "SCRIPTHOLD_REAL_GRAPHQL_A", "graphql", []want{
			{"Role", SymbolKindEnum, 8}, {"MyQuery", SymbolKindType, 18}, {"MyQuery.todo", SymbolKindField, 19},
		}, []string{"forceResolver"}},
		{"graphql-hasura", "SCRIPTHOLD_REAL_GRAPHQL_B", "graphql", []want{
			{"Mutation", SymbolKindType, 1}, {"Mutation.actionName", SymbolKindField, 2}, {"SampleInput", SymbolKindType, 7},
		}, []string{"arg1"}},
		{"proto-addressbook", "SCRIPTHOLD_REAL_PROTO_A", "proto", []want{
			{"tutorial.Person", SymbolKindType, 6}, {"tutorial.Person.name", SymbolKindField, 7},
		}, []string{"AddressBookProtos"}},
		{"proto-descriptor", "SCRIPTHOLD_REAL_PROTO_B", "proto", []want{
			{"test.MyMessage", SymbolKindType, 33}, {"test.MyMessage.foo", SymbolKindField, 36},
		}, []string{"optimize_for"}},
		{"sass-timeline", "SCRIPTHOLD_REAL_SASS_A", "sass", []want{
			{"primary", SymbolKindVariable, 1}, {"body", SymbolKindSelector, 13},
		}, []string{"background-color"}},
		{"sass-rem", "SCRIPTHOLD_REAL_SASS_B", "sass", []want{
			{"browser-context", SymbolKindVariable, 2}, {"rem", SymbolKindFunction, 3},
		}, []string{"unitless"}},
		{"al-sales", "SCRIPTHOLD_REAL_AL_A", "al", []want{
			{"SalesDataMgmt", SymbolKindModule, 15}, {"SalesDataMgmt.GetSalesJSON", SymbolKindMethod, 19},
		}, []string{"CustLedgEntry"}},
		{"al-csv", "SCRIPTHOLD_REAL_AL_B", "al", []want{
			{"BIT CSV TXT Process Lines", SymbolKindModule, 1}, {"BIT CSV TXT Process Lines.OnRun", SymbolKindMethod, 12},
		}, []string{"DataExch"}},
		{"vue-header", "SCRIPTHOLD_REAL_VUE_A", "vue", []want{
			{"props", SymbolKindConstant, 13}, {"emit", SymbolKindConstant, 20},
		}, []string{"ReplStore"}},
		{"vue-nuxt", "SCRIPTHOLD_REAL_VUE_B", "vue", []want{
			{"props", SymbolKindConstant, 9}, {".grid", SymbolKindSelector, 59},
		}, []string{"useHead"}},
		{"r-sample", "SCRIPTHOLD_REAL_R_A", "r", []want{
			{"Square", SymbolKindFunction, 18}, {"countdown", SymbolKindFunction, 28},
		}, []string{"print"}},
		{"r-analysis", "SCRIPTHOLD_REAL_R_B", "r", []want{
			{"gtable_select", SymbolKindFunction, 9}, {"gtable_stack", SymbolKindFunction, 17},
		}, []string{"library"}},
		{"pascal-lsp", "SCRIPTHOLD_REAL_PASCAL_A", "pascal", []want{
			{"LSP.Base", SymbolKindModule, 21}, {"LSP.Base.TLSPContextCustomJSONRPCHandler", SymbolKindClass, 49},
		}, []string{"LSP.Base.TGhost"}},
		{"pascal-brook", "SCRIPTHOLD_REAL_PASCAL_B", "pascal", []want{
			{"BrookFCLCGIBroker", SymbolKindModule, 16}, {"BrookFCLCGIBroker.TBrookApplication", SymbolKindClass, 32},
		}, []string{"BrookFCLCGIBroker.TGhost"}},
		{"powershell-class", "SCRIPTHOLD_REAL_POWERSHELL_A", "powershell", []want{
			{"Foo", SymbolKindClass, 13}, {"Foo.Method", SymbolKindMethod, 16}, {"GetBar", SymbolKindFunction, 22},
		}, []string{"Foo.LocalVar1"}},
		{"powershell-history", "SCRIPTHOLD_REAL_POWERSHELL_B", "powershell", []want{
			{"Save-HistoryAll", SymbolKindFunction, 1}, {"Save-HistoryIncremental", SymbolKindFunction, 9}, {"Search-History", SymbolKindFunction, 44},
		}, []string{"Save-History"}},
		{"perl-package", "SCRIPTHOLD_REAL_PERL_A", "perl", []want{
			{"Test::One", SymbolKindModule, 8}, {"Test::One::Two", SymbolKindModule, 10}, {"Test", SymbolKindModule, 12},
		}, []string{"Test.PodGhost"}},
		{"perl-ack", "SCRIPTHOLD_REAL_PERL_B", "perl", []want{
			{"App::Ack", SymbolKindModule, 1},
		}, []string{"App::Ack.Ghost"}},
		{"mql5-oscillator", "SCRIPTHOLD_REAL_MQL5_A", "mql5", []want{
			{"ENUM_AOS_BI", SymbolKindEnum, 17}, {"AosPeriod", SymbolKindVariable, 24},
		}, []string{"Indicator"}},
		{"mql5-argus", "SCRIPTHOLD_REAL_MQL5_B", "mql5", []want{
			{"ADX_Period", SymbolKindVariable, 23}, {"OnInit", SymbolKindFunction, 48},
		}, []string{"ADX Regime"}},
		{"vb6-modcode", "SCRIPTHOLD_REAL_VB6_A", "vb6", []want{
			{"modCode", SymbolKindModule, 1}, {"modCode.progname", SymbolKindConstant, 6}, {"modCode.currentState", SymbolKindEnum, 10},
		}, []string{"modCode.VB_Name"}},
		{"vb6-module", "SCRIPTHOLD_REAL_VB6_B", "vb6", []want{
			{"Module1", SymbolKindModule, 1}, {"Module1.SC_CLOSE", SymbolKindConstant, 4},
		}, []string{"Module1.VB_Name"}},
		{"ada-expression", "SCRIPTHOLD_REAL_ADA_A", "ada", []want{
			{"Test", SymbolKindFunction, 3}, {"Test.Tagged_Procedure", SymbolKindFunction, 7}, {"Test.Tagged_Procedure.Boolean_As_String", SymbolKindFunction, 8},
		}, []string{"Test.Tagged_Procedure.dummy0", "Test.Tagged_Procedure.dummy1"}},
		{"ada-kde", "SCRIPTHOLD_REAL_ADA_B", "ada", []want{
			{"LAL_DDA", SymbolKindFunction, 5}, {"LAL_DDA.App_Setup", SymbolKindFunction, 57},
		}, []string{"LAL_DDA.Fake"}},
		{"clojure-for", "SCRIPTHOLD_REAL_CLOJURE_A", "clojure", []want{
			{"prime?", SymbolKindFunction, 1},
		}, []string{"for"}},
		{"clojure-highlight", "SCRIPTHOLD_REAL_CLOJURE_B", "clojure", []want{
			{"highlighting", SymbolKindNamespace, 2}, {"highlighting.something-else", SymbolKindFunction, 6}, {"highlighting.bar", SymbolKindFunction, 21},
		}, []string{"highlighting.Ghost"}},
		{"clojure-ctags-metadata", "SCRIPTHOLD_REAL_CLOJURE_META", "clojure", []want{
			{"my-fn", SymbolKindFunction, 1}, {"my-private-fn", SymbolKindFunction, 3},
			{"my-public-fn", SymbolKindFunction, 5}, {"my-hinted-fn", SymbolKindFunction, 7},
			{"my-private-hinted-fn", SymbolKindFunction, 9}, {"so-many-hints", SymbolKindFunction, 11},
		}, []string{"private", "foo", "MyReturnType"}},
		{"tcl-ctags-regex", "SCRIPTHOLD_REAL_TCL_A", "tcl", []want{
			{"proc1", SymbolKindFunction, 2}, {"proc2", SymbolKindFunction, 8},
			{"proc3", SymbolKindFunction, 10}, {"proc4", SymbolKindFunction, 15},
		}, []string{"regexp"}},
		{"tcl-sample-even-odd", "SCRIPTHOLD_REAL_TCL_B", "tcl", []want{
			{"usage", SymbolKindFunction, 1},
		}, []string{"lindex", "regexp", "expr"}},
		{"ada-ctags-block-scope", "SCRIPTHOLD_REAL_ADA_BLOCK", "ada", []want{
			{"Input", SymbolKindPackage, 1}, {"Input.F", SymbolKindFunction, 2}, {"Input.G", SymbolKindFunction, 23},
		}, []string{"Input.F.G", "Input.F.P", "Input.G.D"}},
		{"ada-ctags-altivec", "SCRIPTHOLD_REAL_ADA_ALTIVEC", "ada", []want{
			{"Altivec", SymbolKindFunction, 14},
		}, []string{"Altivec.To_Vector", "Altivec.View_A"}},
		{"lua-ctags-nested-table", "SCRIPTHOLD_REAL_LUA_A", "lua", []want{
			{"a.b.c", SymbolKindFunction, 3}, {"ax.by.cz", SymbolKindFunction, 10},
		}, []string{"a.b", "ax.by", "print"}},
		{"lua-kde-highlight", "SCRIPTHOLD_REAL_LUA_B", "lua", []want{
			{"obj:foo", SymbolKindFunction, 172}, {"obj.bar", SymbolKindFunction, 176},
		}, []string{"string.gfind", "string.gmatch", "xyz"}},
		{"ruby-ctags-method-body", "SCRIPTHOLD_REAL_RUBY_A", "ruby", []want{
			{"A", SymbolKindClass, 1}, {"A.a", SymbolKindMethod, 2}, {"A.b", SymbolKindMethod, 5},
		}, []string{"super", "A.super", "do"}},
		{"ruby-kde-class", "SCRIPTHOLD_REAL_RUBY_B", "ruby", []want{
			{"Periods", SymbolKindClass, 150}, {"Periods.each", SymbolKindMethod, 151},
			{"ClientManager", SymbolKindClass, 205}, {"ClientManager.ClientManager", SymbolKindConstructor, 208},
		}, []string{"dumpDebugInfo", "Periods.Classical", "ClientManager.DBClient"}},
		{"php-ctags-classes", "SCRIPTHOLD_REAL_PHP_A", "php", []want{
			{"Foo", SymbolKindClass, 16}, {"Foo.Foo", SymbolKindConstructor, 17},
			{"Foo.method1", SymbolKindMethod, 21}, {"Bar", SymbolKindClass, 30},
			{"Bar.Bar", SymbolKindConstructor, 31}, {"Bar.method1", SymbolKindMethod, 35},
		}, []string{"Bar.method2", "functions", "classes"}},
		{"php-kde-mixed", "SCRIPTHOLD_REAL_PHP_B", "php", []want{
			{"test", SymbolKindFunction, 9}, {"gen_three_nulls", SymbolKindFunction, 141},
			{"User", SymbolKindClass, 168}, {"User.User", SymbolKindConstructor, 173},
			{"MyClass", SymbolKindClass, 180},
		}, []string{"DateTime", "DateTime.get", "some_js_var", "JSFake"}},
		{"rust-ctags-circle", "SCRIPTHOLD_REAL_RUST_A", "rust", []want{
			{"main", SymbolKindFunction, 4}, {"Circle.x", SymbolKindField, 8},
			{"Circle.radius", SymbolKindField, 10}, {"Circle.area", SymbolKindMethod, 14},
		}, []string{"println", "Circle.println", "Ghost"}},
		{"rust-sample-even-odd", "SCRIPTHOLD_REAL_RUST_B", "rust", []want{
			{"usage", SymbolKindFunction, 8}, {"parse_int", SymbolKindFunction, 13},
			{"main", SymbolKindFunction, 17},
		}, []string{"println", "FromStr", "parse"}},
		{"toml-kde-highlighting", "SCRIPTHOLD_REAL_TOML_A", "toml", []want{
			{"baseURL", SymbolKindKey, 2}, {"Author", SymbolKindSection, 5},
			{"Author.name", SymbolKindKey, 6}, {"outputs.home", SymbolKindKey, 10},
			{"params", SymbolKindSection, 12}, {"params.social", SymbolKindSection, 28},
			{"params.social.name", SymbolKindKey, 29},
			{"params.social.'key2'", SymbolKindKey, 36},
			{"params.social.'quoted \"value\"'", SymbolKindKey, 37},
			{"target.'cfg(all(windows, target_env=\"msvc\"))'.build-dependencies", SymbolKindSection, 40},
			{"target.'cfg(all(windows, target_env=\"msvc\"))'.build-dependencies.fruit.name", SymbolKindKey, 47},
			{"target.'cfg(all(windows, target_env=\"msvc\"))'.build-dependencies.fruit.color", SymbolKindKey, 48},
			{"target.'cfg(all(windows, target_env=\"msvc\"))'.build-dependencies.fruit.flavor", SymbolKindKey, 49},
			{"target.'cfg(all(windows, target_env=\"msvc\"))'.build-dependencies.site.\"google.com\"", SymbolKindKey, 50},
		}, []string{"params . social", "params . social.name", "phantom",
			"target.'cfg(all(windows, target_env=\"msvc\"))'.build-dependencies.fruit. color",
			"target.'cfg(all(windows, target_env=\"msvc\"))'.build-dependencies.site.google.com"}},
		{"yaml-kde-highlighting", "SCRIPTHOLD_REAL_YAML_A", "yaml", []want{
			{"key", SymbolKindKey, 5}, {"boolean", SymbolKindKey, 9},
			{"null_value", SymbolKindKey, 10}, {"single quotes", SymbolKindKey, 11},
			{"double quotes", SymbolKindKey, 12}, {"decimal", SymbolKindKey, 14},
			{"foo", SymbolKindKey, 42},
		}, []string{"comment", "string", "O'Brien", "syntax", "phantom"}},
		{"yaml-ctags-openapi", "SCRIPTHOLD_REAL_YAML_B", "yaml", []want{
			{"openapi", SymbolKindKey, 1}, {"info", SymbolKindKey, 2},
			{"info.title", SymbolKindKey, 3}, {"info.version", SymbolKindKey, 5},
			{"paths", SymbolKindKey, 11}, {"paths./sample/path", SymbolKindKey, 12},
			{"paths./sample/path.get", SymbolKindKey, 13},
			{"paths./sample/path.get.summary", SymbolKindKey, 14},
			{"components.schemas.NullableField", SymbolKindKey, 26},
		}, []string{"servers.url", "servers.description", "tags.name", "fake"}},
		{"swift-kde-arguments", "SCRIPTHOLD_REAL_SWIFT_A", "swift", []want{
			{"Options.maxArgumentNameLength", SymbolKindProperty, 35},
			{"Options.addArguments", SymbolKindMethod, 45},
			{"parseArguments", SymbolKindFunction, 58},
			{"preprocessArguments", SymbolKindFunction, 98},
			{"Foo", SymbolKindInterface, 689},
			{"Foo.foo", SymbolKindMethod, 690},
			{"FooImpl", SymbolKindClass, 695},
		}, []string{"UnicodeScalarView", "FormatError", "FooImpl.foo", "popFirst"}},
		{"swift-sample-even-odd", "SCRIPTHOLD_REAL_SWIFT_B", "swift", []want{
			{"is_even", SymbolKindVariable, 13},
		}, []string{"print", "Even", "Odd", "System.exit"}},
		{"nix-kde-bindings", "SCRIPTHOLD_REAL_NIX_A", "nix", []want{
			{"bool", SymbolKindVariable, 4}, {"var1", SymbolKindVariable, 5},
		}, []string{"false", "Multi-line", "HOME", "builtins"}},
		{"cobol-linguist-hello", "SCRIPTHOLD_REAL_COBOL_LINGUIST", "cobol", []want{
			{"hello", SymbolKindModule, 2},
		}, []string{"DISPLAY", "Hello World!", "ghost"}},
		{"cobol-geany-program", "SCRIPTHOLD_REAL_COBOL_GEANY", "cobol", []want{
			{"Program-Name", SymbolKindModule, 2},
			{"Program-Name.INPUT-OUPUT", SymbolKindSection, 6},
			{"Program-Name.FILE", SymbolKindSection, 12},
			{"Program-Name.WORKING-STORAGE", SymbolKindSection, 17},
			{"Program-Name.Begin", SymbolKindFunction, 27},
			{"Program-Name.Subprogram-Name", SymbolKindFunction, 30},
		}, []string{"STOP", "SAMPLE.DAT", "ZEROS", "ghost"}},
		{"cobol-pygments-ocic", "SCRIPTHOLD_REAL_COBOL_PYGMENTS", "cobol", []want{
			{"OCic", SymbolKindModule, 2},
		}, []string{"OpenCOBOL", "Compile", "ghost"}},
		{"cobol-sample-even-odd", "SCRIPTHOLD_REAL_COBOL_SAMPLE", "cobol", []want{
			{"even-odd", SymbolKindModule, 2},
			{"even-odd.working-storage", SymbolKindSection, 5},
			{"even-odd.main", SymbolKindFunction, 12},
		}, []string{"test-numval", "cmdargs.from.command-line", "Usage: please input a number", "ghost"}},
		{"cobol-ctags-comment-only", "SCRIPTHOLD_REAL_COBOL_CTAGS", "cobol", []want{}, []string{"DONTTAGME", "DONTTAGME.SECTION", "ghost"}},
		{"cobol-lexilla-token-samples", "SCRIPTHOLD_REAL_COBOL_LEXILLA", "cobol", []want{}, []string{"identifier", "data", "remarks", "preprocessor", "ghost"}},
		{"vhdl-linguist-inverter", "SCRIPTHOLD_REAL_VHDL_LINGUIST", "vhdl", []want{
			{"inverter", SymbolKindEntity, 6},
			{"inverter.rtl", SymbolKindImplementation, 11},
		}, []string{"ieee", "std_logic", "rtl", "inverter.ghost"}},
		{"vhdl-geany-process-bare-architecture", "SCRIPTHOLD_REAL_VHDL_GEANY_PROCESS", "vhdl", []want{
			{"StackTraceBuffer", SymbolKindEntity, 16},
			{"StackTraceBuffer.arch_StackTraceBuffer", SymbolKindImplementation, 31},
			{"StackTraceBuffer.arch_StackTraceBuffer.stackbuf", SymbolKindSignal, 34},
			{"StackTraceBuffer.arch_StackTraceBuffer.raddr", SymbolKindSignal, 35},
		}, []string{"StackTraceBuffer.arch_StackTraceBuffer.regs", "StackTraceBuffer.arch_StackTraceBuffer.ghost"}},
		{"vhdl-geany-logical-ops", "SCRIPTHOLD_REAL_VHDL_GEANY", "vhdl", []want{
			{"logical_ops_1", SymbolKindEntity, 2},
		}, []string{"bit", "std_logic", "logical_ops_1.ghost"}},
		{"vhdl-ctags-component", "SCRIPTHOLD_REAL_VHDL_CTAGS", "vhdl", []want{
			{"ENTITY_TOP", SymbolKindEntity, 6},
			{"ENTITY_TOP.arch", SymbolKindImplementation, 15},
			{"ENTITY_TOP.arch.sig", SymbolKindSignal, 16},
		}, []string{"ieee", "ENTITY_TOP.arch.ghost", "sig"}},
		{"vhdl-lexilla-generate", "SCRIPTHOLD_REAL_VHDL_LEXILLA", "vhdl", []want{
			{"x", SymbolKindEntity, 5},
			{"x.behavioral", SymbolKindImplementation, 16},
			{"x.behavioral.q_i", SymbolKindSignal, 17},
		}, []string{"std_logic", "behavioral", "rising_edge", "x.ghost"}},
		{"vhdl-pygments-multientity", "SCRIPTHOLD_REAL_VHDL_PYGMENTS", "vhdl", []want{
			{"top_testbench", SymbolKindEntity, 11},
			{"top_testbench.top_testbench_arch", SymbolKindImplementation, 18},
			{"top_testbench.top_testbench_arch.clk", SymbolKindSignal, 34},
			{"add32csa", SymbolKindEntity, 137},
			{"add32csa.circuits", SymbolKindImplementation, 148},
			{"add32csa.circuits.zero", SymbolKindSignal, 149},
		}, []string{"ieee", "std_logic_vector", "top_testbench.top_testbench_arch.ghost", "add32csa.circuits.ghost"}},
		{"vhdl-kde-muldiv-partial", "SCRIPTHOLD_REAL_VHDL_KDE", "vhdl", []want{
			{"light52_muldiv", SymbolKindEntity, 84},
			{"light52_muldiv.sequential", SymbolKindImplementation, 107},
			{"light52_muldiv.sequential.bit_ctr", SymbolKindSignal, 109},
			{"concat", SymbolKindEntity, 384},
		}, []string{"t_word", "work", "light52_muldiv.ghost"}},
		{"d-linguist-function", "SCRIPTHOLD_REAL_D_LINGUIST", "d", []want{
			{"foo", SymbolKindFunction, 1},
		}, []string{"bar", "void", "ghost"}},
		{"d-geany-struct-class", "SCRIPTHOLD_REAL_D_GEANY", "d", []want{
			{"test.simple", SymbolKindModule, 1},
			{"test.simple.Struct", SymbolKindStruct, 7},
			{"test.simple.Interface", SymbolKindInterface, 22},
			{"test.simple.Class", SymbolKindClass, 27},
			{"test.simple.Class.bar", SymbolKindMethod, 36},
			{"test.simple.main", SymbolKindFunction, 74},
		}, []string{"test.simple.assert", "test.simple.Class.assert", "test.simple.error", "ghost"}},
		{"d-pygments-unittest", "SCRIPTHOLD_REAL_D_PYGMENTS", "d", []want{
			{"phoneno", SymbolKindModule, 4},
			{"phoneno.stripNonDigit", SymbolKindFunction, 14},
			{"phoneno.wordToNum", SymbolKindFunction, 36},
			{"phoneno.main", SymbolKindFunction, 65},
		}, []string{"phoneno.assert", "phoneno.unittest", "phoneno.if", "ghost"}},
		{"d-source-highlight-class", "SCRIPTHOLD_REAL_D_SOURCE_HIGHLIGHT", "d", []want{
			{"main", SymbolKindFunction, 38},
			{"CmdLin", SymbolKindClass, 78},
			{"CmdLin.argnum", SymbolKindMethod, 90},
			{"CmdLin.argv", SymbolKindMethod, 95},
			{"CmdLin.suffix", SymbolKindMethod, 100},
		}, []string{"CmdLin.return", "CmdLin.writefln", "ghost"}},
		{"d-kde-highlight-fixture", "SCRIPTHOLD_REAL_D_KDE", "d", []want{
			{"highlighttest", SymbolKindModule, 6},
			{"highlighttest.main", SymbolKindFunction, 52},
		}, []string{"highlighttest.pragma", "highlighttest.if", "ghost"}},
		{"assembly-linguist-fasm", "SCRIPTHOLD_REAL_ASM_LINGUIST", "assembly", []want{
			{"start", SymbolKindLabel, 10},
			{"display_bytes_count", SymbolKindLabel, 64},
			{"get_params", SymbolKindLabel, 78},
			{"find_param", SymbolKindLabel, 107},
			{"get_output_file", SymbolKindLabel, 121},
		}, []string{"mov", "GetTickCount", "formatter", "ghost"}},
		{"assembly-universal-68hc11", "SCRIPTHOLD_REAL_ASM_CTAGS", "assembly", []want{
			{"INTERRUPTS", SymbolKindLabel, 1},
			{"sym2", SymbolKindLabel, 21},
			{"sym3", SymbolKindLabel, 22},
			{"sym4", SymbolKindLabel, 23},
		}, []string{"LDS", "LDAA", "bne", "ghost"}},
		{"assembly-geany-masm", "SCRIPTHOLD_REAL_ASM_GEANY", "assembly", []want{
			{"QUACK", SymbolKindLabel, 5},
			{"LabelMaker1", SymbolKindLabel, 25},
			{"LabelMaker2", SymbolKindLabel, 27},
		}, []string{"enter", "mov", "ENDP", "ghost"}},
		{"assembly-pygments-nasm", "SCRIPTHOLD_REAL_ASM_PYGMENTS", "assembly", []want{
			{"..start", SymbolKindLabel, 12},
			{"hello", SymbolKindLabel, 26},
			{"stacktop", SymbolKindLabel, 30},
		}, []string{"mov", "int", "resb", "ghost"}},
		{"assembly-source-highlight-screen", "SCRIPTHOLD_REAL_ASM_SOURCE_HIGHLIGHT", "assembly", []want{
			{"_VIOheight", SymbolKindLabel, 39},
			{"odos4", SymbolKindLabel, 47},
			{"cga", SymbolKindLabel, 56},
		}, []string{"ret", "info.txtrow", "public", "ghost"}},
		{"assembly-lexilla-styles", "SCRIPTHOLD_REAL_ASM_LEXILLA", "assembly", []want{}, []string{"identifier", "section", "ECX", "ghost"}},
		{"assembly-nasm-directives", "SCRIPTHOLD_REAL_ASM_NASM", "assembly", []want{}, []string{"__FILE__", "__LINE__", "db", "ghost"}},
		{"xml-elekstube-weather-svg", "SCRIPTHOLD_REAL_XML_ELEKSTUBE", "xml", []want{
			{"a", SymbolKindEntity, 1},
			{"b", SymbolKindEntity, 1},
			{"c", SymbolKindEntity, 1},
		}, []string{"svg", "linearGradient", "path", "d", "ghost"}},
		{"xml-sparkfun-icon-font", "SCRIPTHOLD_REAL_XML_SPARKFUN", "xml", []want{
			{"icomoon", SymbolKindEntity, 6},
		}, []string{"close, remove, times", "info-circle", "floppy-o, save", "font-face", "ghost"}},
		{"xml-kike-laser-svg", "SCRIPTHOLD_REAL_XML_KIKE", "xml", []want{
			{"svg3090", SymbolKindEntity, 2},
			{"metadata3139", SymbolKindEntity, 17},
			{"namedview3137", SymbolKindEntity, 29},
			{"defs3092", SymbolKindEntity, 51},
			{"marker22685", SymbolKindEntity, 53},
			{"path22683", SymbolKindEntity, 61},
		}, []string{"sodipodi:docname", "Arrow1Lend", "svg:svg", "ghost"}},
		{"xml-maxritter-enclosure-svg", "SCRIPTHOLD_REAL_XML_MAXRITTER", "xml", []want{
			{"face0-cut", SymbolKindEntity, 9},
			{"face1-cut", SymbolKindEntity, 13},
			{"face2-cut", SymbolKindEntity, 17},
			{"face0-interior", SymbolKindEntity, 33},
		}, []string{"path", "fill", "stroke", "ghost"}},
		{"xml-dapper-nhibernate-mapping", "SCRIPTHOLD_REAL_XML_DAPPER", "xml", []want{}, []string{"Id", "Counter1", "Posts", "hibernate-mapping", "ghost"}},
		{"xml-serilog-linker", "SCRIPTHOLD_REAL_XML_SERILOG", "xml", []want{}, []string{"Serilog", "Serilog.Capturing.TrimConfiguration", "get_IsStructureValueSupported", "linker", "ghost"}},
		{"xml-lexilla-cdata", "SCRIPTHOLD_REAL_XML_LEXILLA", "xml", []want{}, []string{"len", "done", "example-code", "foo", "ghost"}},
		{"xml-utilitech-favicon", "SCRIPTHOLD_REAL_XML_UTILITECH", "xml", []want{}, []string{"svg", "path", "ghost"}},
		{"sql-flask-schema", "SCRIPTHOLD_REAL_SQL_FLASK", "sql", []want{
			{"user", SymbolKindType, 7},
			{"post", SymbolKindType, 13},
		}, []string{"id", "username", "user.id", "post.author_id", "Ghost"}},
		{"sql-rezoom-user-model", "SCRIPTHOLD_REAL_SQL_REZOOM", "sql", []want{
			{"Users", SymbolKindType, 1},
			{"Groups", SymbolKindType, 9},
			{"UserGroupMaps", SymbolKindType, 14},
		}, []string{"Id", "UserId", "Users.Id", "Groups.Name", "Ghost"}},
		{"sql-sqlprovider-postgresql", "SCRIPTHOLD_REAL_SQL_SQLPROVIDER", "sql", []want{
			{"COUNTRIES", SymbolKindType, 1},
			{"LOCATIONS", SymbolKindType, 10},
			{"EMPLOYEES", SymbolKindType, 53},
			{"JOB_HISTORY", SymbolKindType, 80},
			{"other_schema", SymbolKindSchema, 152},
			{"other_schema.table_in_other_schema", SymbolKindType, 154},
		}, []string{"EMAIL", "EMPLOYEES.EMAIL", "REGION_ID", "phantom_schema", "Ghost"}},
		{"sql-geany-ctags-employees", "SCRIPTHOLD_REAL_SQL_GEANY", "sql", []want{
			{"employees", SymbolKindType, 8},
		}, []string{"address", "employees.address", "employees.name", "Ghost"}},
		{"sql-source-highlight-authors", "SCRIPTHOLD_REAL_SQL_SOURCE_HIGHLIGHT", "sql", []want{
			{"authors", SymbolKindType, 7},
		}, []string{"idauthor", "authors.surname", "surname", "Ghost"}},
		{"sql-pygments-dml-no-declarations", "SCRIPTHOLD_REAL_SQL_PYGMENTS", "sql", []want{},
			[]string{"#temp_employee", "employee", "Department", "Name", "Ghost"}},
		{"sql-sublime-tsql-symbols", "SCRIPTHOLD_REAL_SQL_SUBLIME", "sql", []want{
			{"hello", SymbolKindType, 3},
		}, []string{"IX_some_index", "dbo.some_table", "field1", "Ghost"}},
		{"sql-linguist-plsql-types", "SCRIPTHOLD_REAL_SQL_LINGUIST", "sql", []want{
			{"myobject", SymbolKindType, 1},
			{"myarray", SymbolKindType, 14},
		}, []string{"toString", "Compare", "myobject.m_name", "Ghost"}},
		{"sql-lexilla-non-declarations", "SCRIPTHOLD_REAL_SQL_LEXILLA", "sql", []want{}, []string{"select", "object", "identifier", "dbms_output.disable", "Ghost"}},
		{"sql-kde-malformed-table-header", "SCRIPTHOLD_REAL_SQL_KDE", "sql", []want{
			{"[Current Product List]", SymbolKindType, 51},
		}, []string{"stuff", "Customers", "Ghost"}},
		{"json-elekstube-esp32-board", "SCRIPTHOLD_REAL_JSON_ELEKSTUBE", "json", []want{
			{"build", SymbolKindKey, 2},
			{"build.arduino.ldscript", SymbolKindKey, 4},
			{"build.extra_flags", SymbolKindKey, 7},
			{"upload.flash_size", SymbolKindKey, 29},
			{"vendor", SymbolKindKey, 36},
		}, []string{"Ghost", "build.vendor", "build.arduino.mcu"}},
		{"json-lora-igate-config", "SCRIPTHOLD_REAL_JSON_LORA", "json", []want{
			{"callsign", SymbolKindKey, 2},
			{"wifi.autoAP.password", SymbolKindKey, 5},
			{"beacon.interval", SymbolKindKey, 14},
			{"lora.rxActive", SymbolKindKey, 43},
			{"syslog.active", SymbolKindKey, 80},
			{"mqtt.topic", SymbolKindKey, 94},
		}, []string{"wifi.password", "beacon.Ghost", "lora.rx", "mqtt.server.name"}},
		{"json-geekmagic-devcontainer", "SCRIPTHOLD_REAL_JSON_GEEKMAGIC", "json", []want{
			{"name", SymbolKindKey, 2},
			{"build.dockerfile", SymbolKindKey, 4},
			{"customizations.vscode.settings.terminal.integrated.defaultProfile.linux", SymbolKindKey, 9},
			{"runArgs", SymbolKindKey, 20},
			{"containerEnv.GNUPGHOME", SymbolKindKey, 42},
		}, []string{"dockerfile", "runArgs.name", "containerEnv.Ghost"}},
		{"json-openearable-vscode", "SCRIPTHOLD_REAL_JSON_OPENEARABLE", "json", []want{
			{"files.associations", SymbolKindKey, 2},
			{"files.associations.*.cps", SymbolKindKey, 3},
			{"files.associations.type_traits", SymbolKindKey, 4},
			{"files.associations.*.txt", SymbolKindKey, 14},
		}, []string{"type_traits", "files.associations.Ghost", "files"}},
		{"json-kike-wokwi-diagram", "SCRIPTHOLD_REAL_JSON_KIKE", "json", []want{
			{"version", SymbolKindKey, 2},
			{"author", SymbolKindKey, 3},
			{"parts", SymbolKindKey, 5},
			{"connections", SymbolKindKey, 10},
			{"dependencies", SymbolKindKey, 22},
		}, []string{"parts.type", "parts.id", "connections.id", "Ghost"}},
		{"json-senseshift-library", "SCRIPTHOLD_REAL_JSON_SENSESHIFT", "json", []want{
			{"$schema", SymbolKindKey, 2},
			{"frameworks", SymbolKindKey, 3},
			{"dependencies", SymbolKindKey, 4},
			{"dependencies.opengloves-lib", SymbolKindKey, 5},
		}, []string{"opengloves-lib", "dependencies.Ghost", "schema"}},
		{"json-farmbot-cpp-properties", "SCRIPTHOLD_REAL_JSON_FARMBOT", "json", []want{
			{"configurations", SymbolKindKey, 2},
		}, []string{"configurations.name", "configurations.includePath", "name", "Ghost"}},
		{"json-dcc-installer-root-array", "SCRIPTHOLD_REAL_JSON_DCC", "json", []want{}, []string{"Name", "Libraries.Name", "SupportedBoards.FQBN", "Ghost"}},
		{"html-ebus-config", "SCRIPTHOLD_REAL_HTML_EBUS", "html", []want{
			{"btnHome", SymbolKindEntity, 14},
			{"wifiSsid", SymbolKindEntity, 28},
			{"wifiTableBody", SymbolKindEntity, 50},
		}, []string{"WiFi Credentials", "fetchJson", "configureWireless", "ghost"}},
		{"html-dcc-websocket", "SCRIPTHOLD_REAL_HTML_DCC", "html", []want{
			{"message", SymbolKindEntity, 36},
			{"messages", SymbolKindEntity, 39},
		}, []string{"sender", "messageElem", "WebSocket", "ghost"}},
		{"html-elekstube-app", "SCRIPTHOLD_REAL_HTML_ELEKSTUBE", "html", []want{
			{"mainMenu", SymbolKindEntity, 800},
			{"clock", SymbolKindEntity, 806},
			{"logo", SymbolKindEntity, 810},
		}, []string{"face_item_' + key + '", "updateElements", "setFace", "ghost"}},
		{"html-lora-igate", "SCRIPTHOLD_REAL_HTML_LORA", "html", []want{
			{"backup", SymbolKindEntity, 74},
			{"restore", SymbolKindEntity, 75},
			{"themeToggle", SymbolKindEntity, 76},
			{"callsign", SymbolKindEntity, 264},
		}, []string{"localStorage", "igateTheme", "document", "ghost"}},
		{"html-sparkfun-rtk", "SCRIPTHOLD_REAL_HTML_SPARKFUN", "html", []want{
			{"batteryIcon", SymbolKindEntity, 85},
			{"batteryPercent", SymbolKindEntity, 87},
			{"resetInProcess", SymbolKindEntity, 92},
		}, []string{"WebSocket", "getElementById", "bootstrap", "ghost"}},
		{"html-geekmagic-gif", "SCRIPTHOLD_REAL_HTML_GEEKMAGIC", "html", []want{
			{"header-placeholder", SymbolKindEntity, 20},
			{"footer-placeholder", SymbolKindEntity, 80},
		}, []string{"gifUploadHandler", "uploadGif", "image/gif", "ghost"}},
		{"javascript-ebus-common", "SCRIPTHOLD_REAL_JS_EBUS", "javascript", []want{
			{"handleHome", SymbolKindFunction, 6},
			{"setStatus", SymbolKindFunction, 14},
			{"fetchJson", SymbolKindFunction, 25},
			{"postSimple", SymbolKindFunction, 44},
			{"handleSimpleTable", SymbolKindFunction, 64},
			{"renderSimpleTable", SymbolKindFunction, 81},
		}, []string{"window", "document", "jsonData", "fetch", "Set"}},
		{"javascript-elekstube-server", "SCRIPTHOLD_REAL_JS_ELEKSTUBE", "javascript", []want{
			{"app", SymbolKindVariable, 14},
			{"server", SymbolKindVariable, 16},
			{"uploadHandler", SymbolKindVariable, 29},
			{"pages", SymbolKindVariable, 72},
			{"sendPages", SymbolKindVariable, 90},
		}, []string{"req", "res", "http.createServer", "pages.value", "multiparty.Form"}},
		{"javascript-sparkfun-websocket", "SCRIPTHOLD_REAL_JS_SPARKFUN", "javascript", []want{
			{"gateway", SymbolKindVariable, 1},
			{"onLoad", SymbolKindFunction, 6},
			{"initWebSocket", SymbolKindFunction, 10},
			{"ge", SymbolKindFunction, 17},
			{"CoordinateTypes", SymbolKindConstant, 65},
		}, []string{"window", "WebSocket", "getElementById", "websocket.onmessage"}},
		{"javascript-geekmagic-gif", "SCRIPTHOLD_REAL_JS_GEEKMAGIC", "javascript", []want{
			{"gifUploadHandler", SymbolKindFunction, 1},
		}, []string{"apiFetch", "FormData", "humanFileSize", "this.uploadGif", "this.usedBytes"}},
		{"javascript-lora-aprs-settings", "SCRIPTHOLD_REAL_JS_LORA", "javascript", []want{
			{"currentSettings", SymbolKindVariable, 3},
			{"backupSettings", SymbolKindFunction, 5},
			{"fetchSettings", SymbolKindFunction, 39},
			{"loadSettings", SymbolKindFunction, 53},
			{"BeaconingViaRFCheckbox", SymbolKindConstant, 332},
		}, []string{"document", "settings.callsign", "reader", "this", "alert"}},
	}
	// Decorators and attributes belong to the declaration, while the identifier
	// itself starts on a later line; assert both coordinates independently.
	decoratedNameLines := map[string]map[string]int{
		"blazor-samples-child": {
			"Title": 9, "ChildContent": 12, "OnClickCallback": 15,
		},
		"python-pytest-multipython": {
			"python1": 17, "python2": 23, "test_basic_objects": 68,
		},
		"python-rich-attrs-example": {
			"Point3D": 12, "Triangle": 19, "Model": 26,
		},
		"csharp-humanizer-byte-size": {
			"Benchmarks.ByteSizeBenchmarks":                         7,
			"Benchmarks.ByteSizeBenchmarks.ByteSizeToStringDefault": 13,
			"Benchmarks.ByteSizeBenchmarks.ByteSizeToStringCulture": 17,
			"Benchmarks.ByteSizeBenchmarks.ByteSizeToStringFormat":  21,
			"Benchmarks.ByteSizeBenchmarks.ByteSizeToFullWords":     25,
		},
		"python-click-colors":            {"cli": 25},
		"python-fastapi-response-models": {"read_item": 19},
		"python-flask-auth": {
			"login_required.wrapped_view": 23, "load_logged_in_user": 33,
			"register": 47, "login": 85, "logout": 113,
		},
		"csharp-polly-retry": {
			"Polly.Benchmarks.Retry":                                        4,
			"Polly.Benchmarks.Retry.Retry_Synchronous_Succeeds":             10,
			"Polly.Benchmarks.Retry.Retry_Asynchronous_Succeeds":            14,
			"Polly.Benchmarks.Retry.Retry_Synchronous_Throws_Then_Succeeds": 34,
		},
		"csharp-dapper-belgrade-benchmark": {
			"Dapper.Tests.Performance.BelgradeBenchmarks":                10,
			"Dapper.Tests.Performance.BelgradeBenchmarks.Setup":          15,
			"Dapper.Tests.Performance.BelgradeBenchmarks.FirstOrDefault": 22,
		},
	}
	// Parent relations on independently inspected originals must agree with the
	// qualified identity, not just a matching text span.
	independentParents := map[string]map[string]string{
		"cobol-geany-program":                  {"Program-Name.INPUT-OUPUT": "Program-Name", "Program-Name.FILE": "Program-Name", "Program-Name.WORKING-STORAGE": "Program-Name", "Program-Name.Begin": "Program-Name", "Program-Name.Subprogram-Name": "Program-Name"},
		"cobol-sample-even-odd":                {"even-odd.working-storage": "even-odd", "even-odd.main": "even-odd"},
		"vhdl-geany-process-bare-architecture": {"StackTraceBuffer.arch_StackTraceBuffer": "StackTraceBuffer", "StackTraceBuffer.arch_StackTraceBuffer.stackbuf": "StackTraceBuffer.arch_StackTraceBuffer", "StackTraceBuffer.arch_StackTraceBuffer.raddr": "StackTraceBuffer.arch_StackTraceBuffer"},
		"vhdl-lexilla-generate":                {"x.behavioral": "x", "x.behavioral.q_i": "x.behavioral"},
		"vhdl-pygments-multientity":            {"top_testbench.top_testbench_arch": "top_testbench", "top_testbench.top_testbench_arch.clk": "top_testbench.top_testbench_arch", "add32csa.circuits.zero": "add32csa.circuits"},
		"vhdl-ctags-component":                 {"ENTITY_TOP.arch": "ENTITY_TOP", "ENTITY_TOP.arch.sig": "ENTITY_TOP.arch"},
		"d-geany-struct-class":                 {"test.simple.Class.bar": "test.simple.Class"},
		"d-source-highlight-class":             {"CmdLin.argnum": "CmdLin", "CmdLin.argv": "CmdLin"},
		"sql-sqlprovider-postgresql":           {"other_schema.table_in_other_schema": "other_schema"},
		"json-elekstube-esp32-board":           {"build.arduino.ldscript": "build.arduino", "upload.flash_size": "upload"},
		"json-lora-igate-config":               {"wifi.autoAP.password": "wifi.autoAP", "beacon.interval": "beacon", "lora.rxActive": "lora", "mqtt.topic": "mqtt"},
		"json-geekmagic-devcontainer":          {"build.dockerfile": "build", "customizations.vscode.settings.terminal.integrated.defaultProfile.linux": "customizations.vscode.settings", "containerEnv.GNUPGHOME": "containerEnv"},
		"json-openearable-vscode":              {"files.associations.type_traits": "files.associations", "files.associations.*.cps": "files.associations"},
		"json-senseshift-library":              {"dependencies.opengloves-lib": "dependencies"},
		"haskell-highlightjs-udp":              {"Network.UDP.DataPacket": "Network.UDP", "Network.UDP.openBoundUDPPort": "Network.UDP", "Network.UDP.pingUDPPort": "Network.UDP"},
		"objective-c-appdelegate":              {"RCTAppDelegate.init": "RCTAppDelegate", "RCTAppDelegate.rootViewFactory": "RCTAppDelegate"},
		"objective-c-testlib-common":           {"TestLibraryCommon.defaultPrefix": "TestLibraryCommon"},
		"objective-cpp-gist-wrapper":           {"MyCppNSStringWrapper.length": "MyCppNSStringWrapper", "MyCppNSStringWrapper.characterAtIndex": "MyCppNSStringWrapper"},
		"objective-cpp-react-appdelegate":      {"RCTAppDelegate.init": "RCTAppDelegate", "RCTAppDelegate.rootViewFactory": "RCTAppDelegate"},
		"objective-cpp-testlib-common":         {"TestLibraryCommon.defaultPrefix": "TestLibraryCommon"},
		"dart-sdk-core-bool":                   {"bool.parse": "bool", "bool.fromEnvironment": "bool"},
		"dart-chroma-monte-carlo":              {"Point.Point": "Point"},
		"dart-highlightjs-sample": {
			"SomeClass.SomeClass": "SomeClass", "SomeClass._internal": "SomeClass",
			"SomeClass.sum": "SomeClass", "SomeClass.els": "SomeClass",
		},
		"dart-pygments-greeter": {
			"Greeter.Greeter": "Greeter", "Greeter.withPrefix": "Greeter",
			"Greeter.greet": "Greeter", "Greeter.compareTo": "Greeter",
		},
		"purebasic-kenmo-7zip": {"__7ZIPSTRUCT.Init": "__7ZIPSTRUCT", "__7ZIPSTRUCT.Output": "__7ZIPSTRUCT"},
		"cpp-cli-libyara-meta": {
			"libyaraNET.Meta": "libyaraNET", "libyaraNET.Meta.Meta": "libyaraNET.Meta",
		},
		"cpp-cli-managedfbx-pose": {"ArcManagedFBX.FBXPose": "ArcManagedFBX"},
		"cpp-cli-managedffmpeg-constants": {
			"FFMpegNet.AVFormat": "FFMpegNet", "FFMpegNet.AVFormat.AVConstants": "FFMpegNet.AVFormat",
			"FFMpegNet.AVFormat.AVConstants.AVConstants": "FFMpegNet.AVFormat.AVConstants",
		},
		"cpp-cli-physx-constraint":  {"PhysX.ConstraintShaderTable": "PhysX"},
		"vba-commonlib-arrayutils":  {"VBALib_ArrayUtils.NormalizeArray": "VBALib_ArrayUtils", "VBALib_ArrayUtils.CopyMemory": "VBALib_ArrayUtils"},
		"vba-modernjson-common":     {"Json_Common.JsonTextBuilder": "Json_Common", "Json_Common.JsonSB_Init": "Json_Common"},
		"vba-xlframe-ezarray":       {"EZArray.Resize": "EZArray", "EZArray.NumRows": "EZArray"},
		"vba-xplus-math":            {"xpMath.INTERPOLATE_NUMBER": "xpMath", "xpMath.INTERPOLATE_PERCENT": "xpMath"},
		"cil-cecil-hello":           {"Hello.Foo.Bar": "Hello.Foo", "Hello.Foo.Baz": "Hello.Foo"},
		"cil-ilspy-bitnot":          {"ConvTest.Program.Main": "ConvTest.Program", "ConvTest.Enums.EUInt16.__value": "ConvTest.Enums.EUInt16"},
		"cpp-btop-logger":           {"Logger.Level": "Logger", "Logger.detail": "Logger", "Logger.detail.log_write": "Logger.detail"},
		"cpp-typesense-metrics":     {"AppMetrics.get_instance": "AppMetrics", "AppMetrics.increment_count": "AppMetrics"},
		"cpp-trafficmonitor-plugin": {"CPluginSystemDate.GetItemName": "CPluginSystemDate", "CPluginSystemDate.GetItemId": "CPluginSystemDate"},
		"matlab-lewos-app":          {"LeWoS_RS.RunSegmentationButtonPushed": "LeWoS_RS", "LeWoS_RS.ShowButtonPushed": "LeWoS_RS"},
		"matlab-platemo-problem":    {"CitySegMOP1.Setting": "CitySegMOP1"},
		"matlab-spatialmomentum":    {"SpatialMomentum.new": "SpatialMomentum"},
		"mql4-dingmaotu-text":       {"Text.setPrice": "Text", "Text.setDatetime": "Text"},
		"kotlin-moshi-indexed-parameters": {
			"com.squareup.moshi.kotlin.reflect.IndexedParameterMap.put":     "com.squareup.moshi.kotlin.reflect.IndexedParameterMap",
			"com.squareup.moshi.kotlin.reflect.IndexedParameterMap.entries": "com.squareup.moshi.kotlin.reflect.IndexedParameterMap",
		},
		"kotlin-timber-tree": {
			"timber.log.Timber.Tree":            "timber.log.Timber",
			"timber.log.Timber.Tree.isLoggable": "timber.log.Timber.Tree",
		},
		"kotlin-okio-forwarding-source": {
			"okio.ForwardingSource.delegate": "okio.ForwardingSource",
			"okio.ForwardingSource.timeout":  "okio.ForwardingSource",
		},
		"kotlin-materialfiles-action-state": {
			"me.zhanghai.android.files.util.ActionState.Ready": "me.zhanghai.android.files.util.ActionState",
		},
		"kotlin-gkd-load-status": {
			"li.songe.gkd.util.LoadStatus.Loading": "li.songe.gkd.util.LoadStatus",
		},
		"java-eventbus-no-subscriber": {
			"org.greenrobot.eventbus.NoSubscriberEvent.eventBus":          "org.greenrobot.eventbus.NoSubscriberEvent",
			"org.greenrobot.eventbus.NoSubscriberEvent.NoSubscriberEvent": "org.greenrobot.eventbus.NoSubscriberEvent",
		},
		"java-butterknife-class": {
			"butterknife.ButterKnife.setDebug": "butterknife.ButterKnife",
		},
		"java-algorithms-left-rotation": {
			"com.thealgorithms.others.ArrayLeftRotation.rotateLeft": "com.thealgorithms.others.ArrayLeftRotation",
		},
		"java-jcsprout-application": {
			"com.crossoverjie.Application.main": "com.crossoverjie.Application",
		},
		"java-termux-bytequeue": {
			"com.termux.terminal.ByteQueue.read":  "com.termux.terminal.ByteQueue",
			"com.termux.terminal.ByteQueue.write": "com.termux.terminal.ByteQueue",
		},
		"typescript-quill-selection": {"Bounds.bottom": "Bounds", "Bounds.height": "Bounds", "Bounds.width": "Bounds"},
		"typescript-hexo-types":      {"RenderData.engine": "RenderData", "TagSchema.name": "TagSchema", "TagSchema.slug": "TagSchema"},
		"go-participle-api-interfaces": {
			"participle.Capture.Capture": "participle.Capture",
			"participle.Parseable.Parse": "participle.Parseable",
		},
		"python-requests-exceptions": {
			"RequestException.__init__":  "RequestException",
			"JSONDecodeError.__reduce__": "JSONDecodeError",
		},
		"python-pytest-multipython": {
			"Python.dumps": "Python", "Python.load_and_is_true": "Python",
		},
		"csharp-automapper-features": {
			"AutoMapper.Features.IGlobalFeature.Configure":     "AutoMapper.Features.IGlobalFeature",
			"AutoMapper.Features.Features.Get":                 "AutoMapper.Features.Features",
			"AutoMapper.Features.FeatureExtensions.SetFeature": "AutoMapper.Features.FeatureExtensions",
		},
		"csharp-humanizer-byte-size": {
			"Benchmarks.ByteSizeBenchmarks.ByteSizeToStringDefault": "Benchmarks.ByteSizeBenchmarks",
		},
		"csharp-vs-validation-event-extensions": {
			"Microsoft.EventHandlerExtensions.Raise": "Microsoft.EventHandlerExtensions",
		},
	}
	// Real VHDL highlighting samples may contain unsupported or malformed constructs;
	// their bounded structural coverage must remain explicit.
	vhdlCoverage := map[string]bool{
		"vhdl-linguist-inverter":               true,
		"vhdl-geany-logical-ops":               true,
		"vhdl-geany-process-bare-architecture": true,
		"vhdl-ctags-component":                 true,
		"vhdl-lexilla-generate":                true,
		"vhdl-pygments-multientity":            true,
		"vhdl-kde-muldiv-partial":              false,
	}
	// The JSON provider deliberately reports arrays of objects and object-free
	// root arrays as incomplete, rather than inventing indexed key identities.
	jsonCoverage := map[string]bool{
		"json-elekstube-esp32-board":    true,
		"json-lora-igate-config":        true,
		"json-geekmagic-devcontainer":   true,
		"json-openearable-vscode":       true,
		"json-kike-wokwi-diagram":       false,
		"json-senseshift-library":       true,
		"json-farmbot-cpp-properties":   false,
		"json-dcc-installer-root-array": false,
	}
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	ran := 0
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent source corpus not configured")
			}
			ran++
			info, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			if !info.Mode().IsRegular() {
				t.Fatal("corpus case must be a regular source file, not a link")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			descriptor, ok := registry.Resolve(tc.language)
			if !ok || descriptor.ID != tc.language {
				t.Fatal("no canonical analyzer")
			}
			analyzer, ok := AnalyzerFor(descriptor)
			if !ok {
				t.Fatal("provider not active")
			}
			// Most checked-in samples are UTF-8; this original MQL4 file has
			// an authoritative UTF-16LE BOM and must use the real source loader.
			text := strings.TrimPrefix(string(data), "\uFEFF")
			analyzedDocument := testSourceDocument(filepath.Base(path), text)
			if tc.name == "mql4-harry-adx" {
				if len(data) < 2 || data[0] != 0xff || data[1] != 0xfe {
					t.Fatal("original MQL4 source must retain its UTF-16LE BOM")
				}
				analyzedDocument, err = OpenSourceDocument(context.Background(), path, OpenDocumentOptions{
					RequestedEncoding: "utf-16le", MaxFileBytes: 4 * 1024 * 1024, MaxDecodedCharacters: 1_000_000,
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			result, err := analyzer.Analyze(context.Background(), analyzedDocument, testAnalyzeOptions(true, 2048))
			if err != nil {
				t.Fatal(err)
			}
			assertAnalyzerResultCoordinates(t, result)
			if strings.HasPrefix(tc.name, "assembly-") &&
				(!result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) != 0) {
				t.Errorf("bounded assembly label scan unexpectedly incomplete: complete=%v diagnostics=%+v",
					result.Analysis.CoverageComplete, result.Analysis.Diagnostics)
			}
			if strings.HasPrefix(tc.name, "xml-") && tc.name != "xml-maxritter-enclosure-svg" &&
				(!result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) != 0) {
				t.Errorf("valid original XML unexpectedly incomplete: complete=%v diagnostics=%+v",
					result.Analysis.CoverageComplete, result.Analysis.Diagnostics)
			}
			if complete, exists := vhdlCoverage[tc.name]; exists {
				if result.Analysis.CoverageComplete != complete ||
					(complete && len(result.Analysis.Diagnostics) != 0) ||
					(!complete && len(result.Analysis.Diagnostics) == 0) {
					t.Errorf("VHDL original-source coverage: complete=%v diagnostics=%+v; expected complete=%v",
						result.Analysis.CoverageComplete, result.Analysis.Diagnostics, complete)
				}
			}
			if tc.name == "sql-kde-malformed-table-header" {
				if result.Analysis.CoverageComplete {
					t.Error("malformed real SQL table header must report incomplete coverage")
				}
				found := false
				for _, diagnostic := range result.Analysis.Diagnostics {
					if diagnostic.Code == "sql-unrecognized-table-header" {
						found = true
					}
				}
				if !found {
					t.Errorf("missing SQL table-header diagnostic: %+v", result.Analysis.Diagnostics)
				}
			}
			if wantComplete, exists := jsonCoverage[tc.name]; exists {
				if result.Analysis.CoverageComplete != wantComplete ||
					(wantComplete && len(result.Analysis.Diagnostics) != 0) ||
					(!wantComplete && len(result.Analysis.Diagnostics) == 0) {
					t.Errorf("JSON independent source coverage mismatch: complete=%v diagnostics=%+v; expected complete=%v",
						result.Analysis.CoverageComplete, result.Analysis.Diagnostics, wantComplete)
				}
				if tc.name == "json-dcc-installer-root-array" && len(result.Analysis.Symbols) != 0 {
					t.Errorf("unmodeled JSON array root must not invent keys: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
			seen := make(map[string][]NormalizedSymbol, len(result.Analysis.Symbols))
			for _, symbol := range result.Analysis.Symbols {
				seen[symbol.QualifiedName] = append(seen[symbol.QualifiedName], symbol)
			}
			for _, expected := range tc.present {
				matched := false
				for _, symbol := range seen[expected.name] {
					if symbol.Kind == expected.kind && symbol.DeclarationRange.Start.Line == expected.line {
						if wantLine, ok := decoratedNameLines[tc.name][expected.name]; ok &&
							symbol.NameRange.Start.Line != wantLine {
							t.Errorf("name position for %q: got %d, want %d", expected.name,
								symbol.NameRange.Start.Line, wantLine)
						}
						if wantParent, ok := independentParents[tc.name][expected.name]; ok &&
							symbol.ParentQualifiedName != wantParent {
							t.Errorf("parent for %q: got %q, want %q", expected.name,
								symbol.ParentQualifiedName, wantParent)
						}
						matched = true
						break
					}
				}
				if !matched {
					t.Errorf("expected declaration %q kind=%q line=%d, found=%+v; names=%v", expected.name, expected.kind,
						expected.line, seen[expected.name], sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
			for _, name := range tc.forbidden {
				if symbols := seen[name]; len(symbols) > 0 {
					t.Errorf("expression, comment, or local reference emitted as declaration %q: %+v", name, symbols)
				}
			}
			// Declaration-free templates and API callback-only examples must not
			// turn expressions or schema property names into symbol definitions.
			if len(tc.present) == 0 &&
				(strings.HasPrefix(tc.name, "blazor-") || strings.HasPrefix(tc.name, "jinja-") || strings.HasPrefix(tc.name, "twig-") || strings.HasPrefix(tc.name, "ejs-") ||
					tc.name == "jsp-final-user-form" || tc.name == "jsp-in28minutes-add-todo" || tc.name == "jsp-javaee7-builtins" ||
					tc.name == "razor-eshop-basket" || tc.name == "razor-orchardcore-dashboard" ||
					tc.name == "openapi-oai-callback-example" || tc.name == "luau-pvz-wave" ||
					strings.HasPrefix(tc.name, "classic-basic-") || strings.HasPrefix(tc.name, "php-html-") || strings.HasPrefix(tc.name, "cpp-cli-") || strings.HasPrefix(tc.name, "shell-") || strings.HasPrefix(tc.name, "objective-c-") || tc.name == "haskell-chroma-numeric" || tc.name == "sql-lexilla-non-declarations" || tc.name == "sql-pygments-dml-no-declarations" || strings.HasPrefix(tc.name, "xml-") || strings.HasPrefix(tc.name, "assembly-") || tc.name == "cobol-ctags-comment-only" || tc.name == "cobol-lexilla-token-samples") &&
				len(result.Analysis.Symbols) != 0 {
				t.Errorf("declaration-free source produced symbols: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
			if strings.HasPrefix(tc.name, "go-") || strings.HasPrefix(tc.name, "python-") ||
				strings.HasPrefix(tc.name, "csharp-") {
				if !result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) != 0 {
					t.Errorf("valid independent source unexpectedly incomplete: complete=%v diagnostics=%+v",
						result.Analysis.CoverageComplete, result.Analysis.Diagnostics)
				}
			}
			if strings.HasPrefix(tc.name, "autohotkey-") && len(result.Analysis.Symbols) != len(tc.present) {
				t.Errorf("AutoHotkey original-source declaration inventory differs: got %d symbols, want %d; declarations=%v",
					len(result.Analysis.Symbols), len(tc.present), seen)
			}
			if tc.name == "plsql-oxar" || tc.name == "xml-maxritter-enclosure-svg" {
				// Long source declarations exceed the configured signature-text cap.
				// Retained declarations remain checkable, but coverage must stay explicitly partial.
				if !result.Analysis.Truncated || result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) == 0 {
					t.Errorf("oversized source signature must report partial retention: %+v", result.Analysis)
				}
				for _, diagnostic := range result.Analysis.Diagnostics {
					if diagnostic.Code != "signature-limit" {
						t.Errorf("unexpected source signature partial-coverage reason: %+v", diagnostic)
					}
				}
			} else if result.Analysis.Truncated {
				t.Errorf("oracle cannot prove completeness with truncated source output: retained=%d diagnostics=%+v", len(result.Analysis.Symbols), result.Analysis.Diagnostics)
			}
		})
	}
	if ran == 0 {
		t.Skip("no independent corpus paths configured; portable semantic matrix remains active")
	}
	t.Logf("independent real source files checked: %d/%d; these cases do not certify all native providers", ran, len(cases))
}

// The KDE shell syntax-highlighting corpus intentionally contains malformed
// constructs. An analyzer may omit declarations, but must not claim completeness.
func TestSemanticIndependentMalformedBashCorpusCoverage(t *testing.T) {
	path := os.Getenv("SCRIPTHOLD_REAL_BASH_A")
	if path == "" {
		t.Skip("independent malformed shell corpus not configured")
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Fatal("corpus must be a regular source file, not a link")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (BashAnalyzer{}).Analyze(context.Background(), testSourceDocument(filepath.Base(path), string(data)), testAnalyzeOptions(true, 2048))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("malformed corpus must report incomplete, untruncated analysis: %+v", result.Analysis)
	}
}
