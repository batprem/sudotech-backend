package models

import "time"

type User struct {
	ID           int64     `db:"id"            json:"id"`
	Username     string    `db:"username"      json:"username"`
	PasswordHash string    `db:"password_hash" json:"-"`
	CreatedAt    time.Time `db:"created_at"    json:"created_at"`
}

type PublicUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

func (u User) Public() PublicUser {
	return PublicUser{ID: u.ID, Username: u.Username}
}

type RegisterInput struct {
	Username string `json:"username" binding:"required,min=3,max=32"`
	Password string `json:"password" binding:"required,min=8,max=128"`
}

type LoginInput struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type AuthResponse struct {
	Token string     `json:"token"`
	User  PublicUser `json:"user"`
}
