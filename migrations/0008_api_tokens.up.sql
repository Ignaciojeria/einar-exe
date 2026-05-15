SET ROLE einar;

CREATE TABLE api_tokens (
    id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT         NOT NULL,
    token_prefix TEXT         NOT NULL,
    token_hash   TEXT         NOT NULL UNIQUE,
    scopes       TEXT[]       NOT NULL DEFAULT ARRAY[]::TEXT[],
    last_used_at TIMESTAMPTZ,
    expires_at   TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX api_tokens_user_id_idx ON api_tokens(user_id);
CREATE INDEX api_tokens_prefix_idx ON api_tokens(token_prefix);
CREATE INDEX api_tokens_active_idx ON api_tokens(token_hash) WHERE revoked_at IS NULL;

RESET ROLE;
