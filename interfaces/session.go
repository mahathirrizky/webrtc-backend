package interfaces

// Session interface
type Sessionget struct {
	ID string `bson:"_id"`
	Host     string
	Title    string
	Password string
}
type Session struct {
	Host     string
	Title    string
	Password string
}