package service

import (
	"fmt"
	"testing"

	"github.com/gdotgordon/ipverify/types"
	"github.com/google/uuid"
)

func TestStore(t *testing.T) {
	store, err := NewSQLiteStore(":memory:", newTestLogger(t))
	if err != nil {
		t.Errorf("creating db: %v", err)
	}

	for _, v := range []types.VerifyRequest{
		makeRecord("robby", 1514851999, "128.148.252.133"),
		makeRecord("fred", 1514851200, "128.148.252.151"),
		makeRecord("robby", 1514851200, "128.148.252.151"),
		makeRecord("steve", 1514851200, "128.148.252.151"),
		makeRecord("robby", 1514851299, "128.148.252.151"),
		makeRecord("robby", 1514851233, "128.148.252.133"),
		makeRecord("blaise", 1514851230, "128.148.252.151"),
		makeRecord("robby", 1514851230, "128.148.252.151"),
	} {
		err = store.AddRecord(v)
		if err != nil {
			t.Errorf("adding row: %v", err)
		}
	}

	items, err := store.GetAllRows()
	if err != nil {
		t.Errorf("getting rows: %v", err)
	}
	fmt.Println("items: ", items)

	prev, nxt, err := store.GetPriorNext("robby", uuid.New().String(), 1514851233)
	if err != nil {
		t.Errorf("getting rows: %v", err)
	}
	fmt.Println("prev:", prev, "next", nxt)
}
