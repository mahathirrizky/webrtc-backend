package controllers

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"
	"webrtc/interfaces"
	"webrtc/utils"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ConnectSession - Given a host and a password, returns the session object.
func ConnectSession(ctx *gin.Context) {
	db := ctx.MustGet("db").(*mongo.Client)
	collection := db.Database("MeetKobi").Collection("sockets")

	url := ctx.Param("url")
	result := collection.FindOne(ctx, bson.M{"hashedurl": url})

	var input interfaces.Session
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if result.Err() != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Socket connection not found."})
		return
	}

	var socket interfaces.Socket
	result.Decode(&socket)

	collection = db.Database("MeetKobi").Collection("sessions")
	objectID, err := primitive.ObjectIDFromHex(socket.SessionID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Session not found."})
		return
	}

	result = collection.FindOne(ctx, bson.M{"_id": objectID})
	if result.Err() != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Session not found."})
		return
	}

	var session interfaces.Session
	result.Decode(&session)

	if !utils.ComparePasswords(session.Password, []byte(input.Password)) {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid password."})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"title":  session.Title,
		"socket": socket.SocketURL,
	})
}

// GetSession - Checks if session exists.
func GetSession(ctx *gin.Context) {
	db := ctx.MustGet("db").(*mongo.Client)
	collection := db.Database("MeetKobi").Collection("sockets")

	id := ctx.Request.URL.Query()["url"][0]
	result := collection.FindOne(ctx, bson.M{"hashedurl": id})

	if result.Err() != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Socket connection not found."})
		return
	}

	ctx.Status(http.StatusOK)
}

func GetSessionbyHost(ctx *gin.Context) {
	db := ctx.MustGet("db").(*mongo.Client)
	sessionCollection := db.Database("MeetKobi").Collection("sessions")
	socketsCollection := db.Database("MeetKobi").Collection("sockets")

	var session interfaces.Sessionget
	if err := ctx.ShouldBindJSON(&session); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
	}

	// Define the options for sorting by _id field
	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: "_id", Value: -1}}) // Sorting by _id field in descending order

	cursor, err := sessionCollection.Find(ctx, bson.M{"host": session.Host}, findOptions)
	if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find sessions"})
			return
	}
	defer cursor.Close(ctx)

	var sessions []interfaces.Sessionget
	if err = cursor.All(ctx, &sessions); err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode sessions"})
			return
	}

	if len(sessions) == 0 {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "No sessions found for the given host"})
			return
	}

	var results []gin.H

	for _, sess := range sessions {
			var socketData interfaces.Socket
			err = socketsCollection.FindOne(ctx, bson.M{"sessionid": sess.ID}).Decode(&socketData)
			if err != nil {
					ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find socket data"})
					return
			}
			results = append(results, gin.H{
					"host":  sess.Host,
					"title": sess.Title,
					"coderoom":  socketData.HashedURL,
			})
	}

	ctx.JSON(http.StatusOK, gin.H{"sessions": results})
}



// CreateSocket - Creates socket connection with given session.
func CreateSocket(session interfaces.Session, ctx *gin.Context, id string) string {
	db := ctx.MustGet("db").(*mongo.Client)
	collection := db.Database("MeetKobi").Collection("sockets")
	now := time.Now()

	seconds := now.Unix()
	var socket interfaces.Socket
	hashURL := hashURL(session.Host + session.Title + fmt.Sprintf("%d", seconds))
	socketURL := hashSession(session.Host + session.Password)
	socket.SessionID = id
	socket.HashedURL = hashURL
	socket.SocketURL = socketURL

	collection.InsertOne(ctx, socket)

	return hashURL
}

func hashSession(str string) string {
	hash := sha1.New()
	hash.Write([]byte(str))
	return hex.EncodeToString(hash.Sum(nil))
}

func hashURL(str string) string {
	hash := sha1.New()
	hash.Write([]byte(str))
	hashSum := hash.Sum(nil)[:3] // Trim to first 3 bytes
	return hex.EncodeToString(hashSum)
}
