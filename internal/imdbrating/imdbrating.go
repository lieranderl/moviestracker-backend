package imdbrating

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"log"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	AWS_S3_REGION = "eu-west-2"
	AWS_S3_BUCKET = "imdb-store"
	AWS_KEY_FILE  = "title.ratings.tsv"
)

type fieldsReader struct {
	*csv.Reader
	fields []int
}

type imdbRating struct {
	Id     string
	Rating string
	Votes  string
}

func getRatingById(ratings []*imdbRating, imdbId string) *imdbRating {
	for _, r := range ratings {
		if r.Id == imdbId {
			return r
		}
	}
	return nil
}

func GetImdbRating(apiRequest events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	log.Println("Start GetImdbRating!")
	start := time.Now()

	accessKey := os.Getenv("AWSAccessKeyId")
	secretKey := os.Getenv("AWSSecretKey")
	client := s3.New(s3.Options{
		Region:      AWS_S3_REGION,
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	})

	o, err := client.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(AWS_S3_BUCKET),
		Key:    aws.String(AWS_KEY_FILE),
	})
	if err != nil {
		log.Fatal(err)
	}

	allrating := fetchImdbRatings(o.Body)
	rating := getRatingById(allrating, apiRequest.QueryStringParameters["imdb_id"])
	b, err := json.Marshal(rating)
	elapsed := time.Since(start)
	log.Printf("ALL took %s", elapsed)
	if err != nil {
		return events.APIGatewayProxyResponse{Body: err.Error(), StatusCode: 500}, nil
	}
	return events.APIGatewayProxyResponse{Body: string(b), StatusCode: 200}, nil
}

func newFieldsReader(r io.Reader, fields ...int) *fieldsReader {
	fr := &fieldsReader{
		Reader: csv.NewReader(r),
		fields: fields,
	}
	return fr
}

func (r *fieldsReader) read() (record *imdbRating, err error) {
	rec, err := r.Reader.Read()
	if err != nil {
		return nil, err
	}
	record = new(imdbRating)
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

func (r *fieldsReader) readAll() (records []*imdbRating, err error) {
loop:
	for {
		rec, err := r.read()
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

func fetchImdbRatings(f io.Reader) []*imdbRating {
	reader := bufio.NewReader(f)
	r := newFieldsReader(reader, 0, 1, 2)
	r.Comma = '\t'
	imdbRatings, err := r.readAll()
	if err != nil {
		log.Fatal(err)
	}
	return imdbRatings
}
