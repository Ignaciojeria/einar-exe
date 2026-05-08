package httpserver

import (
	"strings"

	"einar-exe/internal/shared/environment"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(New)

func New(env environment.Conf) *fuego.Server {
	// fuego.WithAddr exige formato "host:port". Aceptamos APP_PORT como
	// "8080" o ":8080" indistintamente y normalizamos. Esto blinda contra
	// el quirk de docker compose, que parsea ":8080" desde .env como YAML
	// y lo aplana a "8080" silenciosamente.
	addr := env.APP_PORT
	if !strings.Contains(addr, ":") {
		addr = ":" + addr
	}
	// Forzamos JSON como único formato de respuesta. fuego por default
	// hace content-negotiation y sirve XML cuando el browser manda
	// `Accept: application/xml;q=0.9` (lo que pasa al pegar la URL en
	// la barra). Para una API JSON pura, eso es ruido.
	return fuego.NewServer(
		fuego.WithAddr(addr),
		fuego.WithSerializer(fuego.SendJSON),
		fuego.WithErrorSerializer(fuego.SendJSONError),
	)
}
