package execution

import (
	"runtime"
	"testing"
)

func TestBuildShellCommandPreservesPowerShellCommandText(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows PowerShell payload regression")
	}
	command := `& { $ErrorActionPreference = "Stop"; $value = "abc"; Write-Output "VALUE=$value" }`
	_, arguments, err := BuildShellCommand("powershell", command)
	if err != nil {
		t.Fatal(err)
	}
	if len(arguments) == 0 || arguments[len(arguments)-1] != command {
		t.Fatalf("command argument = %#v, want exact payload %q", arguments, command)
	}
}

func TestValidateShellRejectsWindowsExecutableNames(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows shell contract")
	}
	for _, shell := range []string{"powershell.exe", "pwsh.exe", "cmd.exe"} {
		t.Run(shell, func(t *testing.T) {
			if err := ValidateShell(shell); err == nil {
				t.Fatalf("ValidateShell(%q) succeeded, want unsupported logical shell name", shell)
			}
		})
	}
}

func TestBuildScriptCommandKeepsArgumentsSeparate(t *testing.T) {
	script := `C:\allowed\tool.exe`
	input := []string{"value with spaces", `$(untrusted)`, `& whoami`}
	program, arguments, err := BuildScriptCommand(script, input)
	if err != nil {
		t.Fatal(err)
	}
	if program != script || len(arguments) != len(input) {
		t.Fatalf("program=%q arguments=%#v", program, arguments)
	}
	for index := range input {
		if arguments[index] != input[index] {
			t.Fatalf("argument %d = %q, want %q", index, arguments[index], input[index])
		}
	}
	input[0] = "modified"
	if arguments[0] == input[0] {
		t.Fatal("BuildScriptCommand retained the caller's argument slice")
	}
}
