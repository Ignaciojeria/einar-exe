# Next steps

Backlog priorizado del MVP cerrado al final de Fase 6 (Metabase). Cada
ítem viene con: motivación, alcance estimado, y dependencias.

---

## Tier 1 — Calidad / robustez (ataca antes de invitar usuarios reales)

### 1.1 Tests unitarios del backend Go

**Por qué**: hoy no hay un solo test. Cada cambio en `signup`, `auth`,
o repos va a ciegas. Una regresión en el flow de provisioning silente
puede dejar tenants huérfanos en Casdoor/OO/Metabase.

**Alcance** (~1 sesión):
- `internal/domain`: tests de invariantes (`HasTenant`, validaciones)
- `internal/adapter/out/postgres`: tests de integración con
  `testcontainers-go` levantando Postgres real
- `internal/adapter/in/http`: tests de handlers con `httptest` +
  mocks de los repos

**Dependencias**: ninguna.

### 1.2 CI/CD en GitHub Actions

**Por qué**: el push a main hoy no valida nada. Un build roto puede
quedar en `main` hasta que se note manual.

**Alcance** (~30 min):
- Workflow `ci.yml`: `go vet`, `go build`, `go test`, `npm run build`
- Cache de módulos Go y `node_modules`
- Status check obligatorio para PRs (cuando empieces a usar branches)
- Imagen Docker build a `ghcr.io` opcional para tener artefactos

**Dependencias**: tests (Tier 1.1). Sin tests, CI solo valida compilación.

### 1.3 Encriptación at-rest de credenciales

**Por qué**: hoy `tenants.openobserve_user_password` y
`metabase_user_password` se guardan plaintext. Comentado como TODO en
las migraciones 0005 y 0006. Si alguien con acceso a la DB filtra una
copia, comprometen credenciales de todos los tenants.

**Alcance** (~1 sesión):
- Variable `APP_SECRET_KEY` (32 bytes random) en `.env`
- Helper `internal/shared/crypto/aes.go` con AES-GCM
- Migración: encriptar columnas existentes, marcarlas `_encrypted`
- Repo: encriptar al `Set*Credentials`, decriptar al `FindBy*`
- Rotación de la key: out of scope hasta que importe

**Dependencias**: tests (1.1) para no romper nada.

### 1.4 Rollback transaccional del signup

**Por qué**: signup hace 4 calls externos (Casdoor + OO + Metabase + DB).
Si OO falla, hoy hacemos rollback del tenant en DB. Pero si Casdoor se
creó OK y Postgres falla mid-transaction, queda un casdoor org huérfano.

**Alcance** (~1 sesión):
- Reificar el flujo de signup en `internal/usecase/signup.go`
- Patrón saga: cada paso tiene `Do` + `Compensate`
- Si cualquier paso falla, las compensaciones de los anteriores corren
  en orden inverso
- Job de reconciliación nocturno que detecta huérfanos y limpia

**Dependencias**: tests (1.1).

---

## Tier 2 — UX / pulido (cuando entren los primeros usuarios)

### 2.1 SSO real: Casdoor → Metabase + OpenObserve

**Por qué**: hoy el user logea **dos veces** (una al shell con Google,
otra a Metabase/OO con la password generada). Es UX de mierda y va a
generar tickets de soporte. SSO real elimina el segundo login.

**Alcance** (~2-3 sesiones):
- **Metabase**: tiene SSO via JWT en sus features paid (€$). En OSS,
  alternativas: OAuth 2.0 con un OIDC provider externo (Casdoor sí
  funciona) — Metabase OSS lo soporta vía `MB_SSO_OAUTH2_*`.
- **OpenObserve**: soporta OIDC nativo via Dex. Hay que sumar Dex como
  proxy entre OO y Casdoor (o configurar OO con Casdoor directo si
  alguna versión nueva lo permite).
- **Provisioning**: con SSO, los users se crean lazy en cada tool al
  primer login. Eliminamos el storage de passwords plaintext.

**Dependencias**: 1.3 (encriptación deja de ser bloqueante porque ya
no hay passwords que encriptar).

### 2.2 Rotar credenciales desde la UI

**Por qué**: si una credencial se filtra, hoy hay que ir a la DB y al
admin de la herramienta a mano. UX dolorosa.

**Alcance** (~1 sesión):
- Endpoint `POST /api/credentials/{tool}/rotate`
- Llama al admin API de la tool, regenera password, persiste, devuelve
- Botón "Rotar" en cada card de `/t/{slug}/credentials`
- Confirmación de "vas a romper sesiones activas, ¿seguro?"

**Dependencias**: 2.1 hace esto innecesario para users humanos, pero
útil para credenciales API tipo service-account.

### 2.3 Multi-user por tenant + invites

**Por qué**: hoy un tenant tiene UN user en Metabase/OO compartido por
todos los miembros del tenant einar. Audit log queda corrupto, no hay
distinción entre quien hizo qué.

**Alcance** (~2 sesiones):
- Migración: tabla `tenant_invites(token, email, role, tenant_id)`
- Endpoint `POST /api/invites` (owner/admin)
- Endpoint `POST /auth/accept-invite/{token}`
- Cada user de einar tiene su propio user en Metabase/OO scopeado al
  group del tenant
- UI: pantalla `/t/{slug}/team` para gestionar miembros e invites

**Dependencias**: 2.1 (sin SSO el provisioning de users por tenant
multiplica passwords plaintext en DB).

### 2.4 Roles y permisos finos

**Por qué**: hoy el role es `owner | admin | member` y los handlers
solo distinguen "puede" vs "no puede". Faltan reglas tipo:
- member NO puede ver creds de otros members
- admin puede invitar pero no transferir ownership
- owner puede borrar el tenant entero

**Alcance** (~1-2 sesiones):
- Migración: tabla `permissions(role, resource, action)` o RBAC casbin
- Middleware genérico que evalúa `(user, resource, action)`
- Refactor de handlers para usar el middleware

**Dependencias**: 2.3 (multi-user) le da sentido.

---

## Tier 3 — Plataforma / escala

### 3.1 Observabilidad de la propia app

**Por qué**: tenemos OpenObserve para los tenants pero la app einar no
manda sus propios logs/metrics ahí. "Cobbler's children have no shoes."

**Alcance** (~1 sesión):
- Wrapper de logging que mande a OO via OTLP HTTP
- Métricas básicas: signup rate, errors por endpoint, latencias
- Dashboard pre-armado en una collection "platform" de Metabase

**Dependencias**: ninguna.

### 3.2 Subdominios por tenant

**Por qué**: `https://einar.exe.xyz/t/acme/` es funcional pero feo.
`https://acme.einar.exe.xyz/` es lo que esperan SaaS users.

**Alcance** (~1 sesión, pero requiere infra):
- exe.dev: agregar CNAME por tenant manualmente (no soportan wildcard)
- Caddy: detectar subdominio en lugar de path
- Cookie scope a `.einar.exe.xyz` para que la sesión cruce subdominios

**Dependencias**: tu propio dominio si querés `*.tudominio.com` (exe.dev
no maneja wildcards, requiere CNAME por tenant). Postergar hasta
producción real.

### 3.3 Plan / billing

**Por qué**: si esto es un SaaS comercial, en algún momento hay que
cobrar.

**Alcance** (~3-5 sesiones):
- Stripe / Paddle integración
- Migración: `tenants.plan`, `subscriptions`, `usage_events`
- Webhook handler para eventos de Stripe
- UI de billing en `/t/{slug}/billing`
- Limits enforcement (max users, max queries/mes, etc.)

**Dependencias**: 2.4 (permisos) para que los limits funcionen.

### 3.4 Onboarding wizard

**Por qué**: signup tira al user a un workspace vacío. UX típica en
SaaS B2B: walkthrough de 3-5 pasos que muestra qué se puede hacer.

**Alcance** (~1 sesión):
- Componente `<Onboarding />` con steps tipo react-joyride
- Detección first-login via `users.last_login_at`
- Crear sample dashboard en Metabase y sample stream en OO al signup

**Dependencias**: ninguna.

---

## Tier 4 — Evoluciones de la plataforma extension

### 4.1 SDK del shell publicado en npm

**Por qué**: hoy el SDK lo importan via `<script src=einar.exe.xyz/sdk/v1.js>`.
Funciona pero atado al script-tag global. Para apps con build, mejor
`npm install @einar/sdk`.

**Alcance** (~1 sesión):
- Repackaging a TS module exportable
- Publish flow (manual o vía Changesets)
- Versioning semántico

**Dependencias**: ninguna.

### 4.2 Webhook bus

**Por qué**: cuando una embedded app necesita reaccionar a eventos del
tenant (user added, billing changed, etc.), hoy no hay forma.

**Alcance** (~2 sesiones):
- Migración: `webhooks(tenant_id, event_type, url, secret)`
- Endpoint `POST /api/webhooks` para registrar
- Worker que dispatchea con retries + signed payload
- UI para gestionar

**Dependencias**: 3.3 (billing tiene los primeros eventos interesantes).

### 4.3 Marketplace de embedded apps

**Por qué**: que terceros puedan publicar apps que cualquier tenant
instala con un click.

**Alcance**: HUGE (~10+ sesiones). Solo si el producto despega y los
devs externos quieren contribuir.

**Dependencias**: todo lo anterior.

---

## Cosas pequeñas que vale la pena no olvidar

- [ ] Health check endpoint `/health` que verifique DB + Casdoor + OO + Metabase
- [ ] Logout debería invalidar la sesión también en Casdoor (hoy solo
      borra cookies einar)
- [ ] El default port de exe.dev (`8000`) no es ideal — confunde con
      Casdoor. Considerar cambiar a otro número
- [ ] El SDK postMessage tiene `postMessage(..., '*')` en algunos
      lugares del iframe → restringir a `shellOrigin` siempre
- [ ] Internacionalización (todo en español/inglés mezclado)
- [ ] README está desactualizado respecto a Metabase y al flow nuevo
- [ ] Considerar mover `internal/adapter/out/metabase` etc. a un
      naming más pegado a "integrations" para distinguir de
      "infrastructure adapters"

---

## Decisiones registradas (ADRs implícitas)

Si en el futuro alguien pregunta "¿por qué hicimos X así?", acá las
respuestas:

1. **BFF + cookie HttpOnly en lugar de localStorage tokens**: IETF draft
   "OAuth 2.0 for Browser-Based Apps" lo recomienda explícitamente.
   XSS en el shell sería catastrófico con tokens en JS.
2. **TanStack Router en lugar de React Router**: type-safety end-to-end,
   loaders integrados con TanStack Query, prefetch en hover.
3. **Metabase en lugar de Redash**: CSP/XFO configurables, soporta
   embedding nativamente, multi-tenant via groups + collections en OSS.
4. **Casdoor org per tenant en lugar de Casdoor groups**: aprovisiona
   un namespace de identidad real, futureproof si decidís dar a cada
   tenant sus propios providers OAuth.
5. **OpenObserve org per tenant**: aislamiento total de logs/metrics,
   no hay riesgo de leak entre tenants.
6. **`einar.exe.xyz` en port 8000 público**: única opción en exe.dev
   sin custom domain. Caddy hace path routing interno.
7. **go:embed del SPA**: deploy de un solo binario, sin FS externos
   en producción.
