package http

import (
	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(helloHandler)

func helloHandler(s *fuego.Server) {
	fuego.Get(s, "/hello", func(c fuego.ContextNoBody) (string, error) {
		return "Hello, World!", nil
	})
}
