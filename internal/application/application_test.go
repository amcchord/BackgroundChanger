package application

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amcchord/WallpaperIdentity/v4/internal/setup"
)

func TestParseCommand(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    commandOptions
		wantErr string
	}{
		{name: "gui", want: commandOptions{Name: "gui"}},
		{name: "windows help", args: []string{"/?"}, want: commandOptions{Name: "help"}},
		{name: "legacy install", args: []string{"--install", "--quiet", "--preset", "BALANCED"}, want: commandOptions{Name: "install", Preset: "balanced", Quiet: true, Headless: true}},
		{name: "windows silent alias", args: []string{"/install", "/silent", "/preset=identity"}, want: commandOptions{Name: "install", Preset: "identity", Quiet: true, Headless: true}},
		{name: "upgrade alias", args: []string{"upgrade", "--headless"}, want: commandOptions{Name: "repair", Headless: true}},
		{name: "json implies headless", args: []string{"status", "--json"}, want: commandOptions{Name: "status", JSON: true, Headless: true}},
		{name: "legacy render path", args: []string{"--render", `C:\Temp\preview image.jpg`}, want: commandOptions{Name: "render", Output: `C:\Temp\preview image.jpg`}},
		{name: "render options after path", args: []string{"render", `C:\Temp\preview.jpg`, "--json"}, want: commandOptions{Name: "render", Output: `C:\Temp\preview.jpg`, JSON: true, Headless: true}},
		{name: "remove data", args: []string{"uninstall", "--headless", "--remove-data"}, want: commandOptions{Name: "uninstall", Headless: true, RemoveData: true}},
		{name: "RMM result file implies headless", args: []string{"install", "--result-file", `C:\RMM Results\wid.json`}, want: commandOptions{Name: "install", Headless: true, ResultFile: `C:\RMM Results\wid.json`}},
		{name: "slash JSON value", args: []string{"status", "/json=true"}, want: commandOptions{Name: "status", JSON: true, Headless: true}},
		{name: "preset and config conflict", args: []string{"install", "--preset", "identity", "--config", "config.yml"}, wantErr: "mutually exclusive"},
		{name: "bad preset", args: []string{"install", "--preset", "typo"}, wantErr: "unknown preset"},
		{name: "missing value", args: []string{"install", "--config"}, wantErr: "needs an argument"},
		{name: "unknown option", args: []string{"uninstall", "--quite"}, wantErr: "flag provided but not defined"},
		{name: "extra argument", args: []string{"status", "extra"}, wantErr: "unexpected arguments"},
		{name: "render output conflict", args: []string{"render", "--output", "one.jpg", "two.jpg"}, wantErr: "either --output"},
		{name: "result overwrites config", args: []string{"install", "--config", "config.yml", "--result-file", "config.yml"}, wantErr: "must not overwrite --config"},
		{name: "result overwrites render", args: []string{"render", "--output", "preview.jpg", "--result-file", "preview.jpg"}, wantErr: "must not overwrite render output"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseCommand(test.args)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %v, want substring %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("options = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestCLINoArgsShowsHelpInsteadOfGUI(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := MainCLI(nil, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(stdout.String(), "WallpaperIdentityCLI.exe install") {
		t.Fatalf("help output missing CLI usage: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr %q", stderr.String())
	}
}

func TestCLIProcessMode(t *testing.T) {
	headless := applyProcessMode(commandOptions{Name: "install"}, true)
	if !headless.Headless {
		t.Fatal("CLI install must be headless by default")
	}
	interactive := applyProcessMode(commandOptions{Name: "uninstall", Interactive: true}, true)
	if interactive.Headless {
		t.Fatal("explicit interactive maintenance must remain interactive")
	}
}

func TestJSONRequestedAliases(t *testing.T) {
	for _, args := range [][]string{{"status", "--json"}, {"status", "--json=true"}, {"status", "/json"}, {"status", "/json=true"}} {
		if !jsonRequested(args) {
			t.Errorf("jsonRequested(%q) = false", args)
		}
	}
	if jsonRequested([]string{"status", "--json=false"}) {
		t.Fatal("--json=false must not request JSON-formatted parse errors")
	}
}

func TestExitCode(t *testing.T) {
	if got := exitCode(nil); got != exitOK {
		t.Fatalf("success exit code = %d", got)
	}
	if got := exitCode(fail(exitAdminRequired, "admin")); got != exitAdminRequired {
		t.Fatalf("typed exit code = %d", got)
	}
	if got := exitCode(errors.New("boom")); got != exitOperationError {
		t.Fatalf("operation exit code = %d", got)
	}
	if got := exitCode(setup.ErrSetupRunning); got != exitSetupRunning {
		t.Fatalf("concurrent setup exit code = %d", got)
	}
}

func TestInvalidJSONInvocationProducesOneDocument(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := mainExit([]string{"install", "--json", "--preset", "typo"}, &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("exit code = %d", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr %q", stderr.String())
	}
	var result commandResult
	decoder := json.NewDecoder(&stdout)
	if err := decoder.Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Success || result.ExitCode != exitUsage || result.Error == nil || result.Error.Message == "" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if decoder.Decode(&struct{}{}) == nil {
		t.Fatal("expected exactly one JSON document")
	}
}

func TestInvalidInvocationWritesRequestedResultFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.json")
	var stdout, stderr bytes.Buffer
	code := mainExit([]string{"install", "--result-file", path, "--preset", "typo"}, &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("exit code = %d", code)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var result commandResult
	if err := json.Unmarshal(b, &result); err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != exitUsage || result.Success || result.Error == nil || result.Error.Code != "invalid_command_line" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestInvalidInvocationDoesNotOverwriteConflictingInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := mainExit([]string{"install", "--config", path, "--result-file", path}, &stdout, &stderr); code != exitUsage {
		t.Fatalf("exit code = %d", code)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "keep me" {
		t.Fatalf("input file was overwritten: %q", b)
	}
}

func TestInvalidInvocationDoesNotOverwritePositionalRenderOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preview.jpg")
	if err := os.WriteFile(path, []byte("jpeg placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := mainExit([]string{"render", path, "--result-file", path, "extra"}, &stdout, &stderr); code != exitUsage {
		t.Fatalf("exit code = %d", code)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "jpeg placeholder" {
		t.Fatalf("render output was overwritten: %q", b)
	}
}

func TestHumanOutputMentionsRequiredReboot(t *testing.T) {
	var output bytes.Buffer
	writeHuman(&output, commandResult{Message: "Done.", RebootRequired: true})
	if !strings.Contains(output.String(), "Reboot required: yes") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestWriteResultFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "result.json")
	want := commandResult{SchemaVersion: 1, Operation: "install", Success: true, ExitCode: 0, Action: "installed", Changed: true}
	if err := writeResultFile(path, want); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got commandResult
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != want.SchemaVersion || got.Operation != want.Operation || got.Action != want.Action || !got.Success || !got.Changed {
		t.Fatalf("result = %#v", got)
	}
}
