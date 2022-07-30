package imdbratingupdater

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/csv"

	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"cloud.google.com/go/firestore"
)


type FieldsReader struct {
	*csv.Reader
	fields []int
}

type ImdbRating struct {
	Id     string
	Rating string
	Votes  string
}

type MyImdbColl struct{
	*firestore.CollectionRef
}

const URLBASE = "https://datasets.imdbws.com/"
const RATINGSFILEARC = "title.ratings.tsv.gz"

func Imdbratingupdater(ctx context.Context, firestoreClient *firestore.Client ) {
	url := URLBASE + RATINGSFILEARC
	start := time.Now()
	headResp, err := http.Head(url)
	if err != nil {
		panic(err)
	}

	defer headResp.Body.Close()

	size, err := strconv.Atoi(headResp.Header.Get("Content-Length"))
	sizeMb := float64(size) * 0.000001
	elapsed := time.Since(start)
	log.Printf("%0.3f MB downloaded completed in %s", sizeMb, elapsed)
	if err != nil {
		log.Println(err)
	}
	//Get data
	resp, err := http.Get(url)
	if err != nil {
		log.Println(err)
	}
	defer resp.Body.Close()

	archive, err := gzip.NewReader(resp.Body)
	if err != nil {
		log.Println(err)
	}
	defer archive.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	imdbRef := new(MyImdbColl)
	imdbRef.CollectionRef = firestoreClient.Collection("imdbratings")
	
	for _, r :=range FetchImdbRatings(archive) {
		_, err := imdbRef.Doc(r.Id).Set(ctx, r)
	if err != nil {
		log.Println("Failed to write", r.Id)
	}	

	}

}

func (t *MyImdbColl)writeTodb(r *ImdbRating) ([]*ImdbRating, error){
	ctx := context.Background()
	t.Doc(r.Id).Set(ctx, r)
	return nil, nil
}


func NewFieldsReader(r io.Reader, fields ...int) *FieldsReader {
	fr := &FieldsReader{
		Reader: csv.NewReader(r),
		fields: fields,
	}
	return fr
}

func (r *FieldsReader) Read() (record *ImdbRating, err error) {
	rec, err := r.Reader.Read()
	if err != nil {
		return nil, err
	}
	record = new(ImdbRating)
	for i, f := range r.fields {
		if i == 0 {
			record.Id = rec[f]
		}
		if i == 1 {
			record.Rating = rec[f]
		}
		if i == 2 {
			record.Votes = rec[f]
		}

	}

	return record, nil
}

func (r *FieldsReader) ReadAll() (records []*ImdbRating, err error) {
loop:
	for {
		rec, err := r.Read()
		switch err {
		case io.EOF:
			break loop
		case nil:
			records = append(records, rec)
		default:
			return nil, err
		}
	}
	x, a := records[0], records[1:]
	a = append(a, x)
	return a, nil
}

func FetchImdbRatings(f io.Reader) []*ImdbRating {
	reader := bufio.NewReader(f)
	r := NewFieldsReader(reader, 0, 1, 2)
	r.Comma = '\t'
	imdbRatings, err := r.ReadAll()
	if err != nil {
		log.Fatal(err)
	}
	return imdbRatings
}
