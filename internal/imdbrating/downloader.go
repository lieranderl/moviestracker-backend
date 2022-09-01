package imdbrating

import (
	"bytes"
	"compress/gzip"
	"context"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	URLBASE = "https://datasets.imdbws.com/"
 	RATINGSFILEARC = "title.ratings.tsv.gz"
)

func getImdbFile() *http.Response {
	url := URLBASE + RATINGSFILEARC
	start := time.Now()
	headResp, err := http.Head(url)
	if err != nil {
		panic(err)
	}

	defer headResp.Body.Close()

	size, err := strconv.Atoi(headResp.Header.Get("Content-Length"))
	if err != nil {
		log.Println(err)
	}
	sizeMb := float64(size) * 0.000001

	//Get data
	resp, err := http.Get(url)
	if err != nil {
		log.Println(err)
	}
	
	if resp.StatusCode == 200 {
		elapsed := time.Since(start)
		log.Printf("%0.3f MB downloaded completed in %s", sizeMb, elapsed)
		if err != nil {
			log.Println(err)
		}
		return resp
	}

	return nil
}

func DownloadImdbData() {   

	resp := getImdbFile()
	if resp == nil {
		log.Fatalln("Empty file")
	}

	archive, err := gzip.NewReader(resp.Body)
	if err != nil {
		log.Fatalln(err)
	}
	defer archive.Close()
	defer resp.Body.Close()
	var p []byte
	p, err = ioutil.ReadAll(archive)
	if err != nil {
		log.Fatalln(err)
	}

	accessKey:=os.Getenv("AWSAccessKeyId")
	secretKey:=os.Getenv("AWSSecretKey")
	client := s3.New(s3.Options{
		Region:      AWS_S3_REGION,
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	})

	_, err = client.PutObject(context.Background(), &s3.PutObjectInput{
        Bucket: aws.String(AWS_S3_BUCKET),
        Key:    aws.String("title.ratings.tsv"),
		Body:   bytes.NewBuffer(p),
	    })
    if err != nil {
        log.Fatal(err)
    }
}
