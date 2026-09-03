package ghops

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	gh "github.com/google/go-github/v66/github"
)

// ListWorkflows returns the workflows defined in the repo.
func ListWorkflows(ctx context.Context, c *gh.Client, owner, repo string, page, perPage int) ([]*gh.Workflow, error) {
	if perPage <= 0 {
		perPage = 10
	}
	list, _, err := c.Actions.ListWorkflows(ctx, owner, repo, &gh.ListOptions{Page: page, PerPage: perPage})
	if err != nil {
		return nil, ClassifyError(err)
	}
	return list.Workflows, nil
}

// ListWorkflowRuns returns recent workflow runs.
func ListWorkflowRuns(ctx context.Context, c *gh.Client, owner, repo string, page, perPage int) ([]*gh.WorkflowRun, error) {
	if perPage <= 0 {
		perPage = 10
	}
	opts := &gh.ListWorkflowRunsOptions{ListOptions: gh.ListOptions{Page: page, PerPage: perPage}}
	list, _, err := c.Actions.ListRepositoryWorkflowRuns(ctx, owner, repo, opts)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return list.WorkflowRuns, nil
}

// DispatchWorkflow triggers a workflow_dispatch event on a branch.
func DispatchWorkflow(ctx context.Context, c *gh.Client, owner, repo, workflowFileName, ref string, inputs map[string]interface{}) error {
	workflowID := workflowFileName
	if !strings.HasSuffix(workflowID, ".yml") && !strings.HasSuffix(workflowID, ".yaml") {
		workflowID += ".yml"
	}
	_, err := c.Actions.CreateWorkflowDispatchEventByFileName(ctx, owner, repo, workflowID, gh.CreateWorkflowDispatchEventRequest{
		Ref:    ref,
		Inputs: inputs,
	})
	return ClassifyError(err)
}

// RerunFailedJobs reruns only failed jobs of a workflow run.
func RerunFailedJobs(ctx context.Context, c *gh.Client, owner, repo string, runID int64) error {
	_, err := c.Actions.RerunFailedJobsByID(ctx, owner, repo, runID)
	return ClassifyError(err)
}

// RerunWorkflow reruns all jobs of a workflow run.
func RerunWorkflow(ctx context.Context, c *gh.Client, owner, repo string, runID int64) error {
	_, err := c.Actions.RerunWorkflowByID(ctx, owner, repo, runID)
	return ClassifyError(err)
}

// CancelWorkflowRun cancels an in-progress run.
func CancelWorkflowRun(ctx context.Context, c *gh.Client, owner, repo string, runID int64) error {
	_, err := c.Actions.CancelWorkflowRunByID(ctx, owner, repo, runID)
	return ClassifyError(err)
}

// GetWorkflowRunLogsURL returns a temporary URL to download the logs ZIP.
// The URL is pre-signed by GitHub and expires in ~10 minutes.
func GetWorkflowRunLogsURL(ctx context.Context, c *gh.Client, owner, repo string, runID int64) (string, error) {
	url, _, err := c.Actions.GetWorkflowRunLogs(ctx, owner, repo, runID, 1)
	if err != nil {
		// If a 410 Gone is returned, logs may have expired.
		return "", ClassifyError(err)
	}
	if url == nil {
		return "", fmt.Errorf("ghops: logs URL is nil")
	}
	return url.String(), nil
}

// FetchLogsLimited downloads up to maxBytes of the logs archive and returns
// the raw bytes. Caller is responsible for sanitising before display.
func FetchLogsLimited(ctx context.Context, client *http.Client, url string, maxBytes int64) ([]byte, error) {
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ghops: logs download returned status %d", resp.StatusCode)
	}
	limited := io.LimitReader(resp.Body, maxBytes)
	return io.ReadAll(limited)
}
