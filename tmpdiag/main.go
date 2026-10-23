package main

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type hello struct {
	SetName        string `bson:"setName"`
	Message        string `bson:"msg"`
	MaxWireVersion int32  `bson:"maxWireVersion"`
	Version        string `bson:"version"`
}

func try(uri, label string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		fmt.Printf("[%s] connect error: %v\n", label, err)
		return false
	}
	defer client.Disconnect(context.Background())
	if err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "ping", Value: 1}}).Err(); err != nil {
		fmt.Printf("[%s] ping: %v\n", label, err)
		return false
	}
	var h hello
	_ = client.Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&h)
	names, err := client.Database("huazhu").ListCollectionNames(ctx, bson.D{})
	if err != nil {
		fmt.Printf("[%s] auth ping OK, listCollections: %v\n", label, err)
		return false
	}
	fmt.Printf("[%s] OK version=%s setName=%q msg=%q wire=%d collections=%v\n",
		label, h.Version, h.SetName, h.Message, h.MaxWireVersion, names)
	return true
}

func main() {
	host := "101.96.204.17:27017"
	candidates := []struct {
		uri   string
		label string
	}{
		{fmt.Sprintf("mongodb://root:root@%s/huazhu?authSource=admin", host), "root/root@admin"},
		{fmt.Sprintf("mongodb://root:root@%s/huazhu?authSource=huazhu", host), "root/root@huazhu"},
		{fmt.Sprintf("mongodb://root:root@%s/huazhu", host), "root/root@default"},
		{fmt.Sprintf("mongodb://admin:admin@%s/huazhu?authSource=admin", host), "admin/admin@admin"},
	}
	for _, c := range candidates {
		if try(c.uri, c.label) {
			fmt.Println("WORKING_URI=" + c.uri)
			return
		}
	}
	fmt.Println("no candidate credentials worked")
}
