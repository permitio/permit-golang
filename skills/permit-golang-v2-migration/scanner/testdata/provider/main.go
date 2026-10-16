// Command provider starts a sample shaped like a Terraform provider that uses v1 of the SDK. It
// imports no v1 package itself: the scanner must report nothing here.
package main

import (
	"context"
	"log"
	"os"
	"time"

	"example.com/provider/internal/provider"
	"example.com/provider/internal/resources"
)

func main() {
	ctx := context.Background()
	client, err := provider.Configure(ctx, provider.Settings{
		APIKey:  os.Getenv("PERMIT_API_KEY"),
		Timeout: 30 * time.Second,
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := resources.NewResource(client).Delete(ctx, "document"); err != nil {
		log.Fatal(err)
	}
}
