package bus

const (
	TopicSettingsUpdated      = "settings.updated"
	TopicStatusbarNotifyEvent = "statusbar.notification"
	TopicProjectSwitched      = "project.switched"
	// request to create a new project.
	TopicProjectCreate        = "project.create"
	TopicWorkspaceFileChanged = "workspace.file.changed"
	TopicGitBranchChanged     = "git.branch.changed"
	TopicGitFileStaged        = "git.file.staged"
	TopicPreviewToggle        = "preview.toggle" // for agent tool
)

type FileChangedEvent struct {
	Path string
}

var allTopics = []string{
	TopicSettingsUpdated,
	TopicStatusbarNotifyEvent,
	TopicProjectSwitched,
	TopicProjectCreate,
	TopicWorkspaceFileChanged,
	TopicGitBranchChanged,
	TopicGitFileStaged,
	TopicPreviewToggle,
}
