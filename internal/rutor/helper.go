package rutor

import (
	"strings"
	"time"

	"github.com/goodsign/monday"
)

func isRussianOnly(text string) bool {
	return !strings.Contains(text, "/")
}

var fullMonth = map[string]string{
	"Янв": "Января",
	"Фев": "Февраля",
	"Мар": "Марта",
	"Апр": "Апреля",
	"Май": "Мая",
	"Июн": "Июня",
	"Июл": "Июля",
	"Авг": "Августа",
	"Сен": "Сентября",
	"Окт": "Октября",
	"Ноя": "Ноября",
	"Дек": "Декабря",
}

func parseDate(text string) string {
	stringDate := ""
	dateList := strings.Fields(text)
	for k, v := range fullMonth {
		if dateList[1] == k {
			if strings.HasPrefix(dateList[0], "0") {
				dateList[0] = strings.Replace(dateList[0], "0", "", 1)
			}
			stringDate = dateList[0] + " " + v + " 20" + dateList[2]
			break
		}
	}
	ts, _ := monday.ParseInLocation("2 January 2006", stringDate, time.Now().UTC().Location(), monday.LocaleRuRU)

	return ts.Format("2006-01-02T15:04:05.000Z")
}
