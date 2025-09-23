package gorest

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/xander1235/gorest/constants/enums"
	"github.com/xander1235/gorest/exceptions/errors"
	"strings"
)

// DAGWorkflowStep represents a single step in a DAG-based workflow with dependencies
type DAGWorkflowStep struct {
	// Name is a unique identifier for this step (for logging and debugging)
	Name string

	// Dependencies is a list of step names that must complete successfully before this step can execute
	Dependencies []string

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

	// MaxRetries specifies the maximum number of retry attempts for this step
	MaxRetries int

	// RetryDelay specifies the delay between retry attempts
	RetryDelay time.Duration
}

// DAGWorkflowExecutor manages the execution of DAG-based workflows
type DAGWorkflowExecutor struct {
	steps           []*DAGWorkflowStep
	stepsByName     map[string]*DAGWorkflowStep
	dependencyGraph map[string][]string // step name -> list of dependent step names
	reverseDeps     map[string][]string // step name -> list of dependency step names
	context         *WorkflowContext
	client          *NetworkClient
	results         map[string]*WorkflowStepResult
	resultsMutex    sync.RWMutex

	// Channels for coordination
	readySteps     chan string
	completedSteps chan string
	errorOccurred  chan *WorkflowStepResult

	// Execution state
	inProgress     map[string]bool
	progressMutex  sync.RWMutex
	completed      map[string]bool
	completedMutex sync.RWMutex

	// Control flags
	stopOnFirstError bool
	maxConcurrency   int
}

// DAGWorkflowResponse contains the complete DAG workflow execution results
type DAGWorkflowResponse struct {
	// StepResults contains results for all workflow steps indexed by step name
	StepResults map[string]*WorkflowStepResult

	// Success indicates whether the entire workflow completed successfully
	Success bool

	// CompletedSteps is the number of steps that were successfully executed
	CompletedSteps int

	// SkippedSteps is the number of steps that were skipped
	SkippedSteps int

	// FailedSteps contains all steps that failed
	FailedSteps []*WorkflowStepResult

	// Context is the final workflow context with all step results
	Context *WorkflowContext

	// ExecutionTime is the total time taken to execute the workflow
	ExecutionTime time.Duration

	// ParallelismStats contains statistics about parallel execution
	ParallelismStats *ParallelismStats
}

// ParallelismStats provides insights into the parallel execution
type ParallelismStats struct {
	// MaxConcurrentSteps is the maximum number of steps executed simultaneously
	MaxConcurrentSteps int

	// TotalExecutionLayers is the number of execution layers (levels of parallelism)
	TotalExecutionLayers int

	// AverageStepsPerLayer is the average number of steps executed per layer
	AverageStepsPerLayer float64

	// StepExecutionOrder maps step names to their execution order
	StepExecutionOrder map[string]int
}

// NewDAGWorkflowExecutor creates a new DAG workflow executor
func NewDAGWorkflowExecutor(client *NetworkClient, steps []*DAGWorkflowStep) *DAGWorkflowExecutor {
	// Calculate increased buffer size to reduce chances of channel blocking
	bufferSize := len(steps) * 2
	if bufferSize < 10 {
		bufferSize = 10 // Minimum buffer size
	}

	executor := &DAGWorkflowExecutor{
		steps:            steps,
		stepsByName:      make(map[string]*DAGWorkflowStep),
		dependencyGraph:  make(map[string][]string),
		reverseDeps:      make(map[string][]string),
		client:           client,
		results:          make(map[string]*WorkflowStepResult),
		readySteps:       make(chan string, bufferSize),              // Increased buffer size
		completedSteps:   make(chan string, bufferSize),              // Increased buffer size
		errorOccurred:    make(chan *WorkflowStepResult, bufferSize), // Increased buffer size
		inProgress:       make(map[string]bool),
		completed:        make(map[string]bool),
		stopOnFirstError: true,
		maxConcurrency:   10, // Default max concurrent goroutines
	}

	// Build step maps and dependency graph
	for _, step := range steps {
		executor.stepsByName[step.Name] = step
		executor.dependencyGraph[step.Name] = []string{}
		executor.reverseDeps[step.Name] = step.Dependencies

		// Build reverse dependency mapping
		for _, dep := range step.Dependencies {
			executor.dependencyGraph[dep] = append(executor.dependencyGraph[dep], step.Name)
		}
	}

	return executor
}

// ValidateDAG validates that the workflow forms a valid DAG (no cycles)
func (executor *DAGWorkflowExecutor) ValidateDAG() error {
	//fmt.Println("Running DAG validation...")
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	// DFS to check for cycles - use the reverseDeps which represents the actual dependencies
	var dfs func(stepName string) bool
	dfs = func(stepName string) bool {
		visited[stepName] = true
		recStack[stepName] = true

		// Check dependencies (not dependents)
		for _, depStep := range executor.reverseDeps[stepName] {
			if !visited[depStep] {
				if dfs(depStep) {
					return true // Cycle detected
				}
			} else if recStack[depStep] {
				return true // Cycle detected
			}
		}

		recStack[stepName] = false
		return false
	}

	// Check each step for cycles
	for stepName := range executor.stepsByName {
		if !visited[stepName] {
			if dfs(stepName) {
				//fmt.Printf("Cycle detected in workflow graph involving step: %s\n", stepName)
				return fmt.Errorf("cycle detected in workflow graph involving step: %s", stepName)
			}
		}
	}

	// Validate that all dependencies exist
	for stepName, step := range executor.stepsByName {
		for _, dep := range step.Dependencies {
			if _, exists := executor.stepsByName[dep]; !exists {
				//fmt.Printf("Step '%s' depends on non-existent step '%s'\n", stepName, dep)
				return fmt.Errorf("step '%s' depends on non-existent step '%s'", stepName, dep)
			}
		}
	}

	//fmt.Println("DAG validation passed successfully")
	return nil
}

// SetMaxConcurrency sets the maximum number of concurrent step executions
func (executor *DAGWorkflowExecutor) SetMaxConcurrency(max int) {
	if max > 0 {
		executor.maxConcurrency = max
	}
}

// SetStopOnFirstError configures whether to stop execution on the first error
func (executor *DAGWorkflowExecutor) SetStopOnFirstError(stop bool) {
	executor.stopOnFirstError = stop
}

// Execute runs the DAG workflow with parallel execution
// Execute runs the DAG workflow with parallel execution
func (executor *DAGWorkflowExecutor) Execute(initialBody interface{}, metadata map[string]interface{}) *DAGWorkflowResponse {
	startTime := time.Now()
	//fmt.Println("Starting DAG workflow execution")

	// Initialize workflow context
	executor.context = &WorkflowContext{
		StepResults: make(map[string]*WorkflowStepResult),
		CurrentStep: 0,
		Metadata:    metadata,
		InitialBody: initialBody,
	}
	if executor.context.Metadata == nil {
		executor.context.Metadata = make(map[string]interface{})
	}

	// Validate DAG structure
	if err := executor.ValidateDAG(); err != nil {
		return &DAGWorkflowResponse{
			StepResults: map[string]*WorkflowStepResult{
				"validation_error": {
					Step: &WorkflowStep{Name: "validation_error"},
					Error: &errors.ErrorDetails{
						Message: fmt.Sprintf("DAG validation failed: %s", err.Error()),
					},
					Success: false,
				},
			},
			Success:       false,
			ExecutionTime: time.Since(startTime),
		}
	}

	//fmt.Println("DAG validation successful, starting execution orchestrator")
	// Start the execution orchestrator
	go executor.orchestrateExecution()

	// Start worker pool for step execution
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, executor.maxConcurrency)

	// Find initial ready steps (no dependencies)
	//fmt.Println("Finding initial ready steps...")
	executor.findReadySteps()

	// Use a mutex to protect access to the execution order and stats map
	var statsLock sync.Mutex
	executionOrder := 0
	stats := &ParallelismStats{
		StepExecutionOrder: make(map[string]int),
	}

	// Main execution loop
	//fmt.Println("Starting main execution loop")
	executionComplete := false

	// Add a global timeout to prevent infinite execution
	globalTimeout := time.After(2 * time.Minute)

	for !executionComplete {
		select {
		// Add global timeout case
		case <-globalTimeout:
			//fmt.Println("WARNING: Global timeout reached, forcing workflow termination")
			executionComplete = true

		case stepName := <-executor.readySteps:
			//fmt.Printf("Executing step: %s\n", stepName)
			wg.Add(1)
			go func(name string) {
				defer wg.Done()
				semaphore <- struct{}{}        // Acquire semaphore
				defer func() { <-semaphore }() // Release semaphore

				// Protect concurrent access to the stats map
				statsLock.Lock()
				stats.StepExecutionOrder[name] = executionOrder
				executionOrder++
				statsLock.Unlock()

				executor.executeStep(name)
			}(stepName)

		case stepName := <-executor.completedSteps:
			//fmt.Printf("Step completed: %s\n", stepName)
			executor.markStepCompleted(stepName)
			executor.findReadySteps()

		case failedStep := <-executor.errorOccurred:
			//fmt.Printf("Step failed: %s\n", failedStep.Step.Name)
			if executor.stopOnFirstError && failedStep.Step.StopOnError {
				executionComplete = true
			}

		default:
			// Check if all steps are completed or if we should stop
			if executor.allStepsCompleted() || executor.shouldStopExecution() {
				executionComplete = true
				//fmt.Println("All steps completed or execution should stop")
			}
			time.Sleep(10 * time.Millisecond) // Small delay to prevent busy waiting
		}
	}

	// Wait for all running goroutines to complete
	//fmt.Println("Waiting for all goroutines to complete...")

	// Set a timeout for waiting on goroutines
	wgDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(wgDone)
	}()

	select {
	case <-wgDone:
		//fmt.Println("All goroutines completed normally")
	case <-time.After(5 * time.Second):
		// Timeout waiting for goroutines
		fmt.Println("WARNING: Timeout waiting for goroutines to complete")
	}

	executionTime := time.Since(startTime)

	// Build response
	response := &DAGWorkflowResponse{
		StepResults:      executor.results,
		Success:          executor.isWorkflowSuccessful(),
		CompletedSteps:   executor.countCompletedSteps(),
		SkippedSteps:     executor.countSkippedSteps(),
		FailedSteps:      executor.getFailedSteps(),
		Context:          executor.context,
		ExecutionTime:    executionTime,
		ParallelismStats: stats,
	}

	// Calculate parallelism statistics
	executor.calculateParallelismStats(response)

	//fmt.Printf("DAG workflow completed in %v\n", executionTime)
	return response
}

// orchestrateExecution manages the workflow execution coordination
// orchestrateExecution manages the workflow execution coordination
func (executor *DAGWorkflowExecutor) orchestrateExecution() {
	// This goroutine helps ensure channel operations don't block
	// by providing additional consumers for the channels

	// Add a timeout to ensure we don't get stuck in a deadlock
	timeout := time.After(60 * time.Second) // More reasonable timeout (1 minute)

	for {
		select {
		// Monitor channels and handle any messages that might be stuck
		case stepName := <-executor.completedSteps:
			executor.markStepCompleted(stepName)
			executor.findReadySteps()

		case failedStep := <-executor.errorOccurred:
			executor.resultsMutex.Lock()
			// Just make sure the result is stored properly
			if executor.results[failedStep.Step.Name] == nil {
				executor.results[failedStep.Step.Name] = failedStep
			}
			// Make sure failed step is marked as completed to prevent deadlock
			executor.resultsMutex.Unlock()

			// Always mark the step as completed even if it failed
			executor.completedMutex.Lock()
			executor.completed[failedStep.Step.Name] = true
			executor.completedMutex.Unlock()

			// Continue execution unless stopOnFirstError
			if !executor.stopOnFirstError || !failedStep.Step.StopOnError {
				executor.findReadySteps()
			}

		case <-timeout:
			fmt.Println("Warning: orchestrateExecution timeout reached, exiting to prevent deadlock")
			return // Exit if timeout is reached

		default:
			// Check if all steps are done
			if executor.allStepsCompleted() || executor.shouldStopExecution() {
				return // Exit goroutine when workflow is complete
			}
			time.Sleep(5 * time.Millisecond) // Avoid tight loop
		}
	}
}

// findReadySteps identifies steps that are ready to execute (all dependencies satisfied)
// findReadySteps identifies steps that are ready to execute (all dependencies satisfied)
func (executor *DAGWorkflowExecutor) findReadySteps() {
	for stepName, step := range executor.stepsByName {
		if executor.isStepReady(stepName) {
			// Check if step condition allows execution
			if step.Condition != nil && !step.Condition(executor.context) {
				// Mark as skipped
				executor.resultsMutex.Lock()
				executor.results[stepName] = &WorkflowStepResult{
					Step:         &WorkflowStep{Name: stepName},
					Success:      true,
					Skipped:      true,
					ResponseData: nil,
				}
				executor.resultsMutex.Unlock()

				// Use non-blocking send for completed steps
				select {
				case executor.completedSteps <- stepName:
					// Successfully sent
				default:
					// Just log the warning, don't spawn new goroutines
					//fmt.Printf("Warning: completedSteps channel full for skipped step %s\n", stepName)
				}
				continue
			}

			executor.progressMutex.Lock()
			if !executor.inProgress[stepName] {
				executor.inProgress[stepName] = true
				executor.progressMutex.Unlock()

				select {
				case executor.readySteps <- stepName:
					// Successfully sent
				default:
					// Channel is full, step will be picked up later
					executor.progressMutex.Lock()
					executor.inProgress[stepName] = false
					executor.progressMutex.Unlock()
				}
			} else {
				executor.progressMutex.Unlock()
			}
		}
	}
}

// isStepReady checks if all dependencies for a step are satisfied
func (executor *DAGWorkflowExecutor) isStepReady(stepName string) bool {
	executor.completedMutex.RLock()
	defer executor.completedMutex.RUnlock()

	executor.progressMutex.RLock()
	defer executor.progressMutex.RUnlock()

	// Don't execute if already completed or in progress
	if executor.completed[stepName] || executor.inProgress[stepName] {
		return false
	}

	// Check if all dependencies are satisfied
	step := executor.stepsByName[stepName]
	for _, dep := range step.Dependencies {
		if !executor.completed[dep] {
			return false
		}

		// Also check if the dependency was successful (unless StopOnError is false)
		executor.resultsMutex.RLock()
		result := executor.results[dep]
		executor.resultsMutex.RUnlock()

		if result == nil || (!result.Success && !result.Skipped) {
			// Dependency failed, this step cannot execute
			return false
		}
	}

	return true
}

// executeStep executes a single workflow step with retry logic
func (executor *DAGWorkflowExecutor) executeStep(stepName string) {
	step := executor.stepsByName[stepName]
	var result *WorkflowStepResult

	maxRetries := step.MaxRetries
	if maxRetries == 0 {
		maxRetries = 1 // At least one attempt
	}

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 && step.RetryDelay > 0 {
			time.Sleep(step.RetryDelay)
		}

		// Convert DAGWorkflowStep to WorkflowStep for existing execution logic
		workflowStep := &WorkflowStep{
			Name:           step.Name,
			Endpoint:       step.Endpoint,
			Method:         step.Method,
			Host:           step.Host,
			BodyBuilder:    step.BodyBuilder,
			HeadersBuilder: step.HeadersBuilder,
			ParamsBuilder:  step.ParamsBuilder,
			Response:       step.Response,
			Context:        step.Context,
			StopOnError:    step.StopOnError,
			Condition:      step.Condition,
		}

		// Execute using existing workflow step execution logic
		result = executor.client.executeWorkflowStep(workflowStep, executor.context, 0)

		if result.Success || attempt == maxRetries-1 {
			break // Success or final attempt
		}
	}

	// Store result
	executor.resultsMutex.Lock()
	executor.results[stepName] = result
	executor.context.StepResults[stepName] = result
	executor.resultsMutex.Unlock()

	// Check for context cancellation error
	isContextCancelled := false
	if result != nil && result.Error != nil {
		// Try to detect if error is related to context cancellation
		errMsg := result.Error.Message
		if errMsg != "" {
			if strings.Contains(errMsg, "context") ||
				strings.Contains(errMsg, "timeout") ||
				strings.Contains(errMsg, "deadline") ||
				strings.Contains(errMsg, "cancel") {
				isContextCancelled = true
				//fmt.Printf("Step %s failed due to context cancellation\n", stepName)
			}
		}
	}

	// Always ensure the step is marked as completed in the executor's state
	// to avoid deadlocks, especially for context timeouts
	if isContextCancelled {
		// For context cancellation, directly mark as completed
		executor.markStepCompleted(stepName)

		// Also signal the error to update the workflow state
		select {
		case executor.errorOccurred <- result:
			// Successfully sent to error channel
		default:
			//fmt.Printf("Warning: errorOccurred channel full for context cancelled step %s\n", stepName)
		}
	} else if result.Success {
		// Normal success path
		select {
		case executor.completedSteps <- stepName:
			// Successfully sent to completed channel
		default:
			//fmt.Printf("Warning: completedSteps channel full for step %s\n", stepName)
			// If channel is full, mark as completed directly
			executor.markStepCompleted(stepName)
		}
	} else {
		// Normal error path
		select {
		case executor.errorOccurred <- result:
			// Successfully sent to error channel
		default:
			//fmt.Printf("Warning: errorOccurred channel full for step %s\n", stepName)
			// If channel is full, mark as completed directly
			executor.markStepCompleted(stepName)
		}
	}
}

// markStepCompleted marks a step as completed and updates internal state
func (executor *DAGWorkflowExecutor) markStepCompleted(stepName string) {
	executor.completedMutex.Lock()
	executor.completed[stepName] = true
	executor.completedMutex.Unlock()

	executor.progressMutex.Lock()
	executor.inProgress[stepName] = false
	executor.progressMutex.Unlock()
}

// allStepsCompleted checks if all workflow steps have been completed
func (executor *DAGWorkflowExecutor) allStepsCompleted() bool {
	executor.completedMutex.RLock()
	defer executor.completedMutex.RUnlock()

	return len(executor.completed) == len(executor.steps)
}

// shouldStopExecution determines if execution should be stopped due to errors
func (executor *DAGWorkflowExecutor) shouldStopExecution() bool {
	if !executor.stopOnFirstError {
		return false
	}

	executor.resultsMutex.RLock()
	defer executor.resultsMutex.RUnlock()

	for _, result := range executor.results {
		if !result.Success && !result.Skipped {
			step := executor.stepsByName[result.Step.Name]
			if step != nil && step.StopOnError {
				return true
			}
		}
	}

	return false
}

// Helper methods for building the response
func (executor *DAGWorkflowExecutor) isWorkflowSuccessful() bool {
	executor.resultsMutex.RLock()
	defer executor.resultsMutex.RUnlock()

	for _, result := range executor.results {
		if !result.Success && !result.Skipped {
			return false
		}
	}
	return true
}

func (executor *DAGWorkflowExecutor) countCompletedSteps() int {
	executor.resultsMutex.RLock()
	defer executor.resultsMutex.RUnlock()

	count := 0
	for _, result := range executor.results {
		if result.Success && !result.Skipped {
			count++
		}
	}
	return count
}

func (executor *DAGWorkflowExecutor) countSkippedSteps() int {
	executor.resultsMutex.RLock()
	defer executor.resultsMutex.RUnlock()

	count := 0
	for _, result := range executor.results {
		if result.Skipped {
			count++
		}
	}
	return count
}

func (executor *DAGWorkflowExecutor) getFailedSteps() []*WorkflowStepResult {
	executor.resultsMutex.RLock()
	defer executor.resultsMutex.RUnlock()

	var failed []*WorkflowStepResult
	for _, result := range executor.results {
		if !result.Success && !result.Skipped {
			failed = append(failed, result)
		}
	}
	return failed
}

func (executor *DAGWorkflowExecutor) calculateParallelismStats(response *DAGWorkflowResponse) {
	// Implementation for calculating detailed parallelism statistics
	// This would analyze execution patterns, concurrent steps, etc.
	stats := response.ParallelismStats
	if stats == nil {
		return
	}

	// Calculate max concurrent steps by analyzing execution times
	// This is a simplified version - could be enhanced with actual timing data
	stats.MaxConcurrentSteps = executor.maxConcurrency
	stats.TotalExecutionLayers = executor.calculateExecutionLayers()

	if stats.TotalExecutionLayers > 0 {
		stats.AverageStepsPerLayer = float64(len(executor.steps)) / float64(stats.TotalExecutionLayers)
	}
}

func (executor *DAGWorkflowExecutor) calculateExecutionLayers() int {
	layers := 0
	remaining := make(map[string]bool)
	for stepName := range executor.stepsByName {
		remaining[stepName] = true
	}

	for len(remaining) > 0 {
		currentLayer := []string{}

		// Find steps with no remaining dependencies
		for stepName := range remaining {
			step := executor.stepsByName[stepName]
			hasUnmetDeps := false

			for _, dep := range step.Dependencies {
				if remaining[dep] {
					hasUnmetDeps = true
					break
				}
			}

			if !hasUnmetDeps {
				currentLayer = append(currentLayer, stepName)
			}
		}

		// Remove current layer steps from remaining
		for _, stepName := range currentLayer {
			delete(remaining, stepName)
		}

		layers++

		// Prevent infinite loop in case of issues
		if layers > len(executor.steps) {
			break
		}
	}

	return layers
}

// ExecuteDAGWorkflow is a convenience method that can be added to NetworkClient
func (nc *NetworkClient) ExecuteDAGWorkflow(steps []*DAGWorkflowStep, initialBody interface{}, metadata map[string]interface{}) *DAGWorkflowResponse {
	executor := NewDAGWorkflowExecutor(nc, steps)
	return executor.Execute(initialBody, metadata)
}

// ExecuteDAGWorkflowWithOptions provides more control over DAG execution
func (nc *NetworkClient) ExecuteDAGWorkflowWithOptions(steps []*DAGWorkflowStep, initialBody interface{}, metadata map[string]interface{}, maxConcurrency int, stopOnFirstError bool) *DAGWorkflowResponse {
	executor := NewDAGWorkflowExecutor(nc, steps)
	executor.SetMaxConcurrency(maxConcurrency)
	executor.SetStopOnFirstError(stopOnFirstError)
	return executor.Execute(initialBody, metadata)
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
			Step: step,
			Error: &errors.ErrorDetails{
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
