package controllers

import (
	"net/http"
	"webrtc/interfaces"
	"webrtc/utils"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// CreateSession - Creates user session
func CreateSession(ctx *gin.Context) {

	db := ctx.MustGet("db").(*mongo.Client)
	collection := db.Database("MeetKobi").Collection("sessions")

	var session interfaces.Session

	if err := ctx.ShouldBindJSON(&session); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	session.Password = utils.HashPassword(session.Password)

	result, err := collection.InsertOne(ctx, session)
	if err != nil {
  ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
  return
	}

	insertedID, ok := result.InsertedID.(primitive.ObjectID)
	if !ok {
  // Handle unexpected insertion result type
  ctx.JSON(http.StatusInternalServerError, gin.H{"error": "unexpected insertion result"})
  return
	}

	url := CreateSocket(session, ctx, insertedID.Hex())
	ctx.JSON(http.StatusOK, gin.H{"socket": url})
}