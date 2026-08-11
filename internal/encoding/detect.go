package encoding

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"unicode/utf8"

	"github.com/zoster81/scripthold/internal/operation"
)

// Detection constants
const (
	ChunkSize               = 128 * 1024 // 128KB chunks for detection
	SmallFileThreshold      = 128 * 1024 // Files smaller than this are read entirely
	HighConfidenceThreshold = 80         // Confidence level to stop sampling early
	MinConfidenceThreshold  = 50         // Minimum confidence to trust detection
)

// GBK two-byte ranges: lead 0x81–0xFE, trail 0x40–0xFE except 0x7F.
const (
	gbkLeadMin       = 0x81
	gbkLeadMax       = 0xFE
	gbkTrailMin      = 0x40
	gbkTrailMax      = 0xFE
	gbkTrailGap      = 0x7F
	gbkConfidenceCap = 85 // cap when GBK is recovered from a Latin guess
)

// DetectionResult holds encoding detection result.
type DetectionResult struct {
	Charset    string
	Confidence int
	HasBOM     bool
}

// DetectBOM checks the registry's BOM-capable descriptors. The registry keeps
// longer signatures first so UTF-32 LE wins over its UTF-16 LE prefix.
func DetectBOM(data []byte) (DetectionResult, bool) {
	for _, descriptor := range bomDescriptors {
		if len(data) >= len(descriptor.BOM) && bytes.Equal(data[:len(descriptor.BOM)], descriptor.BOM) {
			return DetectionResult{Charset: descriptor.Name, Confidence: 100, HasBOM: true}, true
		}
	}
	return DetectionResult{}, false
}

// --- Primary API (file-based, streaming) ---

// DetectFromFile detects encoding from a file path using streaming I/O.
// Modes: "sample" (~384KB max), "chunked" (streams entire file), "full" (loads entire file).
func DetectFromFile(path string, mode string) (result DetectionResult, err error) {
	defer func() {
		err = operation.WrapFilesystem("detect_encoding", path, err)
	}()

	file, err := os.Open(path)
	if err != nil {
		return DetectionResult{}, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return DetectionResult{}, fmt.Errorf("failed to stat file: %w", err)
	}

	return detectFromReader(file, stat.Size(), mode)
}

// Detect detects encoding from a byte slice.
func Detect(data []byte) DetectionResult {
	if result, ok := DetectBOM(data); ok {
		return result
	}
	if mayContainUTF32(data) {
		if result, handled := detectUTF32(data); handled {
			return result
		}
	}
	if mayContainUTF16(data) {
		if result, handled := detectUTF16(data); handled {
			return result
		}
	}
	return detectLegacy(data)
}

func mayContainUTF32(data []byte) bool {
	return len(data) >= 16 && bytes.IndexByte(data, 0) >= 0
}

func mayContainUTF16(data []byte) bool {
	return len(data) >= 4 && (!utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0)
}

func detectLegacy(data []byte) DetectionResult {
	if len(data) == 0 {
		return DetectionResult{Charset: "utf-8", Confidence: 100}
	}
	if result, handled := detectStructuredLegacy(data); handled {
		return result
	}
	if isLikelyBinaryBytes(data) {
		return DetectionResult{}
	}
	if utf8.Valid(data) && bytes.IndexByte(data, 0) < 0 {
		return DetectionResult{Charset: "utf-8", Confidence: 100}
	}
	return detectLegacyCandidates(data)
}

func isLikelyBinaryBytes(data []byte) bool {
	if bytes.IndexByte(data, 0) >= 0 {
		return true
	}
	controls := 0
	for _, current := range data {
		if current < 0x20 && current != '\n' && current != '\r' && current != '\t' {
			controls++
		}
	}
	return len(data) >= 8 && controls*10 >= len(data)
}

// looksLikeGBK reports whether data holds enough valid GBK two-byte sequences,
// biased toward the common-hanzi lead range (0xB0–0xD7), to trust it over Latin.
func looksLikeGBK(data []byte) bool {
	const minSequences = 5
	const minCommonRatio = 0.2

	var total, common int
	for i := 0; i+1 < len(data); {
		lead, trail := data[i], data[i+1]
		if lead >= gbkLeadMin && lead <= gbkLeadMax &&
			trail >= gbkTrailMin && trail <= gbkTrailMax && trail != gbkTrailGap {
			total++
			if lead >= 0xB0 && lead <= 0xD7 {
				common++
			}
			i += 2
			continue
		}
		i++
	}

	return total >= minSequences && float64(common)/float64(total) > minCommonRatio
}

// detectSample detects encoding by sampling beginning, middle, and end of an
// already-loaded byte slice. File-based consumers use DetectFromReaderAt.
func detectSample(data []byte) (DetectionResult, bool) {
	size := len(data)
	if size <= SmallFileThreshold {
		result := Detect(data)
		return result, result.Confidence >= MinConfidenceThreshold
	}
	if result, ok := DetectBOM(data); ok {
		return result, true
	}

	samples := detectionSamplesFromData(data)
	if result, handled := detectUTF32Samples(samples, int64(size)); handled {
		return result, result.Confidence >= MinConfidenceThreshold
	}
	if result, handled := detectUTF16Samples(samples, int64(size)); handled {
		return result, result.Confidence >= MinConfidenceThreshold
	}

	result := detectLegacySamples(samples)
	return result, result.Confidence >= MinConfidenceThreshold
}

func detectionSamplesFromData(data []byte) []byteSample {
	size := len(data)
	samples := []byteSample{{data: data[:min(ChunkSize, size)], offset: 0}}

	if size > ChunkSize*2 {
		middle := (size - ChunkSize) / 2
		middle -= middle % 4
		samples = append(samples, byteSample{data: data[middle : middle+ChunkSize], offset: int64(middle)})
	}
	if size > ChunkSize {
		end := size - ChunkSize
		end -= end % 4
		samples = append(samples, byteSample{data: data[end:], offset: int64(end)})
	}
	return samples
}

func joinDetectionSamples(samples []byteSample) []byte {
	total := 0
	for _, sample := range samples {
		total += len(sample.data)
	}
	joined := make([]byte, 0, total)
	for _, sample := range samples {
		joined = append(joined, sample.data...)
	}
	return joined
}

// DetectFromReaderAt detects encoding from a random-access source without
// taking ownership of it. The caller supplies the stable byte size used by the
// selected detection mode.
func DetectFromReaderAt(r io.ReaderAt, size int64, mode string) (DetectionResult, error) {
	return detectFromReader(r, size, mode)
}

// --- Internal streaming implementation ---

func detectFromReader(r io.ReaderAt, size int64, mode string) (DetectionResult, error) {
	switch mode {
	case "sample":
		return detectSampleFromReader(r, size)
	case "chunked":
		return detectChunkedFromReader(r, size)
	case "full":
		return detectFullFromReader(r, size)
	default:
		return DetectionResult{}, operation.Wrap(
			operation.KindInvalidInput,
			"detect_encoding",
			"",
			fmt.Errorf("invalid mode: %s (valid: sample, chunked, full)", mode),
		)
	}
}

func detectSampleFromReader(r io.ReaderAt, size int64) (DetectionResult, error) {
	if size <= SmallFileThreshold {
		data := make([]byte, size)
		if _, err := r.ReadAt(data, 0); err != nil && err != io.EOF {
			return DetectionResult{}, fmt.Errorf("failed to read file: %w", err)
		}
		return Detect(data), nil
	}

	samples, err := readDetectionSamples(r, size)
	if err != nil {
		return DetectionResult{}, err
	}
	if result, ok := DetectBOM(samples[0].data); ok {
		return result, nil
	}
	if result, handled := detectUTF32Samples(samples, size); handled {
		return result, nil
	}
	if result, handled := detectUTF16Samples(samples, size); handled {
		return result, nil
	}

	return detectLegacySamples(samples), nil
}

func readDetectionSamples(r io.ReaderAt, size int64) ([]byteSample, error) {
	offsets := []int64{0}
	if size > int64(ChunkSize*2) {
		middle := (size - int64(ChunkSize)) / 2
		middle -= middle % 4
		offsets = append(offsets, middle)
	}
	if size > int64(ChunkSize) {
		end := size - int64(ChunkSize)
		end -= end % 4
		offsets = append(offsets, end)
	}

	samples := make([]byteSample, 0, len(offsets))
	for _, offset := range offsets {
		length := min(int64(ChunkSize), size-offset)
		if offset == offsets[len(offsets)-1] {
			length = size - offset
		}
		data := make([]byte, int(length))
		n, err := r.ReadAt(data, offset)
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("failed to read sample at %d: %w", offset, err)
		}
		samples = append(samples, byteSample{data: data[:n], offset: offset})
	}
	return samples, nil
}

func detectChunkedFromReader(r io.ReaderAt, size int64) (DetectionResult, error) {
	if size <= int64(ChunkSize) {
		data := make([]byte, size)
		if _, err := r.ReadAt(data, 0); err != nil && err != io.EOF {
			return DetectionResult{}, fmt.Errorf("failed to read file: %w", err)
		}
		return Detect(data), nil
	}

	bomCheck := make([]byte, 4)
	if n, _ := r.ReadAt(bomCheck, 0); n >= 2 {
		if result, ok := DetectBOM(bomCheck[:n]); ok {
			return result, nil
		}
	}

	type chunkResult struct {
		encoding   string
		confidence int
		weight     int
	}

	utf32LEAnalyzer := newUTF32Analyzer(utf32LESpec)
	utf32BEAnalyzer := newUTF32Analyzer(utf32BESpec)
	leAnalyzer := newUTF16Analyzer(utf16LESpec)
	beAnalyzer := newUTF16Analyzer(utf16BESpec)
	var legacyStructure legacyStructureScanner
	var results []chunkResult
	chunk := make([]byte, ChunkSize)

	for offset := int64(0); offset < size; {
		n, err := r.ReadAt(chunk, offset)
		if err != nil && err != io.EOF {
			return DetectionResult{}, fmt.Errorf("failed to read chunk at %d: %w", offset, err)
		}
		if n == 0 {
			break
		}

		data := chunk[:n]
		utf32LEAnalyzer.Write(data)
		utf32BEAnalyzer.Write(data)
		leAnalyzer.Write(data)
		beAnalyzer.Write(data)
		legacyStructure.Feed(data)
		detected := detectLegacy(data)
		if detected.Charset != "" {
			results = append(results, chunkResult{
				encoding:   detected.Charset,
				confidence: detected.Confidence,
				weight:     n,
			})
		}
		offset += int64(n)
	}

	if result, handled := decideUTF32(utf32LEAnalyzer.Finish(), utf32BEAnalyzer.Finish()); handled {
		return result, nil
	}
	if result, handled := decideUTF16(leAnalyzer.Finish(), beAnalyzer.Finish()); handled {
		return result, nil
	}
	if result, handled := legacyStructure.Decide(func() io.Reader { return io.NewSectionReader(r, 0, size) }); handled {
		return result, nil
	}
	if len(results) == 0 {
		return DetectionResult{}, nil
	}

	encodingWeights := make(map[string]int)
	encodingConfidenceSum := make(map[string]int)
	for _, result := range results {
		encodingWeights[result.encoding] += result.weight
		encodingConfidenceSum[result.encoding] += result.confidence * result.weight
	}

	var bestEncoding string
	var bestWeight int
	for encoding, weight := range encodingWeights {
		if weight > bestWeight || weight == bestWeight && (bestEncoding == "" || encoding < bestEncoding) {
			bestWeight = weight
			bestEncoding = encoding
		}
	}

	return DetectionResult{
		Charset:    bestEncoding,
		Confidence: encodingConfidenceSum[bestEncoding] / encodingWeights[bestEncoding],
	}, nil
}

func detectFullFromReader(r io.ReaderAt, size int64) (DetectionResult, error) {
	data := make([]byte, size)
	if _, err := r.ReadAt(data, 0); err != nil && err != io.EOF {
		return DetectionResult{}, fmt.Errorf("failed to read file: %w", err)
	}
	return Detect(data), nil
}
