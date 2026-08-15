package execution

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// BuildScriptCommand maps a supported script file to a direct executable and
// argument vector. It never invokes a shell to concatenate arguments.
func BuildScriptCommand(scriptPath string, scriptArgs []string) (string, []string, error) {
	extension := strings.ToLower(filepath.Ext(scriptPath))
	switch extension {
	case ".ps1":
		program, err := firstExecutable("pwsh.exe", "pwsh", "powershell.exe", "powershell")
		if err != nil {
			return "", nil, fmt.Errorf("PowerShell was not found: %w", err)
		}
		return program, append([]string{"-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", scriptPath}, scriptArgs...), nil
	case ".bat", ".cmd":
		if runtime.GOOS != "windows" {
			return "", nil, fmt.Errorf("%s scripts are supported only on Windows", extension)
		}
		program, err := firstExecutable("cmd.exe", "cmd")
		if err != nil {
			return "", nil, fmt.Errorf("cmd.exe was not found: %w", err)
		}
		return program, append([]string{"/d", "/s", "/c", scriptPath}, scriptArgs...), nil
	case ".py":
		if program, err := firstExecutable("py.exe", "py"); err == nil {
			return program, append([]string{"-3", scriptPath}, scriptArgs...), nil
		}
		program, err := firstExecutable("python.exe", "python3", "python")
		if err != nil {
			return "", nil, fmt.Errorf("python was not found: %w", err)
		}
		return program, append([]string{scriptPath}, scriptArgs...), nil
	case ".js", ".mjs", ".cjs":
		program, err := firstExecutable("node.exe", "node")
		if err != nil {
			return "", nil, fmt.Errorf("node.js was not found: %w", err)
		}
		return program, append([]string{scriptPath}, scriptArgs...), nil
	case ".sh":
		program, err := firstExecutable("bash.exe", "bash")
		if err != nil {
			return "", nil, fmt.Errorf("bash was not found: %w", err)
		}
		return program, append([]string{scriptPath}, scriptArgs...), nil
	case ".exe", ".com":
		return scriptPath, append([]string(nil), scriptArgs...), nil
	default:
		return "", nil, fmt.Errorf("unsupported script type %q; supported extensions: .ps1, .bat, .cmd, .py, .js, .mjs, .cjs, .sh, .exe, .com", extension)
	}
}

// ValidateShell checks whether the requested logical shell name is supported on
// the current platform without resolving an executable. It is safe to use at
// admission time before durable task creation.
func ValidateShell(requestedShell string) error {
	_, err := normalizeShell(requestedShell)
	return err
}

func normalizeShell(requestedShell string) (string, error) {
	shell := strings.ToLower(strings.TrimSpace(requestedShell))
	if runtime.GOOS == "windows" {
		switch shell {
		case "", "powershell", "windows-powershell":
			return "powershell", nil
		case "pwsh", "powershell-core":
			return "pwsh", nil
		case "cmd":
			return "cmd", nil
		default:
			return "", fmt.Errorf("unsupported shell %q on Windows; use powershell, pwsh, or cmd", requestedShell)
		}
	}
	switch shell {
	case "":
		return "sh", nil
	case "sh", "bash", "pwsh", "powershell":
		return shell, nil
	default:
		return "", fmt.Errorf("unsupported shell %q; use sh, bash, or pwsh", requestedShell)
	}
}

// BuildShellCommand maps a requested shell to a fixed executable invocation.
func BuildShellCommand(requestedShell, command string) (string, []string, error) {
	shell, err := normalizeShell(requestedShell)
	if err != nil {
		return "", nil, err
	}
	if runtime.GOOS == "windows" {
		switch shell {
		case "powershell":
			program, err := firstExecutable("powershell.exe", "powershell")
			if err != nil {
				return "", nil, fmt.Errorf("windows powershell was not found: %w", err)
			}
			return program, []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", command}, nil
		case "pwsh":
			program, err := firstExecutable("pwsh.exe", "pwsh")
			if err != nil {
				return "", nil, fmt.Errorf("PowerShell 7 was not found: %w", err)
			}
			return program, []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", command}, nil
		case "cmd":
			program, err := firstExecutable("cmd.exe", "cmd")
			if err != nil {
				return "", nil, fmt.Errorf("cmd.exe was not found: %w", err)
			}
			return program, []string{"/d", "/s", "/c", command}, nil
		}
	}
	switch shell {
	case "sh", "bash":
		program, err := firstExecutable(shell)
		if err != nil {
			return "", nil, fmt.Errorf("%s was not found: %w", shell, err)
		}
		return program, []string{"-c", command}, nil
	case "pwsh", "powershell":
		program, err := firstExecutable("pwsh", "powershell")
		if err != nil {
			return "", nil, fmt.Errorf("PowerShell was not found: %w", err)
		}
		return program, []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", command}, nil
	}
	return "", nil, fmt.Errorf("unsupported normalized shell %q", shell)
}

func firstExecutable(candidates ...string) (string, error) {
	var lastErr error
	for _, candidate := range candidates {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		} else {
			lastErr = err
		}
	}
	return "", lastErr
}
