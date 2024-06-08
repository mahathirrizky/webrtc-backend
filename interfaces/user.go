package interfaces

// Session interface
type User struct {
	UserName string
	Email    string
	Password string
}

type Login struct {
	Email    string
	Password string
}