package handler

import (
	"os"
	"testing"
)

// Provider 76 uses two further independent Arduino project origins, each
// pinned to an immutable upstream revision and read in full before source_*.
func TestSourceSymbolsArduinoSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("additional original Arduino project snapshots not configured")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "arduino76-irremote-audit-selected", language: "arduino",
			search: "loop", primary: "examples/ControlRelay/ControlRelay.ino",
			negativeFile:  "examples/TinySender/TinySender.ino",
			negativeQuery: "handleReceivedTinyIRData",
			paths: []string{
				"examples/ControlRelay/ControlRelay.ino",
				"examples/TinySender/TinySender.ino",
				"examples/TinyReceiver/TinyReceiver.ino",
			},
			oracles: []extraAuditEvidence{
				{"examples/ControlRelay/ControlRelay.ino", "void setup() {"},
				{"examples/ControlRelay/ControlRelay.ino", "if (millis() - last > 250)"},
				{"examples/ControlRelay/ControlRelay.ino", "digitalWrite(RELAY_PIN, HIGH)"},
				{"examples/ControlRelay/ControlRelay.ino", "IrReceiver.resume()"},
				{"examples/TinySender/TinySender.ino", "sendFAST(IR_SEND_PIN, sCommand, sRepeats)"},
				{"examples/TinySender/TinySender.ino", "sendNEC(IR_SEND_PIN, sAddress, sCommand, sRepeats)"},
				{"examples/TinyReceiver/TinyReceiver.ino", "if (TinyIRReceiverDecode())"},
				{"examples/TinyReceiver/TinyReceiver.ino", "void handleReceivedTinyIRData() {"},
				{"examples/TinyReceiver/TinyReceiver.ino", "#if defined(USE_CALLBACK_FOR_TINY_RECEIVER)"},
			},
			targets: []extraAuditSymbol{
				{"examples/ControlRelay/ControlRelay.ino", "setup", 51},
				{"examples/ControlRelay/ControlRelay.ino", "loop", 75},
				{"examples/TinySender/TinySender.ino", "setup", 53},
				{"examples/TinySender/TinySender.ino", "loop", 75},
				{"examples/TinyReceiver/TinyReceiver.ino", "setup", 71},
				{"examples/TinyReceiver/TinyReceiver.ino", "loop", 101},
				{"examples/TinyReceiver/TinyReceiver.ino", "handleReceivedTinyIRData", 147},
			},
		},
		{
			project: "arduino76-pubsubclient-audit-selected", language: "arduino",
			search: "reconnect", primary: "examples/mqtt_reconnect_nonblocking/mqtt_reconnect_nonblocking.ino",
			negativeFile:  "examples/mqtt_reconnect_nonblocking/mqtt_reconnect_nonblocking.ino",
			negativeQuery: "setup_wifi",
			paths: []string{
				"examples/mqtt_esp8266/mqtt_esp8266.ino",
				"examples/mqtt_large_message/mqtt_large_message.ino",
				"examples/mqtt_reconnect_nonblocking/mqtt_reconnect_nonblocking.ino",
			},
			oracles: []extraAuditEvidence{
				{"examples/mqtt_esp8266/mqtt_esp8266.ino", "void setup_wifi() {"},
				{"examples/mqtt_esp8266/mqtt_esp8266.ino", "client.subscribe(\"inTopic\")"},
				{"examples/mqtt_esp8266/mqtt_esp8266.ino", "delay(5000)"},
				{"examples/mqtt_large_message/mqtt_large_message.ino", "client.beginPublish(\"greenBottles/lyrics\", msgLen, false)"},
				{"examples/mqtt_large_message/mqtt_large_message.ino", "client.endPublish()"},
				{"examples/mqtt_reconnect_nonblocking/mqtt_reconnect_nonblocking.ino", "boolean reconnect() {"},
				{"examples/mqtt_reconnect_nonblocking/mqtt_reconnect_nonblocking.ino", "lastReconnectAttempt > 5000"},
				{"examples/mqtt_reconnect_nonblocking/mqtt_reconnect_nonblocking.ino", "return client.connected()"},
			},
			targets: []extraAuditSymbol{
				{"examples/mqtt_esp8266/mqtt_esp8266.ino", "setup_wifi", 37},
				{"examples/mqtt_esp8266/mqtt_esp8266.ino", "callback", 61},
				{"examples/mqtt_esp8266/mqtt_esp8266.ino", "reconnect", 81},
				{"examples/mqtt_large_message/mqtt_large_message.ino", "callback", 65},
				{"examples/mqtt_large_message/mqtt_large_message.ino", "reconnect", 141},
				{"examples/mqtt_reconnect_nonblocking/mqtt_reconnect_nonblocking.ino", "callback", 20},
				{"examples/mqtt_reconnect_nonblocking/mqtt_reconnect_nonblocking.ino", "reconnect", 29},
				{"examples/mqtt_reconnect_nonblocking/mqtt_reconnect_nonblocking.ino", "setup", 39},
				{"examples/mqtt_reconnect_nonblocking/mqtt_reconnect_nonblocking.ino", "loop", 50},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
