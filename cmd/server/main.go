package main

import (
	"context"
	"etcd-gateway/internal/api"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
)

var (
	logger     *zap.Logger
	etcdClient *clientv3.Client
)

func init() {
	var err error
	if os.Getenv("APP_ENV") == "production" {
		logger, err = zap.NewProduction()
	} else {
		logger, err = zap.NewDevelopment()
	}
	if err != nil {
		fmt.Printf("Cannot create zap logger: %v\n", err)
		os.Exit(1)
	}

	etcdClient, err = clientv3.New(clientv3.Config{
		Endpoints:   []string{"localhost:2379"},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		logger.Fatal("Cannot connect to etcd:", zap.Error(err))
	}
}

func main() {
	// Create a new router
	router := gin.New()

	// Middlewares
	router.Use(gin.Recovery())
	router.Use(gin.Logger())
	router.Use(ZapLoggingMiddleware(logger))

	if os.Getenv("APP_ENV") == "production" {
		router.Use(corsMiddlewareForProduction())
	} else {
		router.Use(corsMiddlewareForDevelopment())
	}

	setupRoutes(router, logger)

	srv := &http.Server{
		Addr:    ":8080",
		Handler: router,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("listen:", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal("Server forced to shutdown:", zap.Error(err))
	}

	logger.Info("Server exiting")
}

func setupRoutes(router *gin.Engine, logger *zap.Logger) {
	router.GET("/health", healthCheckHandler)
	router.GET("/api/keys", api.FetchKeysHandler(etcdClient))
	router.GET("/api/value/*key", api.FetchValueForKeyHandler(etcdClient, logger))

	if os.Getenv("APP_ENV") != "production" {
		router.GET("/", func(c *gin.Context) {
			c.String(http.StatusOK, "Development root endpoint.")
		})
	}

}

func healthCheckHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
	})
}

func corsMiddlewareForDevelopment() gin.HandlerFunc {
	return cors.New(cors.Config{
		AllowAllOrigins:  true,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Length", "Content-Type"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	})
}

func corsMiddlewareForProduction() gin.HandlerFunc {
	return cors.New(cors.Config{
		AllowOrigins:  []string{"https://example.com"},
		AllowMethods:  []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:  []string{"Origin", "Content-Length", "Content-Type"},
		ExposeHeaders: []string{"Content-Length"},
		MaxAge:        12 * time.Hour,
	})
}

func ZapLoggingMiddleware(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		t := time.Now()

		c.Next()

		latency := time.Since(t)
		logger.Info("Request completed",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Duration("latency", latency),
			zap.Int("status", c.Writer.Status()),
		)
	}
}
