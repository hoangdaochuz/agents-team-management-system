## Purpose

Provides workspace-level administration (members, roles, invites, audit log) and system-level
superadministration (organizations, feature flags, system health, system audit) for authorized
operators.

## ADDED Requirements

### Requirement: Members tab
The system SHALL list workspace members with their role, status, and last-active time, and SHALL
allow changing a member's role inline. A seat usage indicator SHALL reflect plan limits.

#### Scenario: Changing a role
- **WHEN** an admin changes a member's role via the inline selector
- **THEN** the API is called to update the role and the row reflects the new role on success

#### Scenario: Seat usage
- **WHEN** the members tab renders
- **THEN** a seat indicator shows used/total seats and notes that service accounts do not count

### Requirement: Roles and permissions matrix
The system SHALL display a capability matrix across Owner/Admin/Member roles.

#### Scenario: Viewing the matrix
- **WHEN** the user opens the "Roles & permissions" tab
- **THEN** each capability shows a granted/denied indicator per role

### Requirement: Audit log
The system SHALL display a workspace audit log with actor, action, target, time, and IP, and SHALL
offer an export action.

#### Scenario: Exporting the audit log
- **WHEN** the admin clicks "Export audit log"
- **THEN** the system requests an export via the API and reports success or failure

### Requirement: Pending approvals and invites
The system SHALL show a pending-approvals queue (join requests) with approve/decline actions, and
SHALL provide an invite modal accepting one or more emails and a role.

#### Scenario: Approving a request
- **WHEN** the admin approves a pending request
- **THEN** the request is removed from the queue, the API records the approval, and a success toast
  is shown

#### Scenario: Sending invites
- **WHEN** the admin submits the invite modal with emails and a role
- **THEN** invitations are sent via the API, the modal closes, and a success toast is shown

### Requirement: Sysadmin organizations view
The sysadmin console SHALL list all organizations with plan, workspace count, seat usage, and
status, and SHALL allow suspending/restoring an organization. Access SHALL require the superadmin
role.

#### Scenario: Suspending an org
- **WHEN** a superadmin suspends an active organization
- **THEN** the org's status updates to suspended via the API and the row offers a "Restore" action

#### Scenario: Unauthorized access
- **WHEN** a non-superadmin navigates to `/sysadmin`
- **THEN** the system shows a "no access" state

### Requirement: Feature flags
The sysadmin console SHALL list feature flags with a description and an enable/disable toggle.

#### Scenario: Toggling a flag
- **WHEN** a superadmin toggles a feature flag
- **THEN** the API is called to update it and the toggle reflects the new state

### Requirement: System health and system audit
The sysadmin console SHALL display system-health metrics (per-service health with a progress
indicator) and a system-audit feed of recent superadmin actions.

#### Scenario: Viewing system health
- **WHEN** the superadmin opens the system console
- **THEN** each tracked service shows a health percentage and a color-coded status

### Requirement: Cross-org sign-up requests
The sysadmin console SHALL list pending sign-up requests across organizations with an approve
action and a pending-count badge.

#### Scenario: Approving a cross-org request
- **WHEN** the superadmin approves a request
- **THEN** the request is removed from the list and the pending-count badge decrements
