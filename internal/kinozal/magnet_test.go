package kinozal

import (
	"log"
	"net/http"
	"net/http/cookiejar"
	"os"
	"testing"

	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
)

func TestPagnet(t *testing.T) {
	err := godotenv.Load()
    if err != nil {
        log.Fatal("Error loading .env file")
    }
	cred := new(Cred)
	cred.Login = os.Getenv("KZ_LOGIN")
	cred.Password = os.Getenv("KZ_PASSWORD")

	var jar *cookiejar.Jar
	tbTransport := &http.Transport{
	}
	jar, _ = cookiejar.New(&cookiejar.Options{})
	httpcl := &http.Client{Jar: jar, Transport: tbTransport}
	
	l, httpcl := Login(httpcl, cred)
	if l {
		ch := make(chan map[string]string)
		go GetMagnet(httpcl, "1933366", ch)
		yy := <-ch
		close(ch)
		assert.Equal(t, "magnet:?xt=urn:btih:A14E679DD461CF6A3C70FAACAB2EEC95C66AA817", yy["1933366"])
	} else {
		t.Error()
	}
	

}