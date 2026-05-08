-- ============================================================
-- 0002_tenants — Multi-tenancy + embedded apps
-- ============================================================
-- Modelo:
--   - tenants: una fila por workspace. El "casdoor_org" se reserva
--     para el día que provisionemos org en Casdoor por tenant
--     (Fase 5+). Hoy queda nullable.
--   - users: ya existe con (id, casdoor_sub). Le agregamos los
--     campos que necesita la app (tenant_id, email, role) sin
--     romper inserts previos.
--   - embedded_apps: catálogo por tenant de iframes registrados.
-- ============================================================

SET ROLE einar;

-- ------------------------------------------------------------
-- tenants
-- ------------------------------------------------------------
CREATE TABLE tenants (
    id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    slug         TEXT         NOT NULL UNIQUE,
    display_name TEXT         NOT NULL,
    -- Para Fase 5+: nombre del org en Casdoor cuando se aprovisione.
    casdoor_org  TEXT                  UNIQUE,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now()
);

COMMENT ON TABLE  tenants              IS 'Workspaces de la plataforma. Cada uno aísla sus users y embedded_apps.';
COMMENT ON COLUMN tenants.slug         IS 'Identificador URL-safe (sirve en /t/{slug}/...).';
COMMENT ON COLUMN tenants.casdoor_org  IS 'Nombre del org en Casdoor (Fase 5+); NULL hasta que se aprovisione.';

-- Validación básica del slug: lowercase + dígitos + guiones, 2-32 chars.
-- Evita slugs raros o demasiado cortos/largos.
ALTER TABLE tenants
    ADD CONSTRAINT tenants_slug_format
    CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,30}[a-z0-9]$');

-- ------------------------------------------------------------
-- users: extender la tabla existente
-- ------------------------------------------------------------
-- tenant_id NULLABLE intencionalmente: un user que recién se loguea
-- no tiene tenant todavía. Lo asigna el flujo /signup.
ALTER TABLE users
    ADD COLUMN tenant_id UUID    REFERENCES tenants(id) ON DELETE RESTRICT,
    ADD COLUMN email     TEXT,
    ADD COLUMN role      TEXT    NOT NULL DEFAULT 'member'
        CHECK (role IN ('owner', 'admin', 'member'));

COMMENT ON COLUMN users.tenant_id IS 'Tenant al que pertenece el user. NULL = recién creado, aún no completó signup.';
COMMENT ON COLUMN users.email     IS 'Cacheado del JWT; fuente de verdad sigue siendo Casdoor.';
COMMENT ON COLUMN users.role      IS 'Rol dentro del tenant: owner | admin | member.';

CREATE INDEX users_tenant_id_idx ON users(tenant_id);

-- ------------------------------------------------------------
-- embedded_apps
-- ------------------------------------------------------------
-- Cada tenant tiene N apps embebidas en su sidenav. El SDK de
-- iframes (Fase 4) lee este catálogo via /api/embedded-apps.
CREATE TABLE embedded_apps (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID         NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        TEXT         NOT NULL,
    -- Origin = scheme://host:port; el shell solo manda postMessage
    -- a este origin exacto. Sin trailing slash, sin path.
    origin      TEXT         NOT NULL,
    icon_url    TEXT,
    -- Posición en el sidenav (orden visual). El frontend lo usa para sort.
    position    INT          NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),

    -- Una app es única por (tenant, origin): no tiene sentido dos
    -- entradas hacia el mismo iframe.
    UNIQUE (tenant_id, origin)
);

COMMENT ON TABLE  embedded_apps         IS 'Iframes registrados por tenant. Source of truth para el sidenav del shell.';
COMMENT ON COLUMN embedded_apps.origin  IS 'Origin RFC 6454 (scheme://host[:port]). Whitelist para postMessage.';

CREATE INDEX embedded_apps_tenant_pos_idx ON embedded_apps(tenant_id, position);

RESET ROLE;
