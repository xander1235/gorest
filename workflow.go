package gorest

import (
	"context"
	"fmt"
	"reflect"

	"github.com/xander1235/gorest/constants/enums"
	"github.com/xander1235/gorest/exceptions/errors"
)

// WorkflowStep represents a single step in a request workflow
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
	
	// Condition is an optional function to determine if this step should execute
	// If returns false, the step is skipped
	Condition func(ctx *WorkflowContext) bool
}

// WorkflowContext provides access to all previous step results during workflow execution
type WorkflowContext struct {
	// StepResults maps step names to their execution results
	StepResults map[string]*WorkflowStepResult
	
	// CurrentStep is the index of the step currently being executed
	CurrentStep int
	
	// Metadata allows passing custom data between workflow steps
	Metadata map[string]interface{}
	
	// InitialBody is the original request body provided to the workflow
	InitialBody interface{}
}

// WorkflowStepResult holds the result of executing a single workflow step
type WorkflowStepResult struct {
	// Step is the original step configuration
	Step *WorkflowStep
	
	// Error contains any error that occurred during this step
	Error *errors.ErrorDetails
	
	// Success indicates whether the step completed successfully
	Success bool
	
	// ResponseData contains the parsed response data (same as Step.Response)
	ResponseData interface{}
	
	// Skipped indicates whether this step was skipped due to its condition
	Skipped bool
	
	// Index is the position of this step in the workflow
	Index int
}

// WorkflowResponse contains the complete workflow execution results
type WorkflowResponse struct {
	// StepResults contains results for all workflow steps in execution order
	StepResults []*WorkflowStepResult
	
	// Success indicates whether the entire workflow completed successfully
	Success bool
	
	// CompletedSteps is the number of steps that were successfully executed
	CompletedSteps int
	
	// SkippedSteps is the number of steps that were skipped
	SkippedSteps int
	
	// FailedStep contains the first step that failed (if any)
	FailedStep *WorkflowStepResult
	
	// Context is the final workflow context with all step results
	Context *WorkflowContext
}

// ExecuteWorkflow executes a sequence of dependent HTTP requests where each step
// can use data from previous steps to build its request.
//
// This method is perfect for API workflows like:
// - Authentication followed by resource operations
// - Creating parent resource, then child resources
// - Multi-step data submission workflows
// - Complex API orchestration scenarios
//
// The workflow stops on the first failed step (unless StopOnError is false)
// and provides detailed results for each step.
//
// Parameters:
//   - steps: Slice of workflow steps to execute in order
//   - initialBody: Optional initial data available to all steps
//   - metadata: Optional metadata to pass between steps
//
// Returns:
//   - *WorkflowResponse: Complete workflow execution results
//
// Example:
//
//	// Multi-step user onboarding workflow
//	steps := []*WorkflowStep{
//	    {
//	        Name: "authenticate",
//	        Endpoint: "/auth/login",
//	        Method: enums.POST,
//	        BodyBuilder: func(ctx *WorkflowContext) interface{} {
//	            return map[string]string{
//	                "username": "admin",
//	                "password": "secret",
//	            }
//	        },
//	        Response: &AuthResponse{},
//	        StopOnError: true,
//	    },
//	    {
//	        Name: "create_user",
//	        Endpoint: "/users",
//	        Method: enums.POST,
//	        BodyBuilder: func(ctx *WorkflowContext) interface{} {
//	            // Use authentication token from previous step
//	            authResult := ctx.StepResults["authenticate"]
//	            if authResult.Success {
//	                auth := authResult.ResponseData.(*AuthResponse)
//	                // Use initial body data
//	                userData := ctx.InitialBody.(map[string]interface{})
//	                userData["created_by"] = auth.UserID
//	                return userData
//	            }
//	            return ctx.InitialBody
//	        },
//	        HeadersBuilder: func(ctx *WorkflowContext) map[string]string {
//	            authResult := ctx.StepResults["authenticate"]
//	            if authResult.Success {
//	                auth := authResult.ResponseData.(*AuthResponse)
//	                return map[string]string{
//	                    "Authorization": "Bearer " + auth.Token,
//	                }
//	            }
//	            return nil
//	        },
//	        Response: &UserResponse{},
//	        StopOnError: true,
//	    },
//	    {
//	        Name: "send_welcome_email",
//	        Endpoint: "/notifications/email",
//	        Method: enums.POST,
//	        BodyBuilder: func(ctx *WorkflowContext) interface{} {
//	            userResult := ctx.StepResults["create_user"]
//	            if userResult.Success {
//	                user := userResult.ResponseData.(*UserResponse)
//	                return map[string]interface{}{
//	                    "to": user.Email,
//	                    "template": "welcome",
//	                    "data": map[string]interface{}{
//	                        "name": user.Name,
//	                    },
//	                }
//	            }
//	            return nil
//	        },
//	        StopOnError: false, // Don't fail workflow if email fails
//	    },
//	}
//
//	userData := map[string]interface{}{
//	    "name": "John Doe",
//	    "email": "john@example.com",
//	}
//
//	response := client.Host("https://api.example.com").ExecuteWorkflow(steps, userData, nil)
func (nc *NetworkClient) ExecuteWorkflow(steps []*WorkflowStep, initialBody interface{}, metadata map[string]interface{}) *WorkflowResponse {
	copyClient := nc.ensureRequestCopy()
	
	// Initialize workflow context
	ctx := &WorkflowContext{
		StepResults: make(map[string]*WorkflowStepResult),
		CurrentStep: 0,
		Metadata:    metadata,
		InitialBody: initialBody,
	}
	if ctx.Metadata == nil {
		ctx.Metadata = make(map[string]interface{})
	}
	
	results := make([]*WorkflowStepResult, len(steps))
	completedSteps := 0
	skippedSteps := 0
	var failedStep *WorkflowStepResult
	
	// Execute steps sequentially
	for i, step := range steps {
		ctx.CurrentStep = i
		
		// Check if step should be executed
		shouldExecute := true
		if step.Condition != nil {
			shouldExecute = step.Condition(ctx)
		}
		
		if !shouldExecute {
			// Skip this step
			results[i] = &WorkflowStepResult{
				Step:         step,
				Error:        nil,
				Success:      true,
				ResponseData: nil,
				Skipped:      true,
				Index:        i,
			}
			ctx.StepResults[step.Name] = results[i]
			skippedSteps++
			continue
		}
		
		// Execute the step
		stepResult := copyClient.executeWorkflowStep(step, ctx, i)
		results[i] = stepResult
		ctx.StepResults[step.Name] = stepResult
		
		if stepResult.Success {
			completedSteps++
		} else {
			// Step failed
			if failedStep == nil {
				failedStep = stepResult
			}
			
			// Check if we should stop on error
			if step.StopOnError {
				// Mark remaining steps as skipped
				for j := i + 1; j < len(steps); j++ {
					results[j] = &WorkflowStepResult{
						Step:         steps[j],
						Error:        nil,
						Success:      false,
						ResponseData: nil,
						Skipped:      true,
						Index:        j,
					}
					skippedSteps++
				}
				break
			}
		}
	}
	
	return &WorkflowResponse{
		StepResults:    results,
		Success:        failedStep == nil,
		CompletedSteps: completedSteps,
		SkippedSteps:   skippedSteps,
		FailedStep:     failedStep,
		Context:        ctx,
	}
}

// executeWorkflowStep executes a single step in the workflow
func (nc *NetworkClient) executeWorkflowStep(step *WorkflowStep, ctx *WorkflowContext, index int) *WorkflowStepResult {
	// Create a copy for this step
	stepClient := nc.copyForRequest()
	
	// Set host for this step - use step-specific host or fallback to client host
	if step.Host != "" {
		stepClient.host = step.Host
	} else if stepClient.host == "" {
		// Fallback to original client's host if neither step nor client has one
		stepClient.host = nc.host
	}
	
	// Return error immediately if no host is available
	if stepClient.host == "" {
		return &WorkflowStepResult{
			Step:         step,
			Error:        &errors.ErrorDetails{
				Message:      "No host specified for step '" + step.Name + "' endpoint " + step.Endpoint + ". Provide host via step.Host or client.Host()",
				ResponseCode: 0,
			},
			Success:      false,
			ResponseData: nil,
			Skipped:      false,
			Index:        index,
		}
	}
	
	// Build request body using previous responses
	if step.BodyBuilder != nil {
		stepClient.body = step.BodyBuilder(ctx)
	}
	
	// Build headers using previous responses
	if step.HeadersBuilder != nil {
		headers := step.HeadersBuilder(ctx)
		if headers != nil {
			if stepClient.headers == nil {
				stepClient.headers = make(map[string]string)
			}
			for k, v := range headers {
				stepClient.headers[k] = v
			}
		}
	}
	
	// Build params using previous responses
	if step.ParamsBuilder != nil {
		params := step.ParamsBuilder(ctx)
		if params != nil {
			if stepClient.params == nil {
				stepClient.params = make(map[string]string)
			}
			for k, v := range params {
				stepClient.params[k] = v
			}
		}
	}
	
	// Set response target
	if step.Response != nil {
		stepClient.response = step.Response
	}
	
	// Set context
	if step.Context != nil {
		stepClient.ctx = step.Context
	}
	
	// Execute the request
	err := stepClient.executeRequest(step.Method, step.Endpoint)
	
	return &WorkflowStepResult{
		Step:         step,
		Error:        err,
		Success:      err == nil,
		ResponseData: step.Response,
		Skipped:      false,
		Index:        index,
	}
}

// GetStepResult retrieves the result of a specific step by name
func (wc *WorkflowContext) GetStepResult(stepName string) *WorkflowStepResult {
	return wc.StepResults[stepName]
}

// GetStepResponse retrieves the response data of a specific step by name
func (wc *WorkflowContext) GetStepResponse(stepName string) interface{} {
	if result := wc.StepResults[stepName]; result != nil && result.Success {
		return result.ResponseData
	}
	return nil
}

// GetTypedStepResponse retrieves and casts the response data of a specific step
func (wc *WorkflowContext) GetTypedStepResponse(stepName string, targetType interface{}) interface{} {
	response := wc.GetStepResponse(stepName)
	if response == nil {
		return nil
	}
	
	// Check if response is of the expected type
	responseType := reflect.TypeOf(response)
	targetTypeRef := reflect.TypeOf(targetType)
	
	if responseType == targetTypeRef {
		return response
	}
	
	return nil
}

// SetMetadata sets a metadata value for the workflow context
func (wc *WorkflowContext) SetMetadata(key string, value interface{}) {
	wc.Metadata[key] = value
}

// GetMetadata retrieves a metadata value from the workflow context
func (wc *WorkflowContext) GetMetadata(key string) interface{} {
	return wc.Metadata[key]
}

// HasSuccessfulStep checks if a specific step completed successfully
func (wc *WorkflowContext) HasSuccessfulStep(stepName string) bool {
	if result := wc.StepResults[stepName]; result != nil {
		return result.Success && !result.Skipped
	}
	return false
}

// GetSuccessfulSteps returns all workflow steps that completed successfully
func (wr *WorkflowResponse) GetSuccessfulSteps() []*WorkflowStepResult {
	var successful []*WorkflowStepResult
	for _, result := range wr.StepResults {
		if result.Success && !result.Skipped {
			successful = append(successful, result)
		}
	}
	return successful
}

// GetFailedSteps returns all workflow steps that failed
func (wr *WorkflowResponse) GetFailedSteps() []*WorkflowStepResult {
	var failed []*WorkflowStepResult
	for _, result := range wr.StepResults {
		if !result.Success && !result.Skipped {
			failed = append(failed, result)
		}
	}
	return failed
}

// GetSkippedSteps returns all workflow steps that were skipped
func (wr *WorkflowResponse) GetSkippedSteps() []*WorkflowStepResult {
	var skipped []*WorkflowStepResult
	for _, result := range wr.StepResults {
		if result.Skipped {
			skipped = append(skipped, result)
		}
	}
	return skipped
}

// GetStepByName finds a step result by its name
func (wr *WorkflowResponse) GetStepByName(name string) *WorkflowStepResult {
	for _, result := range wr.StepResults {
		if result.Step.Name == name {
			return result
		}
	}
	return nil
}

// GetErrorSummary returns a summary of all step errors
func (wr *WorkflowResponse) GetErrorSummary() map[string]*errors.ErrorDetails {
	errorMap := make(map[string]*errors.ErrorDetails)
	for _, result := range wr.StepResults {
		if result.Error != nil {
			errorMap[result.Step.Name] = result.Error
		}
	}
	return errorMap
}

// String provides a human-readable summary of the workflow execution
func (wr *WorkflowResponse) String() string {
	totalSteps := len(wr.StepResults)
	return fmt.Sprintf("Workflow: %d total steps, %d completed, %d skipped, %d failed",
		totalSteps, wr.CompletedSteps, wr.SkippedSteps, totalSteps-wr.CompletedSteps-wr.SkippedSteps)
}
