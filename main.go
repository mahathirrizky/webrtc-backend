package main

import (
	"context"
	"log"
	"text/template"
	"time"

	"os"
	"webrtc/controllers"
	"webrtc/handlers"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	indexTemplate = &template.Template{}
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}
	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		log.Fatal("Set your 'MONGODB_URI' environment variable. " +
			"See: " +
			"www.mongodb.com/docs/drivers/go/current/usage-examples/#environment-variable")
	}
	
	router := gin.Default()

	config := cors.Config{
		AllowOrigins:     []string{getenv("HOST_URL", "http://localhost")},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Length", "Content-Type"},
		AllowCredentials: true,
	}

	router.Use(cors.New(config))

	indexHTML, err := os.ReadFile("templates/index.html")
	if err != nil {
		log.Fatalf("Error reading index.html: %v", err)
	}
	indexTemplate = template.Must(template.New("").Parse(string(indexHTML)))


	client, err := mongo.Connect(context.TODO(), options.Client().
		ApplyURI(uri))
	if err != nil {
		panic(err)
	}

	defer func() {
		if err = client.Disconnect(context.TODO()); err != nil {
			log.Fatalf("Error disconnecting from MongoDB: %v", err)
		}
	}()



	router.Use(func(c *gin.Context) {
		c.Set("db", client)
		c.Next()
	})

	router.POST("/createuser", controllers.CreateUser)
	router.POST("/login", controllers.Login)
	router.POST("/session", controllers.CreateSession)
	router.POST("/sessionbyhost", controllers.GetSessionbyHost)
	router.GET("/connect", controllers.GetSession)
	router.POST("/connect/:url", controllers.ConnectSession)

	router.GET("/", func(c *gin.Context) {
		err := indexTemplate.Execute(c.Writer, nil)
		if err != nil {
			log.Fatalf("Error executing template: %v", err)
		}
	})

	router.GET("/websocket/:roomId", func(c *gin.Context) {
		roomId := c.Param("roomId")
		handlers.WebsocketHandler(c.Writer, c.Request, roomId)
	})

	go func() {
		for range time.NewTicker(time.Second * 3).C {
			handlers.DispatchKeyFrame()
		}
	}()

	if err := router.Run("0.0.0.0:" + getenv("PORT", "9000")); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return fallback
	}
	return value
}
