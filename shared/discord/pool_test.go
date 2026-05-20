package discord

import "testing"

func TestWebhookPoolRoundRobin(t *testing.T) {
	pool := NewWebhookPool([]string{"a", "b", "c"})
	if pool.Len() != 3 {
		t.Fatalf("len = %d, want 3", pool.Len())
	}

	want := []int{0, 1, 2, 0, 1}
	for i, idx := range want {
		gotIndex, gotURL := pool.Next()
		if gotIndex != idx {
			t.Fatalf("step %d: index = %d, want %d", i, gotIndex, idx)
		}
		if gotURL != pool.URLAt(idx) {
			t.Fatalf("step %d: url mismatch", i)
		}
	}
}
