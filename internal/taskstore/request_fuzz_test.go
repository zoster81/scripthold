package taskstore

import (
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"
)

func FuzzValidateRequest(f *testing.F) {
	validDigest := strings.Repeat("a", 64)
	absoluteWork := filepath.Join(string(filepath.Separator), "work")
	absoluteScript := filepath.Join(string(filepath.Separator), "script.ps1")
	f.Add("shell", "key", absoluteWork, "echo ok", "", "", 0)
	f.Add("script", "key", absoluteWork, "", absoluteScript, validDigest, 1)

	limits := Limits{MaxRuntimeSeconds: 3600}
	f.Fuzz(func(t *testing.T, kind, idempotencyKey, workingDirectory, command, scriptPath, scriptDigest string, maxRuntimeSeconds int) {
		request := Request{
			Kind:              Kind(kind),
			IdempotencyKey:    idempotencyKey,
			WorkingDirectory:  workingDirectory,
			Command:           command,
			ScriptPath:        scriptPath,
			ScriptDigest:      scriptDigest,
			MaxRuntimeSeconds: maxRuntimeSeconds,
		}
		err := validateRequest(request, limits)
		if err != nil {
			return
		}

		if request.Kind != KindShell && request.Kind != KindScript {
			t.Fatalf("validateRequest accepted unsupported kind %q", request.Kind)
		}
		if request.IdempotencyKey == "" || !filepath.IsAbs(request.WorkingDirectory) {
			t.Fatalf("validateRequest accepted invalid identity/path: %#v", request)
		}
		if request.MaxRuntimeSeconds < 0 || request.MaxRuntimeSeconds > limits.MaxRuntimeSeconds {
			t.Fatalf("validateRequest accepted runtime %d outside configured bounds", request.MaxRuntimeSeconds)
		}
		if request.Kind == KindShell {
			if strings.TrimSpace(request.Command) == "" || request.ScriptPath != "" || request.ScriptDigest != "" {
				t.Fatalf("validateRequest accepted cross-kind shell request: %#v", request)
			}
			return
		}
		if !filepath.IsAbs(request.ScriptPath) || request.Command != "" || len(request.ScriptDigest) != 64 {
			t.Fatalf("validateRequest accepted malformed script request: %#v", request)
		}
		if _, err := hex.DecodeString(request.ScriptDigest); err != nil {
			t.Fatalf("validateRequest accepted non-hex script digest %q", request.ScriptDigest)
		}
	})
}
