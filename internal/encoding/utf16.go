package encoding

import (
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

const (
	utf16MinRunes          = 4
	utf16MinTextPercent    = 95
	utf16MaxControlPercent = 2
	utf16MinScore          = 82
	utf16MinScoreLead      = 12
	utf16MinZeroDominance  = 0.12
)

type utf16Spec struct {
	charset          string
	order            detectionByteOrder
	expectedZeroByte int
}

var (
	utf16LESpec = utf16Spec{charset: "utf-16-le", order: detectionLittleEndian, expectedZeroByte: 1}
	utf16BESpec = utf16Spec{charset: "utf-16-be", order: detectionBigEndian, expectedZeroByte: 0}
)

type byteSample struct {
	data   []byte
	offset int64
}

type utf16Evidence struct {
	charset string

	unitCount        int
	runeCount        int
	goodRuneCount    int
	controlCount     int
	nulCount         int
	replacementRunes int
	noncharacters    int
	surrogatePairs   int
	expectedZeros    int
	unexpectedZeros  int

	structuralValid bool
	roundTrip       bool
}

type utf16Analyzer struct {
	spec     utf16Spec
	evidence utf16Evidence

	pendingByte    byte
	hasPendingByte bool
	pendingHigh    uint16
	hasPendingHigh bool
}

func newUTF16Analyzer(spec utf16Spec) *utf16Analyzer {
	return &utf16Analyzer{
		spec: spec,
		evidence: utf16Evidence{
			charset:         spec.charset,
			structuralValid: true,
			roundTrip:       true,
		},
	}
}

func (a *utf16Analyzer) Write(data []byte) {
	for _, current := range data {
		if !a.hasPendingByte {
			a.pendingByte = current
			a.hasPendingByte = true
			continue
		}

		first := a.pendingByte
		second := current
		a.hasPendingByte = false
		a.consumePair(first, second)
	}
}

func (a *utf16Analyzer) consumePair(first, second byte) {
	a.evidence.unitCount++

	pair := [2]byte{first, second}
	if pair[a.spec.expectedZeroByte] == 0 {
		a.evidence.expectedZeros++
	}
	if pair[1-a.spec.expectedZeroByte] == 0 {
		a.evidence.unexpectedZeros++
	}

	unit := a.spec.order.uint16(pair[0], pair[1])

	if a.hasPendingHigh {
		if unit >= 0xDC00 && unit <= 0xDFFF {
			high := a.pendingHigh
			a.hasPendingHigh = false
			decoded := utf16.DecodeRune(rune(high), rune(unit))
			encodedHigh, encodedLow := utf16.EncodeRune(decoded)
			if uint16(encodedHigh) != high || uint16(encodedLow) != unit {
				a.evidence.roundTrip = false
			}
			a.evidence.surrogatePairs++
			a.observeRune(decoded)
			return
		}

		a.evidence.structuralValid = false
		a.hasPendingHigh = false
	}

	switch {
	case unit >= 0xD800 && unit <= 0xDBFF:
		a.pendingHigh = unit
		a.hasPendingHigh = true
	case unit >= 0xDC00 && unit <= 0xDFFF:
		a.evidence.structuralValid = false
	default:
		decoded := rune(unit)
		if uint16(decoded) != unit {
			a.evidence.roundTrip = false
		}
		a.observeRune(decoded)
	}
}

func (a *utf16Analyzer) observeRune(value rune) {
	a.evidence.runeCount++

	switch {
	case value == utf8.RuneError:
		a.evidence.replacementRunes++
	case value == 0:
		a.evidence.nulCount++
	case isUnicodeNoncharacter(value):
		a.evidence.noncharacters++
	case value == '\n' || value == '\r' || value == '\t' || unicode.IsSpace(value):
		a.evidence.goodRuneCount++
	case unicode.IsPrint(value):
		a.evidence.goodRuneCount++
	case unicode.IsControl(value):
		a.evidence.controlCount++
	}
}

func (a *utf16Analyzer) Finish() utf16Evidence {
	if a.hasPendingByte {
		a.evidence.structuralValid = false
		a.hasPendingByte = false
	}
	if a.hasPendingHigh {
		a.evidence.structuralValid = false
		a.hasPendingHigh = false
	}
	return a.evidence
}

func isUnicodeNoncharacter(value rune) bool {
	return value >= 0xFDD0 && value <= 0xFDEF || value >= 0 && value <= utf8.MaxRune && value&0xFFFF >= 0xFFFE
}

func analyzeUTF16(data []byte, spec utf16Spec) utf16Evidence {
	analyzer := newUTF16Analyzer(spec)
	analyzer.Write(data)
	return analyzer.Finish()
}

func analyzeUTF16Samples(samples []byteSample, totalSize int64, spec utf16Spec) utf16Evidence {
	combined := utf16Evidence{charset: spec.charset, structuralValid: true, roundTrip: true}
	for _, sample := range samples {
		data := sample.data
		offset := sample.offset

		if offset%2 != 0 && len(data) > 0 {
			data = data[1:]
			offset++
		}
		if len(data)%2 != 0 && offset+int64(len(data)) < totalSize {
			data = data[:len(data)-1]
		}
		if offset > 0 && len(data) >= 2 {
			first := spec.order.uint16(data[0], data[1])
			if first >= 0xDC00 && first <= 0xDFFF {
				data = data[2:]
				offset += 2
			}
		}
		if offset+int64(len(data)) < totalSize && len(data) >= 2 {
			last := spec.order.uint16(data[len(data)-2], data[len(data)-1])
			if last >= 0xD800 && last <= 0xDBFF {
				data = data[:len(data)-2]
			}
		}

		mergeUTF16Evidence(&combined, analyzeUTF16(data, spec))
	}
	if totalSize%2 != 0 {
		combined.structuralValid = false
	}
	return combined
}

func mergeUTF16Evidence(target *utf16Evidence, source utf16Evidence) {
	target.unitCount += source.unitCount
	target.runeCount += source.runeCount
	target.goodRuneCount += source.goodRuneCount
	target.controlCount += source.controlCount
	target.nulCount += source.nulCount
	target.replacementRunes += source.replacementRunes
	target.noncharacters += source.noncharacters
	target.surrogatePairs += source.surrogatePairs
	target.expectedZeros += source.expectedZeros
	target.unexpectedZeros += source.unexpectedZeros
	target.structuralValid = target.structuralValid && source.structuralValid
	target.roundTrip = target.roundTrip && source.roundTrip
}

func (e utf16Evidence) eligible() bool {
	if !e.structuralValid || !e.roundTrip || e.unitCount < utf16MinRunes || e.runeCount < utf16MinRunes {
		return false
	}
	if e.nulCount != 0 || e.replacementRunes != 0 || e.noncharacters != 0 {
		return false
	}
	if e.goodRuneCount*100 < e.runeCount*utf16MinTextPercent {
		return false
	}
	return e.controlCount*100 <= e.runeCount*utf16MaxControlPercent
}

func (e utf16Evidence) zeroDominance() float64 {
	if e.unitCount == 0 {
		return 0
	}
	return float64(e.expectedZeros-e.unexpectedZeros) / float64(e.unitCount)
}

func (e utf16Evidence) score() int {
	if e.runeCount == 0 {
		return 0
	}
	quality := e.goodRuneCount * 50 / e.runeCount
	zeroBonus := int(maxFloat64(0, e.zeroDominance()) * 30)
	if zeroBonus > 30 {
		zeroBonus = 30
	}
	lengthBonus := e.runeCount / 8
	if lengthBonus > 10 {
		lengthBonus = 10
	}
	surrogateBonus := e.surrogatePairs
	if surrogateBonus > 3 {
		surrogateBonus = 3
	}
	return 20 + quality + zeroBonus + lengthBonus + surrogateBonus
}

func detectUTF16(data []byte) (DetectionResult, bool) {
	le := analyzeUTF16(data, utf16LESpec)
	be := analyzeUTF16(data, utf16BESpec)
	return decideUTF16(le, be)
}

func detectUTF16Samples(samples []byteSample, totalSize int64) (DetectionResult, bool) {
	le := analyzeUTF16Samples(samples, totalSize, utf16LESpec)
	be := analyzeUTF16Samples(samples, totalSize, utf16BESpec)
	return decideUTF16(le, be)
}

func decideUTF16(le, be utf16Evidence) (DetectionResult, bool) {
	leEligible := le.eligible()
	beEligible := be.eligible()
	rawSignal := hasUTF16RawSignal(le, be)

	if !leEligible && !beEligible {
		return DetectionResult{}, rawSignal
	}

	if leEligible && beEligible {
		winner, loser := le, be
		if be.score() > le.score() {
			winner, loser = be, le
		}
		if winner.score() >= utf16MinScore && winner.score()-loser.score() >= utf16MinScoreLead && winner.zeroDominance() >= utf16MinZeroDominance {
			return utf16Result(winner), true
		}
		return DetectionResult{}, rawSignal
	}

	winner := le
	if beEligible {
		winner = be
	}
	if winner.score() >= utf16MinScore && winner.zeroDominance() >= utf16MinZeroDominance {
		return utf16Result(winner), true
	}
	return DetectionResult{}, rawSignal
}

func utf16Result(evidence utf16Evidence) DetectionResult {
	confidence := evidence.score()
	if confidence < HighConfidenceThreshold {
		confidence = HighConfidenceThreshold
	}
	if confidence > 95 {
		confidence = 95
	}
	return DetectionResult{Charset: evidence.charset, Confidence: confidence}
}

func hasUTF16RawSignal(le, be utf16Evidence) bool {
	units := le.unitCount
	if be.unitCount > units {
		units = be.unitCount
	}
	if units == 0 {
		return false
	}

	evenZeros := be.expectedZeros
	oddZeros := le.expectedZeros
	maxZeros := evenZeros
	minZeros := oddZeros
	if oddZeros > evenZeros {
		maxZeros, minZeros = oddZeros, evenZeros
	}
	maxRatio := float64(maxZeros) / float64(units)
	dominance := float64(maxZeros-minZeros) / float64(units)

	if units >= 2 && maxRatio >= 0.5 && dominance >= 0.5 {
		return true
	}
	return units >= 4 && maxRatio >= 0.2 && dominance >= 0.15
}

func maxFloat64(first, second float64) float64 {
	if first > second {
		return first
	}
	return second
}
