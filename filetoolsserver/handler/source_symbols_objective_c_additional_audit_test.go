package handler

import (
	"os"
	"testing"
)

// Source-first task oracles on independent original Objective-C projects.
// These tests verify navigation, not Objective-C application execution.
func TestSourceSymbolsOriginalObjectiveCAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Objective-C corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "objective-c-bradlarson-gpuimage", language: "objective-c",
			// Change brightness default and shader uniform update; contrast
			// has a similar but distinct setter and default.
			search: "setBrightness:", primary: "framework/Source/GPUImageBrightnessFilter.m",
			negativeFile:  "framework/Source/GPUImageContrastFilter.m",
			negativeQuery: "setBrightness:",
			paths: []string{
				"framework/Source/GPUImageBrightnessFilter.m",
				"framework/Source/GPUImageBrightnessFilter.h",
				"framework/Source/GPUImageFilter.m",
				"framework/Source/GPUImageContrastFilter.m",
			},
			oracles: []extraAuditEvidence{
				{"framework/Source/GPUImageBrightnessFilter.m", "@implementation GPUImageBrightnessFilter"},
				{"framework/Source/GPUImageBrightnessFilter.m", "self.brightness = 0.0;"},
				{"framework/Source/GPUImageBrightnessFilter.m", "brightnessUniform = [filterProgram uniformIndex:@\"brightness\"];"},
				{"framework/Source/GPUImageBrightnessFilter.m", "[self setFloat:_brightness forUniform:brightnessUniform program:filterProgram];"},
				{"framework/Source/GPUImageBrightnessFilter.h", "@interface GPUImageBrightnessFilter : GPUImageFilter"},
				{"framework/Source/GPUImageBrightnessFilter.h", "@property(readwrite, nonatomic) CGFloat brightness;"},
				{"framework/Source/GPUImageFilter.m", "- (void)setFloat:(GLfloat)floatValue forUniform:(GLint)uniform program:(GLProgram *)shaderProgram;"},
				{"framework/Source/GPUImageContrastFilter.m", "self.contrast = 1.0;"},
				{"framework/Source/GPUImageContrastFilter.m", "- (void)setContrast:(CGFloat)newValue;"},
			},
			targets: []extraAuditSymbol{
				{"framework/Source/GPUImageBrightnessFilter.m", "GPUImageBrightnessFilter", 35},
				{"framework/Source/GPUImageBrightnessFilter.m", "setBrightness:", 58},
				{"framework/Source/GPUImageBrightnessFilter.h", "GPUImageBrightnessFilter", 3},
				{"framework/Source/GPUImageContrastFilter.m", "setContrast:", 58},
			},
		},
		{
			project: "objective-c-core-code-corelib", language: "objective-c",
			// Locate sleep/wake overdue timer handling and schedule
			// tolerance without confusing an unrelated persistent cache.
			search: "receiveWakeNote:", primary: "Mac/JMCorrectTimer.m",
			negativeFile: "Mac/JMPersistentCache.m", negativeQuery: "receiveWakeNote:",
			paths: []string{
				"Mac/JMCorrectTimer.m",
				"Mac/JMCorrectTimer.h",
				"Mac/JMPersistentCache.m",
				"Tests/CoreLibTests.m",
			},
			oracles: []extraAuditEvidence{
				{"Mac/JMCorrectTimer.m", "@implementation JMCorrectTimer"},
				{"Mac/JMCorrectTimer.m", "t.tolerance = 0.1;"},
				{"Mac/JMCorrectTimer.m", "- (void)receiveSleepNote:(id)sender"},
				{"Mac/JMCorrectTimer.m", "- (void)receiveWakeNote:(id)sender"},
				{"Mac/JMCorrectTimer.m", "timeIntervalSinceDate:self.date] > 0.01"},
				{"Mac/JMCorrectTimer.m", "[self scheduleTimer];"},
				{"Mac/JMCorrectTimer.h", "@interface JMCorrectTimer : NSObject"},
				{"Mac/JMCorrectTimer.h", "initWithFireDate:(NSDate *)d timerBlock:"},
				{"Mac/JMPersistentCache.m", "@implementation JMPersistentCache"},
				{"Mac/JMPersistentCache.m", "- (BOOL)save:(NSError * __autoreleasing *)error"},
				{"Tests/CoreLibTests.m", "@implementation CoreLibTests"},
			},
			targets: []extraAuditSymbol{
				{"Mac/JMCorrectTimer.m", "JMCorrectTimer", 27},
				{"Mac/JMCorrectTimer.m", "scheduleTimer", 61},
				{"Mac/JMCorrectTimer.m", "receiveSleepNote:", 108},
				{"Mac/JMCorrectTimer.m", "receiveWakeNote:", 121},
				{"Mac/JMCorrectTimer.h", "JMCorrectTimer", 14},
				{"Mac/JMPersistentCache.m", "JMPersistentCache", 25},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
