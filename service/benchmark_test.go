package service

import (
	"bytes"
	"io/ioutil"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/gdotgordon/ipverify/store"
)

// The purpose of this benchmark is to evaluate the usefulness of
// various optimizations on the main verify() API, such as building
// an index on the timestamp in the database, caching IP lookups, etc.
// It repeatedly invokes the verify endpoint with randomly generated
// user names.

func BenchmarkIndex(b *testing.B) {
	tmpfile, err := ioutil.TempFile("", "index_bench")
	if err != nil {
		b.Fatalf("error creating temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	log := newNoopLogger()
	store, err := store.NewSQLiteStore(tmpfile.Name(), log)
	if err != nil {
		b.Fatalf("error creating db: %v", err)
	}
	srv, err := New("../mmdb/GeoLite2-City.mmdb", store, log)
	if err != nil {
		b.Fatalf("error creating service: %v", err)
	}

	userCount := 20
	users := make([]string, userCount)
	for i := 0; i < userCount; i++ {
		users[i] = makeName()
	}
	now := time.Now()
	rand.Seed(now.Unix())

	for i := 0; i < b.N; i++ {
		// Randomize the timestamps over the last 5 hours
		req := makeReq(users[rand.Int()%20], "128.148.252.151", (now.Unix() - int64(rand.Int()%(3600*5))))

		// Invoke the verify service
		_, err := srv.VerifyIP(req)
		if err != nil {
			b.Fatalf("error verifying request: %v", err)
		}
	}
}

func makeName() string {
	var buf bytes.Buffer
	buf.WriteByte("ABCDEFGHIJKLMNOPQRSTUVWXYZ"[rand.Int()%26])
	len := rand.Int()%10 + 2
	for i := 0; i < len; i++ {
		buf.WriteByte("abcdefghijklmnopqrstuvwxyz"[rand.Int()%26])
	}
	return buf.String()
}
