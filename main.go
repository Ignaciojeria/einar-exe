package main

import (
    "fmt"
    "log"
    "os"
    "os/signal"
    "syscall"

    "github.com/Ignaciojeria/ioc"
)

func main() {
    fmt.Println("Starting Einar Exe...")

    if err := ioc.LoadDependencies(); err != nil {
        log.Fatal(err)
    }
    // Wait for termination signal (e.g. Ctrl+C or Kubernetes SIGTERM)
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
    <-quit 
    
    // Execute graceful shutdown
    if err := ioc.Shutdown(); err != nil {
        log.Fatalf("Shutdown errors: %v", err)
    }
}