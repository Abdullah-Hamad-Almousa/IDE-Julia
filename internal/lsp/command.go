package lsp

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/jide-dev/jide/internal/jlproc"
)

// Command describes the process that runs a language server.
type Command struct {
	// Executable is the path of the program.
	Executable string
	// Args are the arguments of the program.
	Args []string
	// Dir is the working directory of the process.
	Dir string
	// Env is the environment of the process. A nil environment inherits the
	// environment of the engine.
	Env []string
}

// JuliaCommand returns the command that starts LanguageServer.jl with the
// given Julia runtime. The language-server package environment is separate
// from the Julia analysis environment of the workspace.
//
// The startup file of Julia is disabled, so that the configuration of the
// user cannot break the session. LanguageServer.jl runs from its tooling
// project while the workspace root remains the process working directory.
func JuliaCommand(runtime jlproc.Runtime, projectPath string) Command {
	workspaceRoot := "."
	if projectPath != "" {
		workspaceRoot = filepath.Dir(projectPath)
	}
	return juliaCommand(runtime, projectPath, workspaceRoot)
}

// juliaProjectPathForWorkspace reports the Julia environment selected for a
// workspace root. Source-only workspaces use Julia's environment autodetection.
func juliaProjectPathForWorkspace(projectPath, workspaceRoot string) string {
	if projectPath != "" {
		return filepath.Dir(projectPath)
	}
	if hasJuliaEnvironmentMarker(workspaceRoot) {
		return workspaceRoot
	}
	return ""
}

func JuliaWorkspaceCommand(runtime jlproc.Runtime, projectPath, workspaceRoot string) Command {
	return juliaCommand(runtime, projectPath, workspaceRoot)
}

func juliaCommand(juliaRuntime jlproc.Runtime, projectPath, workspaceRoot string) Command {
	serverProject, err := serverProjectPath()
	if err != nil {
		serverProject = filepath.Join(os.TempDir(), "jide", "language-server")
	}
	_ = os.MkdirAll(filepath.Join(serverProject, "symbolcache"), 0o700)
	args := []string{"--startup-file=no", "--history-file=no", "--color=no", "--project=" + serverProject}
	envPath := juliaProjectPathForWorkspace(projectPath, workspaceRoot)
	serverCode := `using LanguageServer
server = LanguageServer.LanguageServerInstance(stdin, stdout, ARGS[1], nothing, ARGS[2])
run(server)`
	args = append(args, "-e", serverCode, "--", envPath, filepath.Join(serverProject, "symbolcache"))
	return Command{Executable: juliaRuntime.Executable, Args: args, Dir: workspaceRoot}
}

func hasJuliaEnvironmentMarker(root string) bool {
	for _, name := range []string{"Project.toml", "JuliaProject.toml"} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			return true
		}
	}
	return false
}

func serverProjectPath() (string, error) {
	config, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("lsp: locate the per-user config directory: %w", err)
	}
	return filepath.Join(config, "jide", "language-server"), nil
}

func ensureLanguageServer(ctx context.Context, command Command) error {
	serverProject, err := serverProjectPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(serverProject, "symbolcache"), 0o700); err != nil {
		return fmt.Errorf("lsp: create the language-server environment: %w", err)
	}
	args := []string{"--startup-file=no", "--history-file=no", "--color=no", "--project=" + serverProject, "-e", `using Pkg; Pkg.activate(ARGS[1]); if !haskey(Pkg.project().dependencies, "LanguageServer"); Pkg.add("LanguageServer"); end`, "--", serverProject}
	installer := exec.CommandContext(ctx, command.Executable, args...)
	installer.Dir = command.Dir
	installer.Env = command.Env
	output, err := installer.CombinedOutput()
	if err != nil {
		return fmt.Errorf("lsp: install LanguageServer.jl in the per-user environment: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// FileURI returns the file URI of a path.
//
// The path is resolved to an absolute path first, so that the URI does not
// depend on the working directory of the process.
func FileURI(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		absolute = path
	}
	slashed := filepath.ToSlash(absolute)
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed
	}
	if runtime.GOOS == "windows" && len(slashed) >= 3 && slashed[2] == ':' {
		slashed = "/" + strings.ToLower(slashed[1:2]) + slashed[2:]
	}
	escaped := (&url.URL{Path: slashed}).EscapedPath()
	if runtime.GOOS == "windows" && len(slashed) >= 3 && slashed[2] == ':' {
		escaped = strings.Replace(escaped, "%3A", ":", 1)
	}
	// Julia's URI parser treats an escaped colon in the drive-letter segment
	// as a non-canonical path and later emits a different URI for diagnostics.
	return "file://" + escaped
}

// PathFromURI returns the path of a file URI.
//
// A URI that is not a file URI is returned unchanged.
func PathFromURI(uri string) string {
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != "file" {
		return uri
	}
	path := parsed.Path
	if runtime.GOOS == "windows" && len(path) > 2 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return filepath.FromSlash(path)
}
