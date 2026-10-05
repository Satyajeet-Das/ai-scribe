package main

import (
	"fmt"
	"os"

	"github.com/labstack/echo/v4"

	"github.com/Satyajeet-Das/ai-scribe/internal/answer"
	"github.com/Satyajeet-Das/ai-scribe/internal/assignment"
	"github.com/Satyajeet-Das/ai-scribe/internal/auth"
	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/http/handler"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/openapi"
	"github.com/Satyajeet-Das/ai-scribe/internal/question"
	"github.com/Satyajeet-Das/ai-scribe/internal/session"
	"github.com/Satyajeet-Das/ai-scribe/internal/user"
)

func main() {
	e := echo.New()

	noopMiddleware := func(next echo.HandlerFunc) echo.HandlerFunc {
		return next
	}

	// Register system routes
	healthH := handler.NewHealthHandler("dev", nil, nil, nil)
	e.GET("/status", healthH.CheckHealth)

	v1 := e.Group("/api/v1")

	// Handlers with nil services (only registering route definitions)
	authH := auth.NewHandler(nil, false)
	authH.RegisterRoutes(v1, noopMiddleware, noopMiddleware)

	userH := user.NewHandler(nil)
	userH.RegisterRoutes(v1, noopMiddleware, noopMiddleware)
	userH.RegisterRoutes(e.Group(""), noopMiddleware, noopMiddleware)

	examH := exam.NewHandler(nil)
	examH.RegisterRoutes(v1, noopMiddleware, noopMiddleware)

	questionH := question.NewHandler(nil)
	questionH.RegisterRoutes(v1, noopMiddleware)

	assignmentH := assignment.NewHandler(nil)
	assignmentH.RegisterRoutes(v1, noopMiddleware)

	sessionH := session.NewHandler(nil)
	sessionH.RegisterRoutes(v1, noopMiddleware)

	answerH := answer.NewHandler(nil)
	answerH.RegisterRoutes(v1, noopMiddleware)

	targetFile := "static/openapi.json"
	if _, err := os.Stat("static"); os.IsNotExist(err) {
		// If running from repo root
		targetFile = "apps/backend/static/openapi.json"
	}

	if err := openapi.SyncOpenAPISpec(e.Routes(), targetFile); err != nil {
		fmt.Printf("Error updating OpenAPI spec: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully updated %s from Go codebase!\n", targetFile)
}
