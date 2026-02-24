package main

import (
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/TWRT/integration-mapper/internal/api"
	"github.com/TWRT/integration-mapper/internal/repository"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Erro ao carregar .env")
	}

	asanaToken := os.Getenv("ASANA_TOKEN")
	clickUpToken := os.Getenv("CLICKUP_TOKEN")
	if asanaToken == "" || clickUpToken == "" {
		log.Fatal("Tokens não configurados")
	}

	db, err := repository.InitDB("./migrator.db")
	if err != nil {
		log.Fatal("Erro ao inicializar BD:", err)
	}
	defer db.Close()

	slog.Info("database initialized")

	router := api.SetupRouter(db, asanaToken, clickUpToken)

	if err := http.ListenAndServe(":8080", router); err != nil {
		log.Fatal("Erro ao iniciar servidor:", err)
	}
}
