package main

import (
	"log"
	"os"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"todo-list/backend/auth"
	"todo-list/backend/db"
	"todo-list/backend/handlers"
)

func main() {
	conn, err := db.Open("todos.db")
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "dev-secret-change-me"
		log.Println("WARNING: JWT_SECRET not set; using insecure dev secret")
	}
	authSvc := auth.NewService(secret)

	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}))

	api := r.Group("/api")

	authH := &handlers.AuthHandler{DB: conn, Auth: authSvc}
	authH.Register(api.Group("/auth"))

	protected := api.Group("", authSvc.RequireUser())
	boardH := &handlers.BoardHandler{DB: conn}
	boardH.Register(protected)
	columnH := &handlers.ColumnHandler{DB: conn}
	columnH.Register(protected)

	if err := r.Run(":8080"); err != nil {
		log.Fatalf("run: %v", err)
	}
}
