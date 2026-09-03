package github

// Event describes a supported GitHub webhook event type.
type Event struct {
	Name  string // GitHub event name (e.g. "push")
	Label string // Human label (e.g. "Push events")
	Short string // Short identifier (e.g. "p")
}

// SupportedEvents lists the GitHub webhook event types that the bot can
// subscribe to and (where applicable) format for Telegram.
var SupportedEvents = []Event{
	{Name: "push", Label: "Push (commits)", Short: "p"},
	{Name: "pull_request", Label: "Pull requests", Short: "pr"},
	{Name: "pull_request_review", Label: "PR reviews", Short: "prr"},
	{Name: "pull_request_review_comment", Label: "PR review comments", Short: "prrc"},
	{Name: "pull_request_review_thread", Label: "PR review threads", Short: "prrt"},
	{Name: "issues", Label: "Issues", Short: "i"},
	{Name: "issue_comment", Label: "Issue/PR comments", Short: "ic"},
	{Name: "commit_comment", Label: "Commit comments", Short: "cc"},
	{Name: "check_run", Label: "Check runs", Short: "cr"},
	{Name: "check_suite", Label: "Check suites", Short: "cs"},
	{Name: "workflow_run", Label: "Workflow runs", Short: "wr"},
	{Name: "workflow_job", Label: "Workflow jobs", Short: "wj"},
	{Name: "workflow_dispatch", Label: "Workflow dispatch", Short: "wd"},
	{Name: "release", Label: "Releases", Short: "rel"},
	{Name: "deployment", Label: "Deployments", Short: "dep"},
	{Name: "deployment_status", Label: "Deployment status", Short: "deps"},
	{Name: "fork", Label: "Forks", Short: "f"},
	{Name: "star", Label: "Stars", Short: "s"},
	{Name: "watch", Label: "Watch", Short: "w"},
	{Name: "member", Label: "Collaborator changes", Short: "m"},
	{Name: "team", Label: "Team changes", Short: "t"},
	{Name: "team_add", Label: "Team add", Short: "ta"},
	{Name: "membership", Label: "Membership", Short: "mem"},
	{Name: "organization", Label: "Organization", Short: "org"},
	{Name: "public", Label: "Repo made public", Short: "pub"},
	{Name: "repository", Label: "Repository settings", Short: "rep"},
	{Name: "create", Label: "Branch/tag created", Short: "cre"},
	{Name: "delete", Label: "Branch/tag deleted", Short: "del"},
	{Name: "label", Label: "Label changes", Short: "lbl"},
	{Name: "milestone", Label: "Milestone changes", Short: "ms"},
	{Name: "gollum", Label: "Wiki", Short: "g"},
	{Name: "page_build", Label: "Pages build", Short: "pb"},
	{Name: "status", Label: "Commit status", Short: "st"},
	{Name: "deployment_protection_rule", Label: "Deployment protection rule", Short: "dpr"},
	{Name: "deployment_review", Label: "Deployment review", Short: "dr"},
	{Name: "deploy_key", Label: "Deploy keys", Short: "dk"},
	{Name: "discussion", Label: "Discussions", Short: "disc"},
	{Name: "discussion_comment", Label: "Discussion comments", Short: "discc"},
	{Name: "meta", Label: "Webhook ping/meta", Short: "mt"},
	{Name: "ping", Label: "Ping", Short: "pi"},
	{Name: "installation", Label: "App installation", Short: "inst"},
	{Name: "installation_repositories", Label: "App installation repos", Short: "instr"},
}

// EventShortToName converts a short code back to the full event name.
func EventShortToName(short string) (string, bool) {
	for _, e := range SupportedEvents {
		if e.Short == short {
			return e.Name, true
		}
	}
	return "", false
}

// EventNameToShort converts a full event name to its short code.
func EventNameToShort(name string) (string, bool) {
	for _, e := range SupportedEvents {
		if e.Name == name {
			return e.Short, true
		}
	}
	return "", false
}

// AllEventNames returns the list of all supported event names.
func AllEventNames() []string {
	out := make([]string, len(SupportedEvents))
	for i, e := range SupportedEvents {
		out[i] = e.Name
	}
	return out
}
