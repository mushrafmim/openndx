package models

// UserGroup represents different user groups in the system
type UserGroup string

const (
	UserGroupAdmin  UserGroup = "OpenNDX_Admin"
	UserGroupMember UserGroup = "OpenNDX_Members"
)

// Field length constraints remain as regular constants
const (
	MaxNameLength        = 255
	MaxDescriptionLength = 1000
	MaxEmailLength       = 320 // RFC 3696 specification
	MaxPhoneLength       = 15  // E.164 format
	MaxEndpointLength    = 2048
)

// IDP Application Constants
const (
	TemplateIDM2M = "m2m-application"
)
