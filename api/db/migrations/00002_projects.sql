-- +goose Up
-- Keys are what the URLs, SDKs and humans use: lowercase, stable, unique.
CREATE DOMAIN slug AS text CHECK (VALUE ~ '^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$');

CREATE TABLE projects (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    key         slug NOT NULL UNIQUE,
    name        text NOT NULL,
    description text NOT NULL DEFAULT '',
    created_by  uuid REFERENCES users ON DELETE SET NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE environments (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id        uuid NOT NULL REFERENCES projects ON DELETE CASCADE,
    key               slug NOT NULL,
    name              text NOT NULL,
    -- Changes to flags here need a second person's approval.
    requires_approval boolean NOT NULL DEFAULT false,
    position          integer NOT NULL DEFAULT 0,
    created_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, key)
);

-- Server-side SDK keys, one environment each. The key is shown once; only
-- its SHA-256 is kept, with a short prefix to tell keys apart.
CREATE TABLE sdk_keys (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    environment_id uuid NOT NULL REFERENCES environments ON DELETE CASCADE,
    name           text NOT NULL,
    prefix         text NOT NULL,
    key_hash       bytea NOT NULL UNIQUE,
    created_by     uuid REFERENCES users ON DELETE SET NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    revoked_at     timestamptz
);
CREATE INDEX sdk_keys_environment ON sdk_keys (environment_id);

-- +goose Down
DROP TABLE sdk_keys;
DROP TABLE environments;
DROP TABLE projects;
DROP DOMAIN slug;
