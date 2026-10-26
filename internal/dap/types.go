package dap

// Capabilities is the adapter capability object returned by initialize.
type Capabilities struct {
	SupportsConfigurationDoneRequest  bool                        `json:"supportsConfigurationDoneRequest,omitempty"`
	SupportsFunctionBreakpoints       bool                        `json:"supportsFunctionBreakpoints,omitempty"`
	SupportsConditionalBreakpoints    bool                        `json:"supportsConditionalBreakpoints,omitempty"`
	SupportsHitConditionalBreakpoints bool                        `json:"supportsHitConditionalBreakpoints,omitempty"`
	SupportsEvaluateForHovers         bool                        `json:"supportsEvaluateForHovers,omitempty"`
	SupportsStepBack                  bool                        `json:"supportsStepBack,omitempty"`
	SupportsSetVariable               bool                        `json:"supportsSetVariable,omitempty"`
	SupportsRestartFrame              bool                        `json:"supportsRestartFrame,omitempty"`
	SupportsStepInTargetsRequest      bool                        `json:"supportsStepInTargetsRequest,omitempty"`
	SupportsExceptionInfoRequest      bool                        `json:"supportsExceptionInfoRequest,omitempty"`
	SupportsTerminateRequest          bool                        `json:"supportsTerminateRequest,omitempty"`
	SupportsRestartRequest            bool                        `json:"supportsRestartRequest,omitempty"`
	SupportsLogPoints                 bool                        `json:"supportsLogPoints,omitempty"`
	SupportsDataBreakpoints           bool                        `json:"supportsDataBreakpoints,omitempty"`
	SupportsVariablePaging            bool                        `json:"supportsVariablePaging,omitempty"`
	SupportsVariableType              bool                        `json:"supportsVariableType,omitempty"`
	ExceptionBreakpointFilters        []ExceptionBreakpointFilter `json:"exceptionBreakpointFilters,omitempty"`
}

// ExceptionBreakpointFilter describes a named adapter exception filter.
type ExceptionBreakpointFilter struct {
	Filter      string `json:"filter"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Default     bool   `json:"default,omitempty"`
}

// ExceptionInfoResult is the adapter's description of an exception stop.
type ExceptionInfoResult struct {
	ExceptionID string            `json:"exceptionId"`
	Description string            `json:"description,omitempty"`
	BreakMode   string            `json:"breakMode"`
	Details     *ExceptionDetails `json:"details,omitempty"`
}

// ExceptionDetails contains optional adapter-provided exception information.
type ExceptionDetails struct {
	Message      string `json:"message,omitempty"`
	TypeName     string `json:"typeName,omitempty"`
	FullTypeName string `json:"fullTypeName,omitempty"`
	EvaluateName string `json:"evaluateName,omitempty"`
	StackTrace   string `json:"stackTrace,omitempty"`
}

// Source identifies a source file in a debuggee.
type Source struct {
	Name             string `json:"name,omitempty"`
	Path             string `json:"path,omitempty"`
	SourceReference  int    `json:"sourceReference,omitempty"`
	PresentationHint string `json:"presentationHint,omitempty"`
}

// SourceBreakpoint is a requested source-line breakpoint.
type SourceBreakpoint struct {
	Line         int    `json:"line"`
	Column       int    `json:"column,omitempty"`
	Condition    string `json:"condition,omitempty"`
	HitCondition string `json:"hitCondition,omitempty"`
	LogMessage   string `json:"logMessage,omitempty"`
}

// FunctionBreakpoint is a requested function breakpoint.
type FunctionBreakpoint struct {
	Name         string `json:"name"`
	Condition    string `json:"condition,omitempty"`
	HitCondition string `json:"hitCondition,omitempty"`
}

// Breakpoint is an adapter-verified breakpoint.
type Breakpoint struct {
	ID                   int     `json:"id,omitempty"`
	Verified             bool    `json:"verified"`
	Message              string  `json:"message,omitempty"`
	Source               *Source `json:"source,omitempty"`
	Line                 int     `json:"line,omitempty"`
	Column               int     `json:"column,omitempty"`
	EndLine              int     `json:"endLine,omitempty"`
	EndColumn            int     `json:"endColumn,omitempty"`
	InstructionReference string  `json:"instructionReference,omitempty"`
}

// ContinueResult describes the threads that remain stopped after continue.
type ContinueResult struct {
	AllThreadsContinued bool `json:"allThreadsContinued,omitempty"`
}

// Thread is a running or stopped debuggee thread.
type Thread struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// StackTraceResult is a page of frames for one thread.
type StackTraceResult struct {
	StackFrames []StackFrame `json:"stackFrames"`
	TotalFrames int          `json:"totalFrames,omitempty"`
}

// StackFrame is one frame in a debuggee thread.
type StackFrame struct {
	ID             int     `json:"id"`
	Name           string  `json:"name"`
	Source         *Source `json:"source,omitempty"`
	Line           int     `json:"line"`
	Column         int     `json:"column"`
	EndLine        int     `json:"endLine,omitempty"`
	EndColumn      int     `json:"endColumn,omitempty"`
	CanRestart     bool    `json:"canRestart,omitempty"`
	InstructionRef string  `json:"instructionPointerReference,omitempty"`
}

// Scope is a group of variables visible in a stack frame.
type Scope struct {
	Name               string `json:"name"`
	PresentationHint   string `json:"presentationHint,omitempty"`
	VariablesReference int    `json:"variablesReference"`
	NamedVariables     int    `json:"namedVariables,omitempty"`
	IndexedVariables   int    `json:"indexedVariables,omitempty"`
	Expensive          bool   `json:"expensive"`
}

// Variable is one visible value in a scope or another variable.
type Variable struct {
	Name               string `json:"name"`
	Value              string `json:"value"`
	Type               string `json:"type,omitempty"`
	PresentationHint   any    `json:"presentationHint,omitempty"`
	EvaluateName       string `json:"evaluateName,omitempty"`
	VariablesReference int    `json:"variablesReference"`
	NamedVariables     int    `json:"namedVariables,omitempty"`
	IndexedVariables   int    `json:"indexedVariables,omitempty"`
}

// SetVariableResult describes the adapter's assigned variable value.
type SetVariableResult struct {
	Value              string `json:"value"`
	Type               string `json:"type,omitempty"`
	VariablesReference int    `json:"variablesReference"`
	NamedVariables     int    `json:"namedVariables,omitempty"`
	IndexedVariables   int    `json:"indexedVariables,omitempty"`
}

// EvaluateResult is the result of evaluating a debug expression.
type EvaluateResult struct {
	Result             string `json:"result"`
	Type               string `json:"type,omitempty"`
	PresentationHint   any    `json:"presentationHint,omitempty"`
	VariablesReference int    `json:"variablesReference"`
	NamedVariables     int    `json:"namedVariables,omitempty"`
	IndexedVariables   int    `json:"indexedVariables,omitempty"`
	MemoryReference    string `json:"memoryReference,omitempty"`
}

// SetBreakpointsArguments is the DAP request body for setBreakpoints.
type SetBreakpointsArguments struct {
	Source         Source             `json:"source"`
	Breakpoints    []SourceBreakpoint `json:"breakpoints,omitempty"`
	Lines          []int              `json:"lines,omitempty"`
	SourceModified bool               `json:"sourceModified,omitempty"`
}
