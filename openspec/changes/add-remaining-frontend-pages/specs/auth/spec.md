## Purpose

Lets users sign in, request access to a workspace, and establish a session that gates every other
page. Defines the authentication, signup, approval, and access-control behavior the SPA enforces
client-side against a backend that implements it later.

## ADDED Requirements

### Requirement: Email/password sign-in
The system SHALL provide a sign-in screen accepting a work email and password, plus a "keep me
signed in" option. On success the system SHALL establish a session and route the user to their
last workspace's dashboard.

#### Scenario: Valid credentials
- **WHEN** the user submits a valid email and password
- **THEN** the system creates a session, stores the current user and active workspace, and navigates
  to `/dashboard`

#### Scenario: Invalid credentials
- **WHEN** the backend rejects the credentials
- **THEN** the form stays mounted with its values, an inline error is shown, and no session is
  created

#### Scenario: Backend not implemented
- **WHEN** the sign-in call fails with a connection/404 error
- **THEN** the page renders its full layout and surfaces a friendly "backend not implemented yet"
  message instead of crashing

### Requirement: SSO entry points
The system SHALL present Google Workspace and SAML single-sign-on buttons on both sign-in and
sign-up screens.

#### Scenario: Initiating SSO
- **WHEN** the user clicks an SSO button
- **THEN** the system initiates the SSO flow (declared as a stub until the backend implements it)
  and never leaves the screen in a broken intermediate state

### Requirement: Signup with join-or-create modes
The system SHALL provide a signup screen with a segmented choice between "Join a workspace"
(requires an invite code/URL) and "Create new org" (requires an organization name). On submit the
system SHALL record the request and route to the awaiting-approval screen.

#### Scenario: Joining a workspace
- **WHEN** the user selects "Join a workspace", enters their name, email, password, and an invite
  code, and submits
- **THEN** the system records the access request and navigates to `/pending`

#### Scenario: Creating a new organization
- **WHEN** the user selects "Create new org" and fills the organization name field
- **THEN** the join-code field is hidden, the organization name field is required, and submission
  records the request and navigates to `/pending`

#### Scenario: Terms not accepted
- **WHEN** the user submits without agreeing to the Terms
- **THEN** submission is blocked with an inline validation error

### Requirement: Awaiting-approval state
The system SHALL show an "awaiting approval" screen after signup, naming the requesting email and
target workspace, with a step progress indicator and a "resend notification" action.

#### Scenario: Viewing pending status
- **WHEN** an authenticated-but-unapproved user lands on `/pending`
- **THEN** the screen shows the four-step progress (account created → request submitted → admin
  review → access granted) with "admin review" as the current step

#### Scenario: Resend notification
- **WHEN** the user clicks "Resend notification"
- **THEN** a confirmation toast is shown and the request is re-sent via the API client

### Requirement: Session and current user
The system SHALL expose the authenticated user, their role(s), and the active workspace via a
session store hydrated from `GET /api/auth/me`. Sign-out SHALL clear the session and return to the
sign-in screen.

#### Scenario: Hydrating on load
- **WHEN** the app boots
- **THEN** it calls `me()` and, if a valid session exists, populates the session store before
  rendering protected routes

#### Scenario: No active session
- **WHEN** `me()` indicates no session (or the backend is unimplemented)
- **THEN** protected routes redirect to `/login`

#### Scenario: Sign out
- **WHEN** the user signs out
- **THEN** the session store is cleared, `POST /api/auth/logout` is called, and the user is routed
  to `/login`

### Requirement: Route access control
The system SHALL guard all non-auth routes so that unauthenticated users are redirected to
`/login`, and SHALL role-gate the admin and sysadmin routes so unauthorized users see a
"no access" state.

#### Scenario: Unauthenticated access to a protected route
- **WHEN** a user without a session navigates to `/board`
- **THEN** they are redirected to `/login` with a return-to path preserved

#### Scenario: Non-admin accessing admin console
- **WHEN** a member-role user navigates to `/admin`
- **THEN** the system shows a "no access" state rather than the admin content

#### Scenario: No-auth dev fallback
- **WHEN** the backend is unimplemented and `me()` errors in development
- **THEN** the system MAY fall back to a synthetic single-operator session (toggleable) so the rest
  of the SPA remains usable, and SHALL clearly indicate it is in dev-fallback mode
