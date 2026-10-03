-- +goose Up
-- Exposures aggregated by the worker from Kafka, one row per hour.
CREATE TABLE exposure_counts (
    flag_id        uuid NOT NULL REFERENCES flags ON DELETE CASCADE,
    environment_id uuid NOT NULL REFERENCES environments ON DELETE CASCADE,
    variant        text NOT NULL,
    bucket         timestamptz NOT NULL,
    count          bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (flag_id, environment_id, bucket, variant)
);

-- +goose Down
DROP TABLE exposure_counts;
