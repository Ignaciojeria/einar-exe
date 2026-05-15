SET ROLE einar;

CREATE TABLE projects (
    id         UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  UUID         NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name       TEXT         NOT NULL,
    slug       TEXT         NOT NULL,
    path       TEXT         NOT NULL UNIQUE,
    subdomain  TEXT         NOT NULL UNIQUE,
    status     TEXT         NOT NULL DEFAULT 'ready'
        CHECK (status IN ('pending', 'ready', 'failed')),
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT now(),

    UNIQUE (tenant_id, slug)
);

ALTER TABLE projects
    ADD CONSTRAINT projects_slug_format
    CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,30}[a-z0-9]$');

CREATE INDEX projects_tenant_id_idx ON projects(tenant_id);

RESET ROLE;
