package httpserver

import (
	"context"
	"fmt"
	"os"
	"syscall"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.RegisterAtEnd(startServer)

func startServer(s *fuego.Server, shutdowner ioc.Shutdowner) {
	go func() {
		err := s.Run()
		if err != nil {
			fmt.Printf("server error: %v\n", err)
			p, _ := os.FindProcess(os.Getpid())
			_ = p.Signal(syscall.SIGTERM)
		}
	}()

	shutdowner.RegisterShutdown(func() error {
		return s.Shutdown(context.Background())
	})
}
