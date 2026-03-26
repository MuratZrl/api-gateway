package repository

import (
	"context"
	"time"

	"api-gateway/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoRepository struct {
	db *mongo.Database
}

func NewMongoRepository(uri, database string) (*MongoRepository, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}

	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}

	return &MongoRepository{db: client.Database(database)}, nil
}

// Request Logs

func (r *MongoRepository) InsertLog(ctx context.Context, log *models.RequestLog) error {
	log.CreatedAt = time.Now()
	_, err := r.db.Collection("request_logs").InsertOne(ctx, log)
	return err
}

func (r *MongoRepository) GetStats(ctx context.Context) ([]bson.M, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$path"},
			{Key: "total_requests", Value: bson.D{{Key: "$sum", Value: 1}}},
			{Key: "avg_duration_ms", Value: bson.D{{Key: "$avg", Value: "$duration_ms"}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "total_requests", Value: -1}}}},
	}

	cursor, err := r.db.Collection("request_logs").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []bson.M
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// Routes

func (r *MongoRepository) GetRoutes(ctx context.Context) ([]models.Route, error) {
	cursor, err := r.db.Collection("routes").Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var routes []models.Route
	if err := cursor.All(ctx, &routes); err != nil {
		return nil, err
	}
	return routes, nil
}

func (r *MongoRepository) InsertRoute(ctx context.Context, route *models.Route) error {
	route.CreatedAt = time.Now()
	result, err := r.db.Collection("routes").InsertOne(ctx, route)
	if err != nil {
		return err
	}
	route.ID = result.InsertedID.(primitive.ObjectID)
	return nil
}

func (r *MongoRepository) DeleteRoute(ctx context.Context, id string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	_, err = r.db.Collection("routes").DeleteOne(ctx, bson.M{"_id": objID})
	return err
}

// API Keys

func (r *MongoRepository) GetApiKeyByKey(ctx context.Context, key string) (*models.ApiKey, error) {
	var apiKey models.ApiKey
	err := r.db.Collection("api_keys").FindOne(ctx, bson.M{"key": key, "active": true}).Decode(&apiKey)
	if err != nil {
		return nil, err
	}
	return &apiKey, nil
}

func (r *MongoRepository) InsertApiKey(ctx context.Context, apiKey *models.ApiKey) error {
	apiKey.CreatedAt = time.Now()
	apiKey.Active = true
	result, err := r.db.Collection("api_keys").InsertOne(ctx, apiKey)
	if err != nil {
		return err
	}
	apiKey.ID = result.InsertedID.(primitive.ObjectID)
	return nil
}
