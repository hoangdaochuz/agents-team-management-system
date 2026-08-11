-- Settings service schema (settings_db): provider API keys encrypted at rest
-- with a master key. Ciphertext never leaves this service; the runner fetches
-- plaintext over mTLS only, in-memory, per run.
CREATE TABLE IF NOT EXISTS provider_keys (
    provider     TEXT        PRIMARY KEY, -- openai | anthropic | gemini | glm
    ciphertext   BYTEA       NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
