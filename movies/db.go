package movies

import (
	"context"
	"fmt"
	"log"

	"cloud.google.com/go/firestore"
)



func (m *Short)writeToDb(ctx context.Context, firestoreClient *firestore.Client) {
	moviesListRef := firestoreClient.Collection("latesttorrentsmovies")
	_, err := moviesListRef.Doc(fmt.Sprint(m.ID)).Set(ctx, m)
	if err != nil {
		log.Fatalln("error set data", err)
	}	
}