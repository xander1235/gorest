package gorest

import (
	"context"
	"github.com/xander1235/gorest/v2/constants/enums"
	"github.com/xander1235/gorest/v2/exceptions/errors"
)

// WorkflowStep represents a single step in a workflow
type WorkflowStep struct {
	// Name is a unique identifier for this step (for logging and debugging)
	Name string

	// Endpoint is the URL path for this step
	Endpoint string

	// Method is the HTTP method to use for this step
	Method enums.HttpMethods

	// Host is the optional base URL for this specific step (e.g., "https://api.service2.com")
	// If empty, the client's host will be used as fallback
	Host string

	// BodyBuilder creates the request body for this step using previous responses
	// The function receives a WorkflowContext containing all previous step responses
	BodyBuilder func(ctx *WorkflowContext) interface{}

	// HeadersBuilder creates additional headers for this step using previous responses
	HeadersBuilder func(ctx *WorkflowContext) map[string]string

	// ParamsBuilder creates query parameters for this step using previous responses
	ParamsBuilder func(ctx *WorkflowContext) map[string]string

	// Response is a pointer to struct where this step's response should be stored
	Response interface{}

	// Context allows per-step request context (timeout, cancellation)
	Context context.Context

	// StopOnError determines if workflow should halt if this step fails
	// If false, workflow continues even if this step fails
	StopOnError bool

	// Condition is a function that determines whether this step should execute
	// If nil, step always executes; if returns false, step is skipped
	Condition func(ctx *WorkflowContext) bool
}

// WorkflowContext stores all data shared between workflow steps
type WorkflowContext struct {
	// StepResults contains results from all completed steps by step name
	StepResults map[string]*WorkflowStepResult

	// CurrentStep is the index of the current step being executed
	CurrentStep int

	// Metadata stores any workflow-level metadata (e.g., tracking info)
	Metadata map[string]interface{}

	// InitialBody is the optional data passed to the workflow execution
	InitialBody interface{}
}

// WorkflowStepResult stores the result of a single workflow step
type WorkflowStepResult struct {
	// Step is a reference to the original step configuration
	Step *WorkflowStep

	// Error contains details if the step failed
	Error *errors.ErrorDetails

	// Success indicates if the step completed successfully
	Success bool

	// ResponseData contains the parsed response (matches Step.Response)
	ResponseData interface{}

	// Skipped indicates if the step was skipped due to a condition
	Skipped bool

	// SkippedReason provides context if the step was skipped
	SkippedReason string

	// Index is the position of this step in the workflow
	Index int
}

// Helper method to get a specific step result by name
func (wc *WorkflowContext) GetStepResult(stepName string) *WorkflowStepResult {
	return wc.StepResults[stepName]
}

// Helper method to get a typed step response by name
func (wc *WorkflowContext) GetStepResponse(stepName string) interface{} {
	result := wc.GetStepResult(stepName)
	if result != nil && result.Success {
		return result.ResponseData
	}
	return nil
}

// GetTypedStepResponse returns a response from a specific step with type assertion
func (wc *WorkflowContext) GetTypedStepResponse(stepName string, targetType interface{}) interface{} {
	// Get the basic response
	response := wc.GetStepResponse(stepName)
	if response == nil {
		return nil
	}

	// The target type should be a pointer to the type we want to cast to
	// Return the response cast to the target type
	return response
}

// Helper method to check if a step was successful
func (wc *WorkflowContext) HasSuccessfulStep(stepName string) bool {
	result := wc.GetStepResult(stepName)
	return result != nil && result.Success && !result.Skipped
}

// Helper method to get metadata
func (wc *WorkflowContext) GetMetadata(key string) interface{} {
	if wc.Metadata == nil {
		return nil
	}
	return wc.Metadata[key]
}

// Helper method to set metadata
func (wc *WorkflowContext) SetMetadata(key string, value interface{}) {
	if wc.Metadata == nil {
		wc.Metadata = make(map[string]interface{})
	}
	wc.Metadata[key] = value
}

// Backward compatibility wrapper for the old workflow system
func (nc *NetworkClient) ExecuteWorkflow(steps []*WorkflowStep, initialBody interface{}, metadata map[string]interface{}) *WorkflowResponse {
	// Convert to DAG workflow
	dagSteps := make([]*DAGWorkflowStep, len(steps))
	for i, step := range steps {
		dependencies := []string{}
		if i > 0 {
			// Add previous step as dependency for sequential execution
			dependencies = append(dependencies, steps[i-1].Name)
		}

		dagSteps[i] = &DAGWorkflowStep{
			Name:           step.Name,
			Endpoint:       step.Endpoint,
			Method:         step.Method,
			Host:           step.Host,
			Dependencies:   dependencies,
			BodyBuilder:    step.BodyBuilder,
			HeadersBuilder: step.HeadersBuilder,
			ParamsBuilder:  step.ParamsBuilder,
			Response:       step.Response,
			Context:        step.Context,
			StopOnError:    step.StopOnError,
			Condition:      step.Condition,
		}
	}

	// Execute as DAG workflow
	dagResponse := nc.ExecuteDAGWorkflow(dagSteps, initialBody, metadata)

	// Convert DAG response to old workflow response format
	response := &WorkflowResponse{
		StepResults:    dagResponse.StepResults,
		Success:        dagResponse.Success,
		CompletedSteps: dagResponse.CompletedSteps,
		SkippedSteps:   dagResponse.SkippedSteps,
		Context:        dagResponse.Context,
	}

	// Set failed step if any
	if !dagResponse.Success && len(dagResponse.FailedSteps) > 0 {
		response.FailedStep = dagResponse.FailedSteps[0]
	}

	return response
}

// WorkflowResponse contains the results of a workflow execution
type WorkflowResponse struct {
	// StepResults contains results from all steps by step name
	StepResults map[string]*WorkflowStepResult

	// Success indicates if the entire workflow completed successfully
	Success bool

	// CompletedSteps is the number of steps that successfully completed
	CompletedSteps int

	// SkippedSteps is the number of steps that were skipped
	SkippedSteps int

	// FailedStep contains information about the first step that failed (if any)
	FailedStep *WorkflowStepResult

	// Context is the workflow context with all step results and metadata
	Context *WorkflowContext
}

// GetSuccessfulSteps returns a list of successful step results
func (wr *WorkflowResponse) GetSuccessfulSteps() []*WorkflowStepResult {
	var results []*WorkflowStepResult
	for _, result := range wr.StepResults {
		if result.Success && !result.Skipped {
			results = append(results, result)
		}
	}
	return results
}

// GetFailedSteps returns a list of failed step results
func (wr *WorkflowResponse) GetFailedSteps() []*WorkflowStepResult {
	var results []*WorkflowStepResult
	for _, result := range wr.StepResults {
		if !result.Success && !result.Skipped {
			results = append(results, result)
		}
	}
	return results
}

// GetSkippedSteps returns a list of skipped step results
func (wr *WorkflowResponse) GetSkippedSteps() []*WorkflowStepResult {
	var results []*WorkflowStepResult
	for _, result := range wr.StepResults {
		if result.Skipped {
			results = append(results, result)
		}
	}
	return results
}

// GetStepByName returns a step result by name, or nil if not found
func (wr *WorkflowResponse) GetStepByName(name string) *WorkflowStepResult {
	return wr.StepResults[name]
}

// GetErrorSummary returns a map of step names to error details
func (wr *WorkflowResponse) GetErrorSummary() map[string]*errors.ErrorDetails {
	summary := make(map[string]*errors.ErrorDetails)
	for name, result := range wr.StepResults {
		if !result.Success && !result.Skipped && result.Error != nil {
			summary[name] = result.Error
		}
	}
	return summary
}

// String returns a simple string representation of the workflow response
func (wr *WorkflowResponse) String() string {
	if wr.Success {
		return "Workflow completed successfully"
	}
	return "Workflow failed"
}
