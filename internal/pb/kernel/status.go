package kernel

// Status represents the status of submissions and applications
type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusRejected Status = "rejected"
)

// Version represents application versioning states
type Version string

const (
	ActiveVersion     Version = "active"
	DeprecatedVersion Version = "deprecated"
)
