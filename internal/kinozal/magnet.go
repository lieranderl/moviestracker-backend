package kinozal

import (
	"compress/gzip"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Cred struct {
	Login    string
	Password string
}

const KINOZALLOGINURL = "http://kinozal.tv/takelogin.php"

func Login(httpClient *http.Client, cred *Cred) (bool, *http.Client) {
	_, err := httpClient.Get(KINOZALLOGINURL)
	if err != nil {
		log.Fatalln("Can not login to kinozal.")
	}

	time.Sleep(1 * time.Second)
	_, err = httpClient.PostForm(KINOZALLOGINURL, url.Values{"username": {cred.Login}, "password": {cred.Password}, "wact": {"takerecover"}, "touser": {"1"}})
	if err != nil {
		log.Fatalln("Login attempt failed")
	}
	time.Sleep(1 * time.Second)
	u, _ := url.Parse(KINOZALLOGINURL)
	for _, j := range httpClient.Jar.Cookies(u) {
		if j.Name == "pass" {
			return true, httpClient
		}
	}
	log.Println("No Login")
	return false, nil
}

func GetMagnet(httpClient *http.Client, id string, mc chan map[string]string) {
	var magnet string
	bb, err := get(httpClient, "http://kinozal.tv/get_srv_details.php?id="+id+"&action=2")
	time.Sleep(1 * time.Second)
	if err != nil {
		log.Fatalln("failed to get data.")
	}
	if strings.Contains(string(bb), "хеш:") {
		list := strings.Fields(string(bb))
		for i, s := range list {
			if strings.Contains(string(s), "хеш:") {
				magnet = "magnet:?xt=urn:btih:" + strings.Split(list[i+1], "<")[0]
				break
			}
		}
	}
	mc <- map[string]string{id: magnet}
}

func get(httpClient *http.Client, url1 string) ([]byte, error) {
	headers := http.Header{}
	headers.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/14.1.1 Safari/605.1.15")
	headers.Set("Accept-Encoding", "gzip")
	urlp, err := url.Parse(url1)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	req := &http.Request{Header: headers, URL: urlp}
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	defer resp.Body.Close()

	// Check that the server actual sent compressed data
	var reader io.ReadCloser
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			log.Println("Gzip error", err)
			return nil, err
		}
		defer reader.Close()
	default:
		reader = resp.Body
	}
	bb, err := ioutil.ReadAll(reader)
	if err != nil {
		log.Println("Read error", err)
		return nil, err
	} else {
		return bb, nil
	}
}
