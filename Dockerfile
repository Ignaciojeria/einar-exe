FROM golang:1.25-alpine

# Tooling
#   - air: hot reload en dev
#   - nodejs/npm: build del SPA (web/) que se embebe via go:embed.
#                 Sin esto el binario no compila (internal/web/dist/ vacío).
RUN apk add --no-cache nodejs npm \
 && go install github.com/air-verse/air@latest

WORKDIR /app

# Layer-cache primero para deps de Go.
COPY go.mod go.sum* ./
RUN go mod download 2>/dev/null || true

# Layer-cache deps de npm (independiente del código).
COPY web/package.json web/package-lock.json* ./web/
RUN cd web && npm install --no-audit --no-fund --silent

# Resto del código.
COPY . .

# Build del SPA. air no toca esto, pero un primer arranque sin dist/
# rompería go:embed. Si trabajás el frontend con HMR, corré aparte
# `npm run build -- --watch` o `npm run dev` (proxy a localhost:8000).
RUN cd web && npm run build --silent

EXPOSE 8080

CMD ["air", "-c", ".air.toml"]
