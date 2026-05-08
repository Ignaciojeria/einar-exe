FROM golang:1.25-alpine

RUN go install github.com/air-verse/air@latest

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download 2>/dev/null || true

COPY . .

EXPOSE 8080

CMD ["air", "-c", ".air.toml"]
