package execution

import "testing"

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
