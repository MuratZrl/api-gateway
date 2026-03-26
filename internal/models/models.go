package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Route struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Path      string             `bson:"path" json:"path"`
	Target    string             `bson:"target" json:"target"`
	Methods   []string           `bson:"methods" json:"methods"`
	Protected bool               `bson:"protected" json:"protected"`
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
}

type RequestLog struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Method     string             `bson:"method" json:"method"`
	Path       string             `bson:"path" json:"path"`
	StatusCode int                `bson:"status_code" json:"status_code"`
	Duration   int64              `bson:"duration_ms" json:"duration_ms"`
	ClientIP   string             `bson:"client_ip" json:"client_ip"`
	UserAgent  string             `bson:"user_agent" json:"user_agent"`
	CreatedAt  time.Time          `bson:"created_at" json:"created_at"`
}

type ApiKey struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Key       string             `bson:"key" json:"key"`
	Name      string             `bson:"name" json:"name"`
	Active    bool               `bson:"active" json:"active"`
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
}
