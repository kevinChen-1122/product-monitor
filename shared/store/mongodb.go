package store

import (
	"context"
	"product-monitor/shared/models"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoStore struct {
	client *mongo.Client
	Col    *mongo.Collection
}

func NewMongoStore(uri string, dbName string, collection string) *MongoStore {
	ctx := context.Background()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		panic(err) // 初始化連不上直接中斷
	}

	if dbName == "" {
		dbName = "product_monitor"
	}
	if collection == "" {
		collection = "products"
	}

	col := client.Database(dbName).Collection(collection)
	return &MongoStore{client: client, Col: col}
}

// Close 斷開 MongoDB 連線並釋放連線池
func (m *MongoStore) Close(ctx context.Context) error {
	return m.client.Disconnect(ctx)
}

// SaveProduct 儲存商品
func (m *MongoStore) SaveProduct(ctx context.Context, p models.Product) error {
	_, err := m.Col.InsertOne(ctx, p)
	return err
}

// SaveProducts 實作批次寫入
func (m *MongoStore) SaveProducts(ctx context.Context, products []models.Product) error {
	if len(products) == 0 {
		return nil
	}

	var docs []interface{}
	for _, p := range products {
		docs = append(docs, p)
	}

	_, err := m.Col.InsertMany(ctx, docs)
	return err
}
