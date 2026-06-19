-- Reference DDL. Consumers run this via their own migration tooling.
CREATE TABLE IF NOT EXISTS gnomon_snapshots (
    captured_on  DATE             NOT NULL,
    metric       TEXT             NOT NULL,
    dimension    TEXT             NOT NULL DEFAULT '',
    value        DOUBLE PRECISION NOT NULL,
    PRIMARY KEY (captured_on, metric, dimension)
);
