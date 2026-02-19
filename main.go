package main

import (
	"fmt"
	"log"
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

	// Inicializar banco de dados
	db, err := repository.InitDB("./migrator.db")
	if err != nil {
		log.Fatal("Erro ao inicializar BD:", err)
	}
	defer db.Close()

	fmt.Println("✅ Banco de dados inicializado!")

	// Setup router
	router := api.SetupRouter(db, asanaToken, clickUpToken)

	// Iniciar servidor HTTP
	fmt.Println("🚀 Servidor rodando em http://localhost:8080")
	fmt.Println("📝 Endpoints disponíveis:")
	fmt.Println("   POST /migrations - Criar migração")
	fmt.Println("   GET /migrations/:id - Ver status")
	fmt.Println("   GET /migrations - Listar todas")

	if err := http.ListenAndServe(":8080", router); err != nil {
		log.Fatal("Erro ao iniciar servidor:", err)
	}
}
