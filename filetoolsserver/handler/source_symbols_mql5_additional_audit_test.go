package handler

import (
	"os"
	"testing"
)

// These scenarios are checked against independent original source and never
// execute MetaTrader experts, OpenCL libraries, or third-party build scripts.
func TestSourceSymbolsOriginalMQL5AdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original MQL5 corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "mql4-mrhewitt", language: "mql5",
			search: "OnTimer", primary: "MT5/NewsTrader.mq5",
			negativeFile: "MT5/TheMarket.mq5", negativeQuery: "OnTimer",
			paths: []string{
				"MT5/NewsTrader.mq5", "MT5/SignalTrader.mq5",
			},
			oracles: []extraAuditEvidence{
				{"MT5/NewsTrader.mq5", "input double   ATRMultiplier=1.5;"},
				{"MT5/NewsTrader.mq5", "input int      MinLegSize=5;"},
				{"MT5/NewsTrader.mq5", "EventSetTimer(1);"},
				{"MT5/NewsTrader.mq5", "countdown = 55 - now.sec;"},
				{"MT5/NewsTrader.mq5", "if ( countdown == 0 ) {"},
				{"MT5/NewsTrader.mq5", "double LegSize = MathMax(NormalizeDouble((atr / point) * ATRMultiplier,1), MinLegSize);"},
				{"MT5/NewsTrader.mq5", "placeOrder(ORDER_TYPE_BUY_STOP, LegSize, volume);"},
				{"MT5/NewsTrader.mq5", "placeOrder(ORDER_TYPE_SELL_STOP, LegSize, volume);"},
				{"MT5/NewsTrader.mq5", "request.action    = TRADE_ACTION_PENDING;"},
				{"MT5/NewsTrader.mq5", "if(!OrderSend(request,result))"},
				{"MT5/SignalTrader.mq5", "void OnTimer()"},
				{"MT5/TheMarket.mq5", "string signals[] = "},
				{"MT5/TheMarket.mq5", "void OnTick()"},
			},
			targets: []extraAuditSymbol{
				{"MT5/NewsTrader.mq5", "NewsMinute", 11},
				{"MT5/NewsTrader.mq5", "OnInit", 21},
				{"MT5/NewsTrader.mq5", "placeOrder", 31},
				{"MT5/NewsTrader.mq5", "ArrowedLineCreate", 87},
				{"MT5/NewsTrader.mq5", "OnTick", 173},
				{"MT5/NewsTrader.mq5", "OnTimer", 186},
				{"MT5/SignalTrader.mq5", "OnTimer", 186},
			},
		},
		{
			project: "mql5-dingmaotu-mql4-lib", language: "mql5",
			search: "CLPlatform", primary: "OpenCL/OpenCL.mqh",
			negativeFile: "OpenCL/Definitions.mqh", negativeQuery: "CLPlatform",
			paths: []string{
				"OpenCL/MQL5.mqh", "OpenCL/OpenCL.mqh", "OpenCL/Definitions.mqh",
			},
			oracles: []extraAuditEvidence{
				{"OpenCL/MQL5.mqh", "#property description \"MQL5 OpenCL API\""},
				{"OpenCL/MQL5.mqh", "#ifndef __MQL5BUILD__"},
				{"OpenCL/MQL5.mqh", "enum ENUM_OPENCL_PROPERTY_INTEGER"},
				{"OpenCL/MQL5.mqh", "CL_DEVICE_MAX_COMPUTE_UNITS,"},
				{"OpenCL/MQL5.mqh", "enum ENUM_CL_DEVICE_TYPE"},
				{"OpenCL/MQL5.mqh", "CL_DEVICE_GPU,"},
				{"OpenCL/OpenCL.mqh", "#include \"Definitions.mqh\""},
				{"OpenCL/OpenCL.mqh", "#import \"OpenCL.dll\""},
				{"OpenCL/OpenCL.mqh", "class CLPlatform"},
				{"OpenCL/OpenCL.mqh", "int               getDeviceCount(cl_device_type type=CL_DEVICE_TYPE_ALL) const;"},
				{"OpenCL/OpenCL.mqh", "bool              getDevices(cl_device_id &devices[],cl_device_type type=CL_DEVICE_TYPE_ALL) const;"},
				{"OpenCL/OpenCL.mqh", "if(clGetDeviceIDs(current(),type,deviceCount,devices,0)!=CL_SUCCESS) return false;"},
				{"OpenCL/OpenCL.mqh", "class CLContext"},
				{"OpenCL/Definitions.mqh", "#define cl_device_id intptr_t"},
				{"OpenCL/Definitions.mqh", "struct cl_image_format"},
			},
			targets: []extraAuditSymbol{
				{"OpenCL/MQL5.mqh", "ENUM_OPENCL_PROPERTY_STRING", 14},
				{"OpenCL/MQL5.mqh", "ENUM_OPENCL_PROPERTY_INTEGER", 43},
				{"OpenCL/MQL5.mqh", "ENUM_CL_DEVICE_TYPE", 57},
				{"OpenCL/OpenCL.mqh", "CLPlatform", 844},
				{"OpenCL/OpenCL.mqh", "getDeviceCount", 868},
				{"OpenCL/OpenCL.mqh", "getDevices", 869},
				{"OpenCL/OpenCL.mqh", "getDevices", 915},
				{"OpenCL/OpenCL.mqh", "CLContext", 925},
				{"OpenCL/Definitions.mqh", "cl_image_format", 197},
				{"OpenCL/Definitions.mqh", "cl_buffer_region", 228},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
