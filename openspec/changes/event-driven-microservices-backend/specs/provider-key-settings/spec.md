## Purpose

Stores provider API keys and git credentials encrypted at rest, and is the sole component
permitted to decrypt them. It hands plaintext secrets to the Agent-Runner over an
authenticated internal channel so the sandbox container never receives any credential.

## ADDED Requirements

### Requirement: Provider key CRUD matching the frontend contract
The service SHALL implement `listProviderKeys`, `setProviderKey` (`POST /api/provider-keys`
with `{ provider, api_key }`), `updateProviderKey` (`PUT /api/provider-keys/:provider` with
`{ api_key }`), and `deleteProviderKey`, returning `ProviderKey` objects that expose
`{ provider, created_at }` only — the API key MUST NOT appear in any response.

#### Scenario: Setting a key
- **WHEN** the frontend calls `setProviderKey(provider, apiKey)`
- **THEN** the service stores the key encrypted and returns `{ provider, created_at }`
  without the secret

#### Scenario: Listing keys
- **WHEN** the frontend calls `listProviderKeys`
- **THEN** the service returns provider metadata only, never key material

#### Scenario: Deleting a key
- **WHEN** the frontend calls `deleteProviderKey(provider)`
- **THEN** the service removes the key and responds 204

### Requirement: Plaintext secrets never leave this service except to the runner
The service SHALL be the sole decryptor of stored secrets. Plaintext provider keys and git
credentials MUST be released only to the Agent-Runner over an authenticated, encrypted
internal channel, and MUST NOT be returned through any frontend-facing endpoint, written to
logs, or persisted in plaintext anywhere.

#### Scenario: Frontend attempts to read a key
- **WHEN** any frontend-facing endpoint would return a stored secret
- **THEN** the service omits the secret entirely

#### Scenario: Log safety
- **WHEN** the service processes a key
- **THEN** no plaintext secret is written to logs

### Requirement: Credential handoff to the runner
The service SHALL provide an internal endpoint (authenticated, e.g. mTLS) by which the
Agent-Runner obtains a plaintext secret for a given provider (and the git credential) for
the duration of a run. The plaintext MUST be consumed in the runner's process memory only
and MUST NOT be placed in the sandbox container's environment or filesystem.

#### Scenario: Runner requests a provider key
- **WHEN** the Agent-Runner, authenticated, requests the key for a provider at run start
- **THEN** the service returns the plaintext key for in-process use only

#### Scenario: Sandbox isolation
- **WHEN** a task container is started for a run
- **THEN** the container holds no provider keys and no git credentials

### Requirement: Encryption at rest
Secrets SHALL be encrypted at rest using a master key held by this service. A lost master
key MUST be the only way bulk decryption becomes possible; individual key rotation SHALL be
supported via the set/update endpoints.

#### Scenario: Storage compromise
- **WHEN** the secret store is read directly (e.g. a DB dump)
- **THEN** only ciphertext is recoverable, not usable keys
