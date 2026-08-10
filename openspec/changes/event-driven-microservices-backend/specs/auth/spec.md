## Purpose

Owns the User and Session lifecycle: email/password sign-in, signup in join-workspace or
create-org mode, the awaiting-approval flow, signout, and SSO entry points. The SPA gates
every route behind a session (`RequireAuth`/`RequireRole`), so no other capability is
usable until this one issues sessions. The backend-side behavior behind
`frontend/src/api/auth.ts`.

## ADDED Requirements

### Requirement: Email/password sign-in
The service SHALL accept `POST /api/auth/login` with `{ email, password, remember? }`,
validate the credentials against the stored user record, and on success issue a session
(the client sends no auth header, so the session SHALL be an httpOnly cookie) and return
the `Session` shape (`{ user, workspaces, active_workspace_id? }`). On failure it SHALL
return 401 and SHALL NOT issue a session.

#### Scenario: Valid credentials
- **WHEN** the frontend calls `login` with a valid email and password
- **THEN** the service returns the `Session` with the user's role and workspace memberships, and a session cookie is set

#### Scenario: Invalid credentials
- **WHEN** the credentials do not match a known active user
- **THEN** the service responds 401 and no session is created

#### Scenario: Unapproved user
- **WHEN** the credentials match a user whose signup request has not been approved
- **THEN** the service responds 403 with a pending state so the SPA can route to the awaiting-approval screen

### Requirement: Session hydration
The service SHALL serve `GET /api/auth/me`, returning the `Session` for the request's
session cookie, and SHALL respond 401 when the cookie is absent, expired, or invalid.

#### Scenario: Valid session on boot
- **WHEN** the frontend calls `me()` with a valid session cookie
- **THEN** the service returns the `Session` (user, roles, workspace memberships)
- **AND** the Gateway composes the active-workspace context used to scope subsequent requests

#### Scenario: No active session
- **WHEN** `me()` is called without a valid session
- **THEN** the service responds 401 and the SPA redirects to `/login`

### Requirement: Sign-out
The service SHALL accept `POST /api/auth/logout`, invalidate the session server-side, and
clear the cookie.

#### Scenario: Signing out
- **WHEN** the frontend calls `logout`
- **THEN** the session is invalidated, the cookie is cleared, and the service responds 204

### Requirement: Signup with join-or-create modes
The service SHALL accept `POST /api/auth/signup` with
`{ full_name, email, password, start_mode, invite_code?, organization_name? }` and SHALL
record an access request that is NOT active until approved. In `join` mode the request
targets the workspace named by `invite_code`; in `create` mode it targets a new
organization (and its initial workspace) pending cross-org approval. In both modes the
service SHALL return `{ request_id }` and a `pending` signup status. The service SHALL
reject duplicate signups for the same email and reject `join` mode with an unknown invite
code.

#### Scenario: Joining a workspace
- **WHEN** the user signs up with `start_mode: "join"` and a valid `invite_code`
- **THEN** the service records a pending join request for that workspace and returns `{ request_id }`

#### Scenario: Creating a new organization
- **WHEN** the user signs up with `start_mode: "create"` and an `organization_name`
- **THEN** the service records a pending organization signup (with its initial workspace)
  and returns `{ request_id }`

#### Scenario: Unknown invite code
- **WHEN** a `join` signup carries an invite code that matches no workspace
- **THEN** the service responds 400/404 and records no request

### Requirement: Signup status and resend
The service SHALL serve `GET /api/auth/signup-status` returning
`{ state: pending | approved | declined, email, workspace_name?, admin_name? }` for the
requesting identity, and `POST /api/auth/signup-status/resend` to re-send the
notification. Resend SHALL be idempotent.

#### Scenario: Polling a pending request
- **WHEN** the frontend polls `signupStatus` for an unapproved request
- **THEN** the service returns `state: "pending"` with the target workspace name

#### Scenario: Resend notification
- **WHEN** the frontend calls `resendSignupNotification`
- **THEN** the service re-sends the notification and responds 204

### Requirement: Approval activates access
When a workspace admin or superadmin approves a pending request (see the admin-console
capability), the service SHALL activate the user so that sign-in succeeds and the session
includes the new workspace membership, and `GET /api/auth/signup-status` SHALL then report
`state: "approved"`. Declining SHALL make sign-up report `state: "declined"` and keep the
user inactive.

#### Scenario: Request approved
- **WHEN** an admin approves the pending request for an email
- **THEN** that user can sign in, the session lists the workspace, and `signup-status` returns `approved`

#### Scenario: Request declined
- **WHEN** an admin declines the pending request
- **THEN** the user remains inactive and `signup-status` returns `declined`

### Requirement: SSO begin (stub)
The service SHALL accept `POST /api/auth/sso/begin` with `{ provider: google | saml }` and
SHALL return `{ redirect_url }`. When the provider is not configured, it SHALL return a
documented error rather than a broken URL; the actual provider exchange (callback, token
verification) is out of scope.

#### Scenario: Configuring an SSO provider
- **WHEN** the frontend calls `ssoBegin("google")` and the provider is configured
- **THEN** the service returns a usable `{ redirect_url }`
- **AND** when the provider is not configured, it responds with an error the SPA surfaces gracefully

### Requirement: Roles in the session
The `Session.user` SHALL carry the user's `role` within the active workspace
(`owner | admin | member`) and `is_superadmin` when the user has system-wide access, so
the SPA's route guards and the Gateway's authorization checks have the same source of
truth.

#### Scenario: Superadmin session
- **WHEN** `me()` returns a superadmin user
- **THEN** the session marks `is_superadmin: true` and the SPA can render `/sysadmin`
