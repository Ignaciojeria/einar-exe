package main

import (
   "fmt"
   "net/http"
)

func helloHandler(w http.ResponseWriter, r *http.Request) {
   fmt.Fprintf(w, "Hello, World updated!\n")
}

func main() {
   http.HandleFunc("/hello", helloHandler)
   fmt.Println("Server running at http://localhost:8080")
   err := http.ListenAndServe(":8080", nil)
   if err != nil {
       fmt.Println("Error starting server:", err)
   }
}