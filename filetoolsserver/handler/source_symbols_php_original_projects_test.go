package handler

import (
	"os"
	"testing"
)

// Independent original PHP navigation scenarios; no HTTP calls, mail delivery,
// original project tests, or PHP interpreter execution.
func TestSourceSymbolsOriginalPHPInitialQualification(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original PHP corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "php-guzzle-guzzle", language: "php",
			search: "resolve", primary: "src/HandlerStack.php",
			negativeFile: "src/MessageFormatter.php", negativeQuery: "resolve",
			paths: []string{
				"src/HandlerStack.php",
				"tests/HandlerStackTest.php",
				"src/MessageFormatter.php",
			},
			oracles: []extraAuditEvidence{
				{"src/HandlerStack.php", "class HandlerStack"},
				{"src/HandlerStack.php", "public function push(callable $middleware, string $name = ''): void"},
				{"src/HandlerStack.php", "public function resolve(): callable"},
				{"src/HandlerStack.php", "foreach (\\array_reverse($this->stack) as $fn)"},
				{"src/HandlerStack.php", "throw new \\LogicException('Middleware must be callable');"},
				{"src/HandlerStack.php", "throw new \\LogicException('Middleware must return a callable');"},
				{"tests/HandlerStackTest.php", "public function testPushInOrder(): void"},
				{"tests/HandlerStackTest.php", "public function testResolveRejectsNonCallableMiddleware(): void"},
				{"tests/HandlerStackTest.php", "public function testResolveRejectsMiddlewareReturningNonCallable(): void"},
				{"src/MessageFormatter.php", "final class MessageFormatter implements MessageFormatterInterface"},
				{"src/Client.php", "class Client implements ClientInterface"},
			},
			targets: []extraAuditSymbol{
				{"src/HandlerStack.php", "HandlerStack", 20},
				{"src/HandlerStack.php", "push", 129},
				{"src/HandlerStack.php", "resolve", 200},
				{"src/HandlerStack.php", "before", 142},
				{"tests/HandlerStackTest.php", "testPushInOrder", 117},
				{"tests/HandlerStackTest.php", "testResolveRejectsNonCallableMiddleware", 63},
				{"tests/HandlerStackTest.php", "testResolveRejectsMiddlewareReturningNonCallable", 102},
			},
		},
		{
			project: "php-phpmailer-phpmailer", language: "php",
			search: "configureSMTP", primary: "src/DSNConfigurator.php",
			negativeFile: "src/OAuth.php", negativeQuery: "configureSMTP",
			paths: []string{
				"src/DSNConfigurator.php",
				"test/PHPMailer/DSNConfiguratorTest.php",
				"src/OAuth.php",
			},
			oracles: []extraAuditEvidence{
				{"src/DSNConfigurator.php", "class DSNConfigurator"},
				{"src/DSNConfigurator.php", "public static function mailer($dsn, $exceptions = null)"},
				{"src/DSNConfigurator.php", "public function configure(PHPMailer $mailer, $dsn)"},
				{"src/DSNConfigurator.php", "private function parseDSN($dsn)"},
				{"src/DSNConfigurator.php", "throw new Exception('Malformed DSN');"},
				{"src/DSNConfigurator.php", "private function configureSMTP($mailer, $config)"},
				{"src/DSNConfigurator.php", "$mailer->Port = SMTP::DEFAULT_SECURE_PORT;"},
				{"test/PHPMailer/DSNConfiguratorTest.php", "public function testInvalidDSN()"},
				{"test/PHPMailer/DSNConfiguratorTest.php", "public function testConfigureSmtpWithoutAuthentication()"},
				{"test/PHPMailer/DSNConfiguratorTest.php", "public function testConfigureSmtpsWithoutPort()"},
				{"src/SMTP.php", "const DEFAULT_SECURE_PORT = 465;"},
				{"src/OAuth.php", "class OAuth implements OAuthTokenProvider"},
			},
			targets: []extraAuditSymbol{
				{"src/DSNConfigurator.php", "DSNConfigurator", 31},
				{"src/DSNConfigurator.php", "mailer", 41},
				{"src/DSNConfigurator.php", "configure", 60},
				{"src/DSNConfigurator.php", "parseDSN", 78},
				{"src/DSNConfigurator.php", "applyConfig", 101},
				{"src/DSNConfigurator.php", "configureSMTP", 138},
				{"test/PHPMailer/DSNConfiguratorTest.php", "testInvalidDSN", 32},
				{"test/PHPMailer/DSNConfiguratorTest.php", "testConfigureSmtpWithoutAuthentication", 94},
				{"test/PHPMailer/DSNConfiguratorTest.php", "testConfigureSmtpsWithoutPort", 153},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
