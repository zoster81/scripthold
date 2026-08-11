package encoding

import (
	"bufio"
	"bytes"
	"io"
	"unicode"
	"unicode/utf8"

	"github.com/wlynxg/chardet"
)

const (
	legacyDetectionMinBytes        = 8
	legacyDetectionMinHighBytes    = 4
	legacyDetectionMinRunes        = 4
	legacyDetectionMinTextPercent  = 95
	legacyStatefulMinNonASCIIRunes = 4
	legacyStatefulProbeBytes       = 64 * 1024
	legacyCandidateMargin          = 0.08
	legacyStrongConfidence         = 0.95
)

type legacyStructureScanner struct {
	tail          []byte
	statefulProbe []byte
	captureProbe  bool
	iso2022       bool
	iso2022JP     bool
	iso2022KR     bool
	hzOpen        bool
	hzClose       bool
	gb18030Four   int
}

func (scanner *legacyStructureScanner) Feed(data []byte) {
	if len(data) == 0 {
		return
	}
	combined := make([]byte, 0, len(scanner.tail)+len(data))
	combined = append(combined, scanner.tail...)
	combined = append(combined, data...)

	wasCapturing := scanner.captureProbe
	probeStart := -1
	for index := 0; index < len(combined); index++ {
		if index+1 < len(combined) && combined[index] == '~' {
			switch combined[index+1] {
			case '{':
				scanner.hzOpen = true
				if !scanner.captureProbe {
					scanner.captureProbe = true
					probeStart = index
				}
			case '}':
				scanner.hzClose = true
			}
		}
		if index+2 < len(combined) && combined[index] == 0x1b && combined[index+1] == '$' {
			scanner.iso2022 = true
			if !scanner.captureProbe {
				scanner.captureProbe = true
				probeStart = index
			}
			if combined[index+2] == 'B' || combined[index+2] == '@' {
				scanner.iso2022JP = true
			}
		}
		if index+3 < len(combined) && combined[index] == 0x1b && combined[index+1] == '$' && combined[index+2] == ')' && combined[index+3] == 'C' {
			scanner.iso2022 = true
			scanner.iso2022KR = true
		}
		if index+3 < len(combined) && isGB18030FourByteSequence(combined[index:index+4]) {
			scanner.gb18030Four++
		}
	}

	if wasCapturing {
		scanner.appendStatefulProbe(data)
	} else if scanner.captureProbe && probeStart >= 0 {
		scanner.appendStatefulProbe(combined[probeStart:])
	}

	const carry = 3
	start := max(0, len(combined)-carry)
	scanner.tail = append(scanner.tail[:0], combined[start:]...)
}

func (scanner *legacyStructureScanner) appendStatefulProbe(data []byte) {
	remaining := legacyStatefulProbeBytes - len(scanner.statefulProbe)
	if remaining <= 0 {
		return
	}
	if len(data) > remaining {
		data = data[:remaining]
	}
	scanner.statefulProbe = append(scanner.statefulProbe, data...)
}

func isGB18030FourByteSequence(data []byte) bool {
	return len(data) >= 4 &&
		data[0] >= 0x81 && data[0] <= 0xFE &&
		data[1] >= 0x30 && data[1] <= 0x39 &&
		data[2] >= 0x81 && data[2] <= 0xFE &&
		data[3] >= 0x30 && data[3] <= 0x39
}

func (scanner legacyStructureScanner) Decide(readerFactory func() io.Reader) (DetectionResult, bool) {
	statefulMarker := scanner.iso2022 || scanner.hzOpen && scanner.hzClose
	candidate := ""
	candidateCount := 0
	if scanner.hzOpen && scanner.hzClose {
		candidate = "hz-gb-2312"
		candidateCount++
	}
	if scanner.iso2022JP {
		candidate = "iso-2022-jp"
		candidateCount++
	}
	if scanner.iso2022KR {
		candidate = "iso-2022-kr"
		candidateCount++
	}

	if statefulMarker {
		if candidateCount != 1 || !statefulDetectorMatches(scanner.statefulProbe, candidate) ||
			!candidateTextPlausibleReaderMinNonASCII(readerFactory(), candidate, legacyStatefulMinNonASCIIRunes) {
			return DetectionResult{}, true
		}
		return DetectionResult{Charset: candidate, Confidence: 99}, true
	}

	// A grammatical four-byte GB18030 sequence is decisive evidence that the
	// payload is not GBK. Scripthold exposes both the generic x/text codec and an
	// exact GB18030:2022 codec, whose revision cannot be inferred from bytes
	// alone, so strong family evidence intentionally resolves to ambiguity.
	if scanner.gb18030Four >= 2 && candidateTextPlausibleReader(readerFactory(), "gb18030") {
		return DetectionResult{}, true
	}
	return DetectionResult{}, false
}

func detectStructuredLegacy(data []byte) (DetectionResult, bool) {
	var scanner legacyStructureScanner
	scanner.Feed(data)
	return scanner.Decide(func() io.Reader { return bytes.NewReader(data) })
}

func statefulDetectorMatches(data []byte, canonical string) bool {
	if len(data) == 0 {
		return false
	}
	raw := chardet.Detect(data)
	label := raw.Charset
	if label == "" {
		label = raw.Encoding
	}
	return raw.Confidence >= legacyStrongConfidence && canonicalDetectedCharset(label) == canonical
}

func detectLegacyCandidates(data []byte) DetectionResult {
	if len(data) < legacyDetectionMinBytes || countHighBytes(data) < legacyDetectionMinHighBytes {
		return DetectionResult{}
	}

	results := chardet.DetectAll(data)
	if len(results) == 0 {
		return DetectionResult{}
	}

	var selected legacyDetectionCandidate
	for index, raw := range results {
		label := raw.Charset
		if label == "" {
			label = raw.Encoding
		}
		if label == "" || raw.Confidence <= 0 {
			continue
		}
		canonical := canonicalDetectedCharset(label)
		if canonical == "" {
			// A stronger detector result with an explicit fail-closed disposition
			// must not be bypassed in favor of a weaker supported guess.
			if index == 0 || raw.Confidence >= float64(MinConfidenceThreshold)/100 {
				return DetectionResult{}
			}
			continue
		}
		selected = legacyDetectionCandidate{
			canonical:  canonical,
			confidence: raw.Confidence,
			index:      index,
		}
		break
	}
	if selected.canonical == "" || selected.confidence < legacyCandidateMinimumConfidence(selected.canonical) {
		return DetectionResult{}
	}
	if !candidateTextPlausible(data, selected.canonical) {
		return DetectionResult{}
	}
	if candidateHasPlausibleConfusionPeer(data, selected.canonical) {
		return DetectionResult{}
	}
	if candidateHasCloseDetectorAlternative(data, results, selected) {
		return DetectionResult{}
	}

	return DetectionResult{Charset: selected.canonical, Confidence: int(selected.confidence * 100)}
}

type legacyDetectionCandidate struct {
	canonical  string
	confidence float64
	index      int
}

func legacyCandidateMinimumConfidence(canonical string) float64 {
	switch canonical {
	case "windows-874":
		return 0.85
	case "iso-8859-1", "windows-1252":
		return 0.70
	default:
		return legacyStrongConfidence
	}
}

func candidateHasPlausibleConfusionPeer(data []byte, canonical string) bool {
	for _, peer := range legacyConfusionPeers[canonical] {
		if candidateTextPlausible(data, peer) {
			return true
		}
	}
	return false
}

var legacyConfusionPeers = map[string][]string{
	"iso-8859-1":     {"windows-1252"},
	"windows-1252":   {"iso-8859-1"},
	"iso-8859-7":     {"windows-1253"},
	"windows-1253":   {"iso-8859-7"},
	"iso-8859-8":     {"windows-1255"},
	"windows-1255":   {"iso-8859-8"},
	"iso-8859-9":     {"windows-1254"},
	"windows-1254":   {"iso-8859-9"},
	"x-mac-cyrillic": {"windows-1251"},
	"windows-1251":   {"x-mac-cyrillic"},
	"macintosh":      {"ibm437", "ibm850"},
}

func candidateHasCloseDetectorAlternative(data []byte, results []chardet.Result, selected legacyDetectionCandidate) bool {
	selectedClass := legacyCandidateClass(selected.canonical)
	if selectedClass == "" {
		return false
	}
	for index, raw := range results {
		if index == selected.index {
			continue
		}
		label := raw.Charset
		if label == "" {
			label = raw.Encoding
		}
		canonical := canonicalDetectedCharset(label)
		if canonical == "" || canonical == selected.canonical || legacyCandidateClass(canonical) != selectedClass {
			continue
		}
		if selected.confidence-raw.Confidence > legacyCandidateMargin {
			continue
		}
		if candidateTextPlausible(data, canonical) {
			return true
		}
	}
	return false
}

func legacyCandidateClass(canonical string) string {
	switch canonical {
	case "iso-8859-1", "iso-8859-9", "windows-1252", "windows-1254", "macintosh":
		return "latin"
	case "ibm855", "ibm866", "iso-8859-5", "koi8-r", "windows-1251", "x-mac-cyrillic":
		return "cyrillic"
	case "iso-8859-7", "windows-1253":
		return "greek"
	case "iso-8859-8", "windows-1255":
		return "hebrew"
	case "gbk", "big5":
		return "chinese"
	case "euc-jp", "shift_jis":
		return "japanese"
	case "euc-kr":
		return "korean"
	case "windows-874":
		return "thai"
	default:
		return ""
	}
}

func countHighBytes(data []byte) int {
	count := 0
	for _, value := range data {
		if value >= utf8.RuneSelf {
			count++
		}
	}
	return count
}

func candidateTextPlausible(data []byte, charset string) bool {
	return candidateTextPlausibleReader(bytes.NewReader(data), charset)
}

func candidateTextPlausibleReader(source io.Reader, charset string) bool {
	return candidateTextPlausibleReaderMinNonASCII(source, charset, 0)
}

func candidateTextPlausibleReaderMinNonASCII(source io.Reader, charset string, minNonASCII int) bool {
	decoded, err := NewDecoderReader(source, charset)
	if err != nil {
		return false
	}
	reader := bufio.NewReaderSize(decoded, 32*1024)
	var runeCount, goodRunes, nonASCII int
	for {
		value, _, err := reader.ReadRune()
		if err == io.EOF {
			break
		}
		if err != nil {
			return false
		}
		runeCount++
		if value > unicode.MaxASCII {
			nonASCII++
		}
		switch {
		case value == utf8.RuneError || value == 0 || isUnicodeNoncharacter(value):
			return false
		case value == '\n' || value == '\r' || value == '\t' || unicode.IsPrint(value):
			goodRunes++
		case unicode.IsControl(value):
			return false
		}
	}
	return runeCount >= legacyDetectionMinRunes && nonASCII >= minNonASCII && goodRunes*100 >= runeCount*legacyDetectionMinTextPercent
}

func detectLegacySamples(samples []byteSample) DetectionResult {
	for _, sample := range samples {
		result := detectLegacy(sample.data)
		if result.Charset != "" && result.Charset != "utf-8" && result.Confidence >= HighConfidenceThreshold {
			return result
		}
	}
	return detectLegacy(joinDetectionSamples(samples))
}
