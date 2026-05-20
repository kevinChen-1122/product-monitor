package main

import (
	"context"
	"testing"
	"time"

	"product-monitor/shared/models"
)

func TestCollectNotifyBatchSingleItem(t *testing.T) {
	first := models.Product{ID: "1"}
	batch := collectNotifyBatch(context.Background(), nil, first, 1, time.Hour)
	if len(batch) != 1 {
		t.Fatalf("len = %d, want 1", len(batch))
	}
}
