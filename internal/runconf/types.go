package runconf

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/jide-dev/jide/internal/jlproc"
)

// Built-in run configuration type identifiers.
const (
	TypeJuliaScript = "julia-script"
	TypeJuliaModule = "julia-module"
	TypeJuliaREPL   = "julia-repl"
)

var moduleNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`)

// Configuration is a JSON-marshalable run configuration. Properties carries
// type-specific JSON values for user-defined configuration types.
type Configuration struct {
	ID               string                     `json:"id,omitempty"`
	Name             string                     `json:"name"`
	Type             string                     `json:"type"`
	ProjectPath      string                     `json:"projectPath,omitempty"`
	WorkingDirectory string                     `json:"workingDirectory,omitempty"`
	Program          string                     `json:"program,omitempty"`
	Module           string                     `json:"module,omitempty"`
	Arguments        []string                   `json:"arguments,omitempty"`
	Environment      map[string]string          `json:"environment,omitempty"`
	Properties       map[string]json.RawMessage `json:"properties,omitempty"`
}

// Invocation is the fully resolved Julia process command produced by a type.
type Invocation struct {
	Executable       string
	Arguments        []string
	WorkingDirectory string
	Environment      map[string]string
	Interactive      bool
}

// ConfigType validates and builds one run-configuration type. ID is stable and
// unique within a Registry. Validate and Build must both be implemented.
type ConfigType struct {
	ID       string
	Validate func(Configuration) error
	Build    func(jlproc.Runtime, Configuration) (Invocation, error)
}

// Registry stores built-in and user-defined run-configuration types.
type Registry struct {
	mu    sync.RWMutex
	types map[string]ConfigType
}

// NewRegistry returns a registry with Julia script, module, and REPL types.
func NewRegistry() *Registry {
	registry := &Registry{types: make(map[string]ConfigType)}
	for _, configType := range builtinTypes() {
		registry.types[configType.ID] = configType
	}
	return registry
}

// Register adds a user-defined type. Existing identifiers are not replaced.
func (r *Registry) Register(configType ConfigType) error {
	if r == nil {
		return errors.New("runconf: registry is nil")
	}
	if strings.TrimSpace(configType.ID) == "" || configType.Validate == nil || configType.Build == nil {
		return errors.New("runconf: configuration type needs an id, validator, and builder")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.types[configType.ID]; exists {
		return fmt.Errorf("runconf: configuration type %q is already registered", configType.ID)
	}
	r.types[configType.ID] = configType
	return nil
}

// Types returns the registered type identifiers in stable order.
func (r *Registry) Types() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	types := make([]string, 0, len(r.types))
	for id := range r.types {
		types = append(types, id)
	}
	r.mu.RUnlock()
	sort.Strings(types)
	return types
}

// Build validates config and resolves it into a concrete Julia invocation.
func (r *Registry) Build(runtime jlproc.Runtime, config Configuration) (Invocation, error) {
	if r == nil {
		return Invocation{}, errors.New("runconf: registry is nil")
	}
	r.mu.RLock()
	configType, exists := r.types[config.Type]
	r.mu.RUnlock()
	if !exists {
		return Invocation{}, fmt.Errorf("runconf: unknown configuration type %q", config.Type)
	}
	if err := configType.Validate(config); err != nil {
		return Invocation{}, fmt.Errorf("runconf: invalid %s configuration: %w", config.Type, err)
	}
	invocation, err := configType.Build(runtime, config)
	if err != nil {
		return Invocation{}, fmt.Errorf("runconf: build %s configuration: %w", config.Type, err)
	}
	if strings.TrimSpace(invocation.Executable) == "" {
		return Invocation{}, errors.New("runconf: configuration produced an empty executable")
	}
	invocation.Arguments = slices.Clone(invocation.Arguments)
	invocation.Environment = cloneEnvironment(invocation.Environment)
	return invocation, nil
}

func builtinTypes() []ConfigType {
	return []ConfigType{
		{ID: TypeJuliaScript, Validate: validateScript, Build: buildScript},
		{ID: TypeJuliaModule, Validate: validateModule, Build: buildModule},
		{ID: TypeJuliaREPL, Validate: validateREPL, Build: buildREPL},
	}
}

func validateScript(config Configuration) error {
	if strings.TrimSpace(config.Program) == "" {
		return errors.New("program path is required")
	}
	if strings.ToLower(filepath.Ext(config.Program)) != ".jl" {
		return errors.New("program must be a .jl file")
	}
	info, err := os.Stat(config.Program)
	if err != nil {
		return fmt.Errorf("stat program: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("program is not a regular file")
	}
	if config.Module != "" {
		return errors.New("module must be empty for a script configuration")
	}
	return validatePaths(config)
}

func validateModule(config Configuration) error {
	if !moduleNamePattern.MatchString(config.Module) {
		return errors.New("module must be a Julia module name, such as Example or Parent.Child")
	}
	if config.Program != "" {
		return errors.New("program must be empty for a module configuration")
	}
	return validatePaths(config)
}

func validateREPL(config Configuration) error {
	if config.Program != "" || config.Module != "" || len(config.Arguments) != 0 {
		return errors.New("REPL configurations do not accept a program, module, or arguments")
	}
	return validatePaths(config)
}

func validatePaths(config Configuration) error {
	if config.ProjectPath != "" {
		project := config.ProjectPath
		if filepath.Base(project) != "Project.toml" {
			project = filepath.Join(project, "Project.toml")
		}
		info, err := os.Stat(project)
		if err != nil {
			return fmt.Errorf("stat Julia project: %w", err)
		}
		if !info.Mode().IsRegular() {
			return errors.New("Julia project is not a regular Project.toml")
		}
	}
	if config.WorkingDirectory != "" {
		info, err := os.Stat(config.WorkingDirectory)
		if err != nil {
			return fmt.Errorf("stat working directory: %w", err)
		}
		if !info.IsDir() {
			return errors.New("working directory is not a directory")
		}
	}
	for key := range config.Environment {
		if strings.TrimSpace(key) == "" || strings.ContainsAny(key, "=\x00") {
			return fmt.Errorf("invalid environment variable name %q", key)
		}
	}
	return nil
}

func buildScript(runtime jlproc.Runtime, config Configuration) (Invocation, error) {
	program, err := filepath.Abs(config.Program)
	if err != nil {
		return Invocation{}, err
	}
	args := juliaBaseArguments(config.ProjectPath)
	args = append(args, program)
	args = append(args, config.Arguments...)
	directory := config.WorkingDirectory
	if directory == "" {
		directory = filepath.Dir(program)
	}
	return Invocation{Executable: runtime.Executable, Arguments: args, WorkingDirectory: directory, Environment: config.Environment}, nil
}

func buildModule(runtime jlproc.Runtime, config Configuration) (Invocation, error) {
	args := juliaBaseArguments(config.ProjectPath)
	args = append(args, "-e", "using "+config.Module)
	args = append(args, config.Arguments...)
	directory := projectDirectory(config.ProjectPath)
	if config.WorkingDirectory != "" {
		directory = config.WorkingDirectory
	}
	return Invocation{Executable: runtime.Executable, Arguments: args, WorkingDirectory: directory, Environment: config.Environment}, nil
}

func buildREPL(runtime jlproc.Runtime, config Configuration) (Invocation, error) {
	args := juliaBaseArguments(config.ProjectPath)
	args = append(args, "--interactive")
	directory := projectDirectory(config.ProjectPath)
	if config.WorkingDirectory != "" {
		directory = config.WorkingDirectory
	}
	return Invocation{Executable: runtime.Executable, Arguments: args, WorkingDirectory: directory,
		Environment: config.Environment, Interactive: true}, nil
}

func juliaBaseArguments(projectPath string) []string {
	args := []string{"--startup-file=no", "--history-file=no", "--color=no"}
	if projectPath != "" {
		project := projectPath
		if filepath.Base(project) == "Project.toml" {
			project = filepath.Dir(project)
		}
		args = append(args, "--project="+project)
	}
	return args
}

func projectDirectory(projectPath string) string {
	if projectPath == "" {
		return ""
	}
	if filepath.Base(projectPath) == "Project.toml" {
		return filepath.Dir(projectPath)
	}
	return projectPath
}

func cloneEnvironment(environment map[string]string) map[string]string {
	if environment == nil {
		return nil
	}
	cloned := make(map[string]string, len(environment))
	for key, value := range environment {
		cloned[key] = value
	}
	return cloned
}
