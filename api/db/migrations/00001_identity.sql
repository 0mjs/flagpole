-- +goose Up
CREATE EXTENSION IF NOT EXISTS citext;

-- Roles are ordered by what they allow: each includes the ones before it,
-- except that approving is separate from editing (see flags/changes).
CREATE TYPE user_role AS ENUM ('viewer', 'editor', 'approver', 'admin');

CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         citext NOT NULL UNIQUE,
    name          text NOT NULL,
    role          user_role NOT NULL DEFAULT 'viewer',
    -- NULL until the user accepts an invite and sets a password.
    password_hash text,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    disabled_at   timestamptz
);

CREATE TYPE token_purpose AS ENUM ('invite', 'password_reset');

-- One-time tokens for invites and password resets. Only a hash is stored;
-- the token itself goes out in the email link once.
CREATE TABLE user_tokens (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users ON DELETE CASCADE,
    purpose    token_purpose NOT NULL,
    token_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    used_at    timestamptz,
    created_by uuid REFERENCES users ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX user_tokens_user ON user_tokens (user_id, purpose);

-- +goose Down
DROP TABLE user_tokens;
DROP TYPE token_purpose;
DROP TABLE users;
DROP TYPE user_role;
