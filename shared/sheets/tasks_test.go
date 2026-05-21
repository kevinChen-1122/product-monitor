package sheets

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseSearchTasksCSV(t *testing.T) {
	csv := "keyword,price_start,price_end\niphone,1000,20000\nps5,,\n"
	tasks, err := parseSearchTasksCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("got %d tasks", len(tasks))
	}
	if tasks[0].Keyword != "iphone" || tasks[0].PriceStart != 1000 || tasks[0].PriceEnd != 20000 {
		t.Fatalf("task0: %+v", tasks[0])
	}
	if tasks[1].Keyword != "ps5" || tasks[1].PriceStart != defaultPriceStart || tasks[1].PriceEnd != defaultPriceEnd {
		t.Fatalf("task1: %+v", tasks[1])
	}
}

func TestParseSearchTasksCSV_invalidRange(t *testing.T) {
	csv := "keyword,price_start,price_end\nbad,5000,1000\n"
	_, err := parseSearchTasksCSV(strings.NewReader(csv))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFetchSearchTasks_http(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("keyword,price_start,price_end\nmac,0,50000\n"))
	}))
	defer srv.Close()

	tasks, err := FetchSearchTasks(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Keyword != "mac" || tasks[0].PriceEnd != 50000 {
		t.Fatalf("got %+v", tasks)
	}
}
