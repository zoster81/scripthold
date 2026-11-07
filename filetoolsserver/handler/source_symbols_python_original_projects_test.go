package handler

import (
	"os"
	"testing"
)

// The first two independent Python provider projects are inspected read only.
// No original project tests, HTTP requests or CLI commands are executed.
func TestSourceSymbolsOriginalPythonInitialQualification(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Python corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "python-click", language: "python",
			search: "wrap_text", primary: "src/click/formatting.py",
			negativeFile: "src/click/exceptions.py", negativeQuery: "wrap_text",
			paths: []string{"src/click/formatting.py", "tests/test_formatting.py", "src/click/exceptions.py"},
			oracles: []extraAuditEvidence{
				{"src/click/formatting.py", "def wrap_text("},
				{"src/click/formatting.py", "from ._textwrap import TextWrapper"},
				{"src/click/formatting.py", "preserve_paragraphs: bool = False"},
				{"src/click/formatting.py", "class HelpFormatter:"},
				{"tests/test_formatting.py", "def test_basic_functionality(runner):"},
				{"tests/test_formatting.py", "result = runner.invoke(cli, [\"--help\"], terminal_width=60)"},
				{"src/click/core.py", "from .formatting import HelpFormatter"},
				{"src/click/__init__.py", "from .formatting import wrap_text as wrap_text"},
				{"src/click/exceptions.py", "class ClickException"},
			},
			targets: []extraAuditSymbol{
				{"src/click/formatting.py", "wrap_text", 31},
				{"src/click/formatting.py", "HelpFormatter", 110},
				{"tests/test_formatting.py", "test_basic_functionality", 7},
				{"tests/test_formatting.py", "test_wrapping_long_options_strings", 55},
			},
		},
		{
			project: "python-httpx", language: "python",
			search: "DeflateDecoder", primary: "httpx/_decoders.py",
			negativeFile: "httpx/_multipart.py", negativeQuery: "DeflateDecoder",
			paths: []string{"httpx/_decoders.py", "tests/test_decoders.py", "httpx/_multipart.py"},
			oracles: []extraAuditEvidence{
				{"httpx/_decoders.py", "class DeflateDecoder(ContentDecoder):"},
				{"httpx/_decoders.py", "zlib.decompressobj(-zlib.MAX_WBITS)"},
				{"httpx/_decoders.py", "class GZipDecoder(ContentDecoder):"},
				{"httpx/_decoders.py", "\"deflate\": DeflateDecoder"},
				{"tests/test_decoders.py", "def test_deflate():"},
				{"tests/test_decoders.py", "def test_zlib():"},
				{"tests/test_decoders.py", "headers = [(b\"Content-Encoding\", b\"deflate\")]"},
				{"httpx/_models.py", "def _get_content_decoder(self) -> ContentDecoder:"},
				{"httpx/_multipart.py", "class"},
			},
			targets: []extraAuditSymbol{
				{"httpx/_decoders.py", "DeflateDecoder", 56},
				{"httpx/_decoders.py", "GZipDecoder", 85},
				{"tests/test_decoders.py", "test_deflate", 14},
				{"tests/test_decoders.py", "test_zlib", 33},
				{"tests/test_decoders.py", "test_gzip", 51},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
