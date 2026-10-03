-- +goose Up
CREATE TYPE flag_kind AS ENUM ('boolean', 'string', 'number', 'json');

CREATE TABLE flags (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  uuid NOT NULL REFERENCES projects ON DELETE CASCADE,
    key         text NOT NULL CHECK (key ~ '^[a-z0-9][a-z0-9._-]{0,79}$'),
    name        text NOT NULL,
    description text NOT NULL DEFAULT '',
    kind        flag_kind NOT NULL,
    tags        text[] NOT NULL DEFAULT '{}',
    created_by  uuid REFERENCES users ON DELETE SET NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    UNIQUE (project_id, key)
);

-- The values a flag can serve. A boolean flag has "on" and "off".
CREATE TABLE flag_variants (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    flag_id     uuid NOT NULL REFERENCES flags ON DELETE CASCADE,
    key         text NOT NULL CHECK (key ~ '^[a-z0-9][a-z0-9_-]{0,39}$'),
    value       jsonb NOT NULL,
    description text NOT NULL DEFAULT '',
    position    integer NOT NULL,
    UNIQUE (flag_id, key)
);

-- How a flag behaves in one environment. Rules are an ordered list,
-- validated by the application; version guards against lost updates and
-- lets SDKs cache by it.
CREATE TABLE flag_configs (
    flag_id         uuid NOT NULL REFERENCES flags ON DELETE CASCADE,
    environment_id  uuid NOT NULL REFERENCES environments ON DELETE CASCADE,
    enabled         boolean NOT NULL DEFAULT false,
    default_variant text NOT NULL,
    off_variant     text NOT NULL,
    rules           jsonb NOT NULL DEFAULT '[]',
    version         integer NOT NULL DEFAULT 1,
    updated_by      uuid REFERENCES users ON DELETE SET NULL,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (flag_id, environment_id)
);
CREATE INDEX flag_configs_environment ON flag_configs (environment_id);

CREATE TYPE change_status AS ENUM ('pending', 'applied', 'rejected', 'cancelled', 'conflicted');

-- A proposed config for an environment that requires approval. Approving
-- applies it, if the config is still at base_version.
CREATE TABLE change_requests (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    flag_id        uuid NOT NULL REFERENCES flags ON DELETE CASCADE,
    environment_id uuid NOT NULL REFERENCES environments ON DELETE CASCADE,
    author_id      uuid NOT NULL REFERENCES users,
    status         change_status NOT NULL DEFAULT 'pending',
    base_version   integer NOT NULL,
    proposed       jsonb NOT NULL,
    comment        text NOT NULL DEFAULT '',
    reviewer_id    uuid REFERENCES users,
    review_comment text NOT NULL DEFAULT '',
    reviewed_at    timestamptz,
    created_at     timestamptz NOT NULL DEFAULT now()
);
-- One open request per flag and environment keeps review simple.
CREATE UNIQUE INDEX change_requests_one_pending
    ON change_requests (flag_id, environment_id) WHERE status = 'pending';
CREATE INDEX change_requests_status ON change_requests (status, created_at DESC);

-- Who did what, written in the same transaction as the change.
CREATE TABLE audit_events (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_id    uuid REFERENCES users ON DELETE SET NULL,
    action      text NOT NULL,
    project_id  uuid REFERENCES projects ON DELETE CASCADE,
    entity_type text NOT NULL,
    entity_id   uuid,
    summary     text NOT NULL,
    data        jsonb NOT NULL DEFAULT '{}',
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_events_project ON audit_events (project_id, id DESC);

-- Transactional outbox: events committed with the change, published to
-- Kafka by the worker, so none is lost or sent for a rolled-back change.
CREATE TABLE outbox (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    topic        text NOT NULL,
    key          text NOT NULL,
    payload      jsonb NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz
);
CREATE INDEX outbox_unpublished ON outbox (id) WHERE published_at IS NULL;

-- +goose Down
DROP TABLE outbox;
DROP TABLE audit_events;
DROP TABLE change_requests;
DROP TYPE change_status;
DROP TABLE flag_configs;
DROP TABLE flag_variants;
DROP TABLE flags;
DROP TYPE flag_kind;
