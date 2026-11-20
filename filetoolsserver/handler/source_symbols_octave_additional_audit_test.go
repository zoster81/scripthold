package handler

import (
	"os"
	"testing"
)

// Task-oriented evidence comes from separately read original source.
// The two third-party Octave projects are never executed.
func TestSourceSymbolsOriginalOctaveAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Octave corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "octave-audio", language: "octave",
			// Where should MIDI timestamp/tempo handling be changed, without
			// confusing the read path with the file writer or audio envelope?
			search: "midifileread", primary: "inst/midifileread.m",
			negativeFile: "inst/audioEnvelope.m", negativeQuery: "midifileread",
			paths: []string{
				"inst/midifileread.m",
				"inst/midifileinfo.m",
				"inst/midifilewrite.m",
				"inst/private/readheader.m",
				"inst/private/getvariable.m",
				"inst/audioEnvelope.m",
			},
			oracles: []extraAuditEvidence{
				{"inst/midifileread.m", "function msg = midifileread(filename, varargin)"},
				{"inst/midifileread.m", "includemetaevents = 0;"},
				{"inst/midifileread.m", "blockhdr = readheader (fd);"},
				{"inst/midifileread.m", "t = getvariable(fd);"},
				{"inst/midifileread.m", "t = t * (tempo/ticks_per_qtr)/1e6;"},
				{"inst/midifileread.m", "tempo = polyval(double(data), 256);"},
				{"inst/midifileread.m", "if includemetaevents"},
				{"inst/midifileread.m", "%! msg = midifileread(testname, 'includemetaevents', true);"},
				{"inst/midifileinfo.m", "function mi = midifileinfo(filename)"},
				{"inst/midifileinfo.m", "hdr.ticks_per_qtr = polyval(double([frames ticks]), 256);"},
				{"inst/midifilewrite.m", "function midifilewrite(varargin)"},
				{"inst/midifilewrite.m", "writeheader (fd, hdr);"},
				{"inst/midifilewrite.m", "ts = (ts*1e6)/(tempo/ticks_per_qtr);"},
				{"inst/private/readheader.m", "function h  = readheader (fd)"},
				{"inst/private/getvariable.m", "function t = getvariable (fd)"},
				{"inst/private/getvariable.m", "t = bitshift(t, 7) + bitand(b, uint64(0x7f));"},
				{"inst/audioEnvelope.m", "function [minenv, maxenv, loc, fs] = audioEnvelope (source, varargin)"},
			},
			targets: []extraAuditSymbol{
				{"inst/midifileread.m", "midifileread", 36},
				{"inst/midifileinfo.m", "midifileinfo", 40},
				{"inst/midifilewrite.m", "midifilewrite", 43},
				{"inst/private/readheader.m", "readheader", 22},
				{"inst/private/getvariable.m", "getvariable", 22},
				{"inst/audioEnvelope.m", "audioEnvelope", 54},
			},
		},
		{
			project: "octave-eeglab", language: "octave",
			// Where is original event latency restored after removed segments,
			// and which event editing paths consume the mapping?
			search: "eeg_urlatency", primary: "functions/popfunc/eeg_urlatency.m",
			negativeFile: "functions/popfunc/eeg_eventtypes.m", negativeQuery: "eeg_urlatency",
			paths: []string{
				"functions/popfunc/eeg_urlatency.m",
				"functions/popfunc/eeg_latencyur.m",
				"functions/popfunc/eeg_addnewevents.m",
				"functions/popfunc/pop_editeventvals.m",
				"functions/popfunc/eeg_eventtypes.m",
			},
			oracles: []extraAuditEvidence{
				{"functions/popfunc/eeg_urlatency.m", "function latout = eeg_urlatency( events, latin )"},
				{"functions/popfunc/eeg_urlatency.m", "indbound = strmatch('boundary', boundevents);"},
				{"functions/popfunc/eeg_urlatency.m", "latout(tmpInds) = latout(tmpInds) + events(index).duration;"},
				{"functions/popfunc/eeg_urlatency.m", "latout = NaN;"},
				{"functions/popfunc/eeg_latencyur.m", "function latout = eeg_latencyur( events, latin );"},
				{"functions/popfunc/eeg_latencyur.m", "latout(lowerVals) = latout(lowerVals)-events(index).duration;"},
				{"functions/popfunc/eeg_addnewevents.m", "newUreventLatency = [newUreventLatency eeg_urlatency(EEG.event, EEG.event(i).latency)];"},
				{"functions/popfunc/pop_editeventvals.m", "editval = eeg_urlatency(EEG.event, EEG.event(valnum).latency);"},
				{"functions/popfunc/eeg_eventtypes.m", "function [types,numbers] = eeg_eventtypes(EEG,arg2,arg3)"},
			},
			targets: []extraAuditSymbol{
				{"functions/popfunc/eeg_urlatency.m", "eeg_urlatency", 50},
				{"functions/popfunc/eeg_latencyur.m", "eeg_latencyur", 51},
				{"functions/popfunc/eeg_addnewevents.m", "eeg_addnewevents", 53},
				{"functions/popfunc/pop_editeventvals.m", "pop_editeventvals", 72},
				{"functions/popfunc/eeg_eventtypes.m", "eeg_eventtypes", 73},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
