package encoding

import (
	"encoding/binary"
	"unicode"
	"unicode/utf8"
)

const (
	utf32MinRunes          = 4
	utf32MinTextPercent    = 95
	utf32MaxControlPercent = 2
	utf32MinScore          = 82
	utf32MinScoreLead      = 12
	utf32MinZeroDominance  = 0.35
)

type utf32Spec struct {
	charset               string
	order                 binary.ByteOrder
	expectedAnchorByte    int
	expectedSecondaryByte int
	oppositeAnchorByte    int
}

var (
	utf32LESpec = utf32Spec{charset: "utf-32-le", order: binary.LittleEndian, expectedAnchorByte: 3, expectedSecondaryByte: 2, oppositeAnchorByte: 0}
	utf32BESpec = utf32Spec{charset: "utf-32-be", order: binary.BigEndian, expectedAnchorByte: 0, expectedSecondaryByte: 1, oppositeAnchorByte: 3}
)

type utf32Evidence struct {
	charset string

	unitCount              int
	runeCount              int
	goodRuneCount          int
	controlCount           int
	nulCount               int
	noncharacters          int
	expectedZeros          int
	expectedSecondaryZeros int
	oppositeZeros          int
	structuralValid        bool
	roundTrip              bool
}

type utf32Analyzer struct {
	spec     utf32Spec
	evidence utf32Evidence
	pending  [4]byte
	pendingN int
}

func newUTF32Analyzer(spec utf32Spec) *utf32Analyzer {
	return &utf32Analyzer{
		spec: spec,
		evidence: utf32Evidence{
			charset:         spec.charset,
			structuralValid: true,
			roundTrip:       true,
		},
	}
}

func (analyzer *utf32Analyzer) Write(data []byte) {
	for _, current := range data {
		analyzer.pending[analyzer.pendingN] = current
		analyzer.pendingN++
		if analyzer.pendingN == len(analyzer.pending) {
			analyzer.consumeUnit(analyzer.pending)
			analyzer.pendingN = 0
		}
	}
}

func (analyzer *utf32Analyzer) consumeUnit(raw [4]byte) {
	analyzer.evidence.unitCount++
	if raw[analyzer.spec.expectedAnchorByte] == 0 {
		analyzer.evidence.expectedZeros++
	}
	if raw[analyzer.spec.expectedSecondaryByte] == 0 {
		analyzer.evidence.expectedSecondaryZeros++
	}
	if raw[analyzer.spec.oppositeAnchorByte] == 0 {
		analyzer.evidence.oppositeZeros++
	}

	value := analyzer.spec.order.Uint32(raw[:])
	if value > utf8.MaxRune || value >= 0xD800 && value <= 0xDFFF {
		analyzer.evidence.structuralValid = false
		return
	}

	var encoded [4]byte
	analyzer.spec.order.PutUint32(encoded[:], value)
	if encoded != raw {
		analyzer.evidence.roundTrip = false
	}
	analyzer.observeRune(rune(value))
}

func (analyzer *utf32Analyzer) observeRune(value rune) {
	analyzer.evidence.runeCount++
	switch {
	case value == 0:
		analyzer.evidence.nulCount++
	case isUnicodeNoncharacter(value):
		analyzer.evidence.noncharacters++
	case value == '\n' || value == '\r' || value == '\t' || unicode.IsSpace(value):
		analyzer.evidence.goodRuneCount++
	case unicode.IsPrint(value):
		analyzer.evidence.goodRuneCount++
	case unicode.IsControl(value):
		analyzer.evidence.controlCount++
	}
}

func (analyzer *utf32Analyzer) Finish() utf32Evidence {
	if analyzer.pendingN != 0 {
		analyzer.evidence.structuralValid = false
		analyzer.pendingN = 0
	}
	return analyzer.evidence
}

func analyzeUTF32(data []byte, spec utf32Spec) utf32Evidence {
	analyzer := newUTF32Analyzer(spec)
	analyzer.Write(data)
	return analyzer.Finish()
}

func analyzeUTF32Samples(samples []byteSample, totalSize int64, spec utf32Spec) utf32Evidence {
	combined := utf32Evidence{charset: spec.charset, structuralValid: true, roundTrip: true}
	for _, sample := range samples {
		data := sample.data
		offset := sample.offset
		if remainder := offset % 4; remainder != 0 && len(data) > 0 {
			skip := int(4 - remainder)
			if skip >= len(data) {
				continue
			}
			data = data[skip:]
			offset += int64(skip)
		}
		if remainder := len(data) % 4; remainder != 0 && offset+int64(len(data)) < totalSize {
			data = data[:len(data)-remainder]
		}
		mergeUTF32Evidence(&combined, analyzeUTF32(data, spec))
	}
	if totalSize%4 != 0 {
		combined.structuralValid = false
	}
	return combined
}

func mergeUTF32Evidence(target *utf32Evidence, source utf32Evidence) {
	target.unitCount += source.unitCount
	target.runeCount += source.runeCount
	target.goodRuneCount += source.goodRuneCount
	target.controlCount += source.controlCount
	target.nulCount += source.nulCount
	target.noncharacters += source.noncharacters
	target.expectedZeros += source.expectedZeros
	target.expectedSecondaryZeros += source.expectedSecondaryZeros
	target.oppositeZeros += source.oppositeZeros
	target.structuralValid = target.structuralValid && source.structuralValid
	target.roundTrip = target.roundTrip && source.roundTrip
}

func (evidence utf32Evidence) eligible() bool {
	if !evidence.structuralValid || !evidence.roundTrip || evidence.unitCount < utf32MinRunes || evidence.runeCount < utf32MinRunes {
		return false
	}
	if evidence.nulCount != 0 || evidence.noncharacters != 0 {
		return false
	}
	if evidence.goodRuneCount*100 < evidence.runeCount*utf32MinTextPercent {
		return false
	}
	return evidence.controlCount*100 <= evidence.runeCount*utf32MaxControlPercent
}

func (evidence utf32Evidence) zeroDominance() float64 {
	if evidence.unitCount == 0 {
		return 0
	}
	upperZeros := evidence.expectedZeros
	if evidence.expectedSecondaryZeros < upperZeros {
		upperZeros = evidence.expectedSecondaryZeros
	}
	return float64(upperZeros-evidence.oppositeZeros) / float64(evidence.unitCount)
}

func (evidence utf32Evidence) score() int {
	if evidence.runeCount == 0 {
		return 0
	}
	quality := evidence.goodRuneCount * 50 / evidence.runeCount
	zeroBonus := int(maxFloat64(0, evidence.zeroDominance()) * 30)
	if zeroBonus > 30 {
		zeroBonus = 30
	}
	lengthBonus := evidence.runeCount / 8
	if lengthBonus > 10 {
		lengthBonus = 10
	}
	return 20 + quality + zeroBonus + lengthBonus
}

func detectUTF32(data []byte) (DetectionResult, bool) {
	le := analyzeUTF32(data, utf32LESpec)
	be := analyzeUTF32(data, utf32BESpec)
	return decideUTF32(le, be)
}

func detectUTF32Samples(samples []byteSample, totalSize int64) (DetectionResult, bool) {
	le := analyzeUTF32Samples(samples, totalSize, utf32LESpec)
	be := analyzeUTF32Samples(samples, totalSize, utf32BESpec)
	return decideUTF32(le, be)
}

func decideUTF32(le, be utf32Evidence) (DetectionResult, bool) {
	leEligible := le.eligible()
	beEligible := be.eligible()
	rawSignal := hasUTF32RawSignal(le, be)
	if !leEligible && !beEligible {
		return DetectionResult{}, rawSignal
	}

	if leEligible && beEligible {
		winner, loser := le, be
		if be.score() > le.score() {
			winner, loser = be, le
		}
		if winner.score() >= utf32MinScore && winner.score()-loser.score() >= utf32MinScoreLead && winner.zeroDominance() >= utf32MinZeroDominance {
			return utf32Result(winner), true
		}
		return DetectionResult{}, rawSignal
	}

	winner := le
	if beEligible {
		winner = be
	}
	if winner.score() >= utf32MinScore && winner.zeroDominance() >= utf32MinZeroDominance {
		return utf32Result(winner), true
	}
	return DetectionResult{}, rawSignal
}

func utf32Result(evidence utf32Evidence) DetectionResult {
	confidence := evidence.score()
	if confidence < HighConfidenceThreshold {
		confidence = HighConfidenceThreshold
	}
	if confidence > 95 {
		confidence = 95
	}
	return DetectionResult{Charset: evidence.charset, Confidence: confidence}
}

func hasUTF32RawSignal(le, be utf32Evidence) bool {
	units := le.unitCount
	if be.unitCount > units {
		units = be.unitCount
	}
	if units < 2 {
		return false
	}
	bestPrimary := le.expectedZeros
	bestSecondary := le.expectedSecondaryZeros
	if be.expectedZeros+be.expectedSecondaryZeros > bestPrimary+bestSecondary {
		bestPrimary = be.expectedZeros
		bestSecondary = be.expectedSecondaryZeros
	}
	return float64(bestPrimary)/float64(units) >= 0.75 && float64(bestSecondary)/float64(units) >= 0.5
}
