// Package contracts is the shared wire-contract kernel, now decomposed into
// per-domain subpackages (identity, workspaces, tasks, agentexec, resources,
// admin, events). This file is a TEMPORARY re-export bridge so services that
// still import the monolithic contracts package keep compiling during the
// DDD rollout; each converted service switches to the domain subpackages and
// the bridge is removed once no importer remains.
package contracts

import (
	"github.com/aaks/server/internal/contracts/admin"
	"github.com/aaks/server/internal/contracts/agentexec"
	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/resources"
	"github.com/aaks/server/internal/contracts/tasks"
	"github.com/aaks/server/internal/contracts/workspaces"
)

// ── Base scalars (identity) ────────────────────────────────────────────────

type ID = identity.ID
type ISOTime = identity.ISOTime

type RepoType = identity.RepoType

const (
	RepoTypePath = identity.RepoTypePath
	RepoTypeURL  = identity.RepoTypeURL
)

type Provider = identity.Provider

// ── Identity domain ────────────────────────────────────────────────────────

type Role = identity.Role

const (
	RoleOwner  = identity.RoleOwner
	RoleAdmin  = identity.RoleAdmin
	RoleMember = identity.RoleMember
)

type MemberStatus = identity.MemberStatus

const (
	MemberActive    = identity.MemberActive
	MemberInvited   = identity.MemberInvited
	MemberSuspended = identity.MemberSuspended
)

type Plan = identity.Plan

const (
	PlanFree       = identity.PlanFree
	PlanTeam       = identity.PlanTeam
	PlanPro        = identity.PlanPro
	PlanEnterprise = identity.PlanEnterprise
)

type OrgStatus = identity.OrgStatus

const (
	OrgActive    = identity.OrgActive
	OrgTrial     = identity.OrgTrial
	OrgSuspended = identity.OrgSuspended
)

type SignupState = identity.SignupState

const (
	SignupPending  = identity.SignupPending
	SignupApproved = identity.SignupApproved
	SignupDeclined = identity.SignupDeclined
)

type User = identity.User
type SignupRequest = identity.SignupRequest
type ProviderKey = identity.ProviderKey

// ── Workspaces domain ──────────────────────────────────────────────────────

type Session = workspaces.Session
type Organization = workspaces.Organization
type Workspace = workspaces.Workspace
type Member = workspaces.Member
type MemberUser = workspaces.MemberUser
type Invite = workspaces.Invite

// ── Tasks domain ───────────────────────────────────────────────────────────

type TaskStatus = tasks.TaskStatus

const (
	TaskBacklog   = tasks.TaskBacklog
	TaskDoing     = tasks.TaskDoing
	TaskReview    = tasks.TaskReview
	TaskDone      = tasks.TaskDone
	TaskBlocked   = tasks.TaskBlocked
	TaskCancelled = tasks.TaskCancelled
	TaskStopped   = tasks.TaskStopped
)

type TaskType = tasks.TaskType
type Priority = tasks.Priority

type Project = tasks.Project
type Task = tasks.Task
type Feedback = tasks.Feedback

// ── Agent-execution domain ─────────────────────────────────────────────────

type RunRole = agentexec.RunRole

const (
	RunRoleImplementer = agentexec.RunRoleImplementer
	RunRoleReviewer    = agentexec.RunRoleReviewer
)

type RunStatus = agentexec.RunStatus

const (
	RunRunning = agentexec.RunRunning
	RunDone    = agentexec.RunDone
	RunAborted = agentexec.RunAborted
	RunStopped = agentexec.RunStopped
)

type StepKind = agentexec.StepKind

const (
	StepMessage    = agentexec.StepMessage
	StepToolCall   = agentexec.StepToolCall
	StepToolResult = agentexec.StepToolResult
	StepReasoning  = agentexec.StepReasoning
)

type Severity = agentexec.Severity
type AutonomyMode = agentexec.AutonomyMode

const (
	AutonomyAssigned   = agentexec.AutonomyAssigned
	AutonomyMatching   = agentexec.AutonomyMatching
	AutonomyAutonomous = agentexec.AutonomyAutonomous
)

type VerdictDecision = agentexec.VerdictDecision

const (
	VerdictApprove        = agentexec.VerdictApprove
	VerdictRequestChanges = agentexec.VerdictRequestChanges
)

type Guardrails = agentexec.Guardrails
type Agent = agentexec.Agent
type Run = agentexec.Run
type Step = agentexec.Step
type Payload = agentexec.Payload
type Finding = agentexec.Finding
type Artifact = agentexec.Artifact

// ── Workspace resources domain ─────────────────────────────────────────────

type IndexStatus = resources.IndexStatus

const (
	IndexPending    = resources.IndexPending
	IndexIndexed    = resources.IndexIndexed
	IndexReindexing = resources.IndexReindexing
	IndexFailed     = resources.IndexFailed
)

type Skill = resources.Skill
type McpServer = resources.McpServer
type KnowledgeSource = resources.KnowledgeSource
type Plugin = resources.Plugin
type Rule = resources.Rule
type McpConnection = resources.McpConnection

// ── Admin domain ───────────────────────────────────────────────────────────

type AuditEntry = admin.AuditEntry
type AuditActor = admin.AuditActor
type FeatureFlag = admin.FeatureFlag
type ServiceHealth = admin.ServiceHealth
type SystemHealth = admin.SystemHealth
type SystemKpis = admin.SystemKpis

// ── Events ─────────────────────────────────────────────────────────────────

const (
	TopicTaskRunRequested    = events.TopicTaskRunRequested
	TopicTaskReviewRequested = events.TopicTaskReviewRequested
	TopicTaskStopRequested   = events.TopicTaskStopRequested
	TopicPrOpenRequested     = events.TopicPrOpenRequested

	TopicStep         = events.TopicStep
	TopicRunCompleted = events.TopicRunCompleted
	TopicFinding      = events.TopicFinding
	TopicVerdict      = events.TopicVerdict
	TopicPrOpened     = events.TopicPrOpened

	TopicTaskStatusChanged = events.TopicTaskStatusChanged

	TopicSignupRequested  = events.TopicSignupRequested
	TopicSignupApproved   = events.TopicSignupApproved
	TopicSignupDeclined   = events.TopicSignupDeclined
	TopicInviteCreated    = events.TopicInviteCreated
	TopicWorkspaceCreated = events.TopicWorkspaceCreated

	TopicMcpCreated   = events.TopicMcpCreated
	TopicMcpDeleted   = events.TopicMcpDeleted
	TopicSkillCreated = events.TopicSkillCreated
	TopicSkillDeleted = events.TopicSkillDeleted

	TopicRunStarted    = events.TopicRunStarted
	TopicAuditRecorded = events.TopicAuditRecorded
)

func AllTopics() []string { return events.AllTopics() }

func IsTaskPartitioned(topic string) bool { return events.IsTaskPartitioned(topic) }

type EventEnvelope = events.EventEnvelope

type RunRequestedData = events.RunRequestedData
type ReviewRequestedData = events.ReviewRequestedData
type StopRequestedData = events.StopRequestedData
type PrOpenRequestedData = events.PrOpenRequestedData
type StepData = events.StepData
type RunCompletedData = events.RunCompletedData
type FindingData = events.FindingData
type VerdictData = events.VerdictData
type PrOpenedData = events.PrOpenedData
type TaskStatusChangedData = events.TaskStatusChangedData
type SignupRequestedData = events.SignupRequestedData
type SignupApprovedData = events.SignupApprovedData
type SignupDeclinedData = events.SignupDeclinedData
type InviteCreatedData = events.InviteCreatedData
type WorkspaceCreatedData = events.WorkspaceCreatedData
type McpCreatedData = events.McpCreatedData
type McpDeletedData = events.McpDeletedData
type SkillCreatedData = events.SkillCreatedData
type SkillDeletedData = events.SkillDeletedData
type RunStartedData = events.RunStartedData
type AuditRecordedData = events.AuditRecordedData
