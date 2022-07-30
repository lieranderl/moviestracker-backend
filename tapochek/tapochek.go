package tapochek

import (
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"

	"github.com/gocolly/colly"
)


func HttpClient() (*http.Client) {
	var jar *cookiejar.Jar
	jar, _ = cookiejar.New(&cookiejar.Options{})
	return &http.Client{Jar: jar,}
}

func verifyLogin(c *colly.Collector, login, pass string) bool {
	httpClient := HttpClient()
	_, err := httpClient.PostForm("https://tapochek.net/login.php", url.Values{"login_username": {login}, "login_password": {pass}, "login": {"Вход"}})
	if err != nil {
		log.Println(err)
		return false
	}
	u, _ := url.Parse("https://tapochek.net")
	for _, j := range httpClient.Jar.Cookies(u) {
		if j.Name == "bb_data" {
			if err != c.SetCookies("https://tapochek.net", httpClient.Jar.Cookies(u)) {
				return false
			}
			// return err == c.SetCookies("https://tapochek.net", httpClient.Jar.Cookies(u))
			return true
		} else {
			coo := &http.Cookie{
				Name: "bb_data", 
				Value: "a%3A3%3A%7Bs%3A2%3A%22uk%22%3Bs%3A12%3A%220PLv1Vf4MJhm%22%3Bs%3A3%3A%22uid%22%3Bi%3A19320%3Bs%3A3%3A%22sid%22%3Bs%3A20%3A%221BazNVBL572GaZfFlfoM%22%3B%7D",
				Domain: ".tapochek.net",
				Path:"/",
				Expires: time.Now().Add(time.Minute*525600),
				HttpOnly: true,
			}
			httpClient.Jar.SetCookies(u, []*http.Cookie{coo})
			c.SetCookies("https://tapochek.net", httpClient.Jar.Cookies(u))
			return true
		}
	}
	return false
}