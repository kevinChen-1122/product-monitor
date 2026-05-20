package models

import "time"

type Product struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Price      string    `json:"price"`
	Link       string    `json:"link"`
	ImageURL   string    `json:"image_url" bson:"image_url"`
	SellerName string    `json:"seller_name"`
	CreatedAt  time.Time `json:"created_at" bson:"created_at"`
}
