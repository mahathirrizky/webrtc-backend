package controllers

import (
	"net/http"

	"webrtc/handlers"
	"webrtc/interfaces"
	"webrtc/utils"

	"github.com/gin-gonic/gin"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func CreateUser(ctx *gin.Context){
	db := ctx.MustGet("db").(*mongo.Client)
	collection := db.Database("MeetKobi").Collection("users")

	var user interfaces.User

	if err := ctx.ShouldBindJSON(&user); err != nil{
		ctx.JSON(http.StatusBadRequest, gin.H{"error":err.Error()})
		return
	}

	user.Password = utils.HashPassword(user.Password)

	result,err := collection.InsertOne(ctx, user)
	if err!= nil{
		ctx.JSON(http.StatusInternalServerError, gin.H{"error":err.Error()})
		return
	}
	token, err := handlers.GenerateToken(result.InsertedID.(primitive.ObjectID).Hex()) // Generate token from ObjectID
	if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
	}

	responseUser := gin.H{
		"username": user.UserName,
		"email":   user.Email,
}



	ctx.JSON(http.StatusOK, gin.H{"status": "success", "token": token, "user":responseUser})


}

func Login(ctx *gin.Context){
	db := ctx.MustGet("db").(*mongo.Client)
	collection := db.Database("MeetKobi").Collection("users")

	var login interfaces.Login

	if err := ctx.ShouldBindJSON(&login); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
	}

	filter := bson.M{"email": login.Email}

	// Find the user and get the ObjectID directly
	var result bson.M
	if err := collection.FindOne(ctx, filter).Decode(&result); err != nil {
			ctx.JSON(http.StatusUnauthorized, gin.H{"error": "user tidak ditemukan"})
			return
	}

	// Extract the ObjectID from the result
	userID := result["_id"].(primitive.ObjectID).Hex() 

	if !utils.ComparePasswords(result["password"].(string), []byte(login.Password)) {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid password."})
			return
	}

	token, err := handlers.GenerateToken(userID) // Generate token from ObjectID
	if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
	}
	responseUser := gin.H{
		"username": result["username"],
		"email":    result["email"],
}



	ctx.JSON(http.StatusOK, gin.H{"status": "success", "token": token, "user":responseUser})
}


func GetUserByID(ctx *gin.Context,  userID string) (interfaces.User, error) {
	db := ctx.MustGet("db").(*mongo.Client)
	collection := db.Database("MeetKobi").Collection("users")
	var user interfaces.User

	filter := bson.M{"_id": userID}

	err := collection.FindOne(ctx, filter).Decode(&user)
	if err != nil {
		return user, err
	}

	return user, nil
}