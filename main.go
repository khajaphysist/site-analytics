package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

func getTitle(doc *goquery.Document) (string, error) {
	ret := ""
	doc.Find("title").Each(func(i int, s *goquery.Selection) {
		ret = s.Text()
	})
	if ret == "" {
		return "", errors.New("Title Not Found")
	}
	return ret, nil
}

func getAuthor(doc *goquery.Document) (string, error) {
	ret := ""
	doc.Find("head link[rel=author]").Each(func(i int, s *goquery.Selection) {
		val, exists := s.Attr("href")
		if exists {
			ret = val
		}
	})
	if ret == "" {
		return "", errors.New("Author Not Found")
	}
	idx := strings.IndexByte(ret, '@')
	if idx == -1 {
		return "", errors.New("Author Name not found: " + ret)
	}
	ret = ret[idx+1:]
	return ret, nil
}

func getText(doc *goquery.Document) (string, error) {
	ret := ""
	doc.Find(".section, .section--body").Find(".section-content .section-inner").Children().FilterFunction(func(i int, s *goquery.Selection) bool {
		return !(s.Is("figure") || s.Is("div") || s.Is("pre"))
	}).Each(func(i int, s *goquery.Selection) {
		ret += s.Text() + "\n"
	})

	if ret == "" {
		return "", errors.New("Body Not Found")
	}
	return ret, nil
}

func getPublishedTime(doc *goquery.Document) (string, error) {
	ret := ""
	doc.Find("head meta[property=article\\:published_time]").Each(func(i int, s *goquery.Selection) {
		val, exists := s.Attr("content")
		if exists {
			ret = val
		}
	})
	if ret == "" {
		return "", errors.New("Published Time Not Found")
	}
	return ret, nil
}

func getLikesComments(doc *goquery.Document) (int, int) {
	nlikes, ncomments := 0, 0
	doc.Find(".postActions").Each(func(i int, s *goquery.Selection) {
		ss := strings.Split(s.Text(), " claps")
		if len(ss) > 0 {
			val, err := getNumberFromString(ss[0])
			if err != nil {
				log.Println(err, ss)
			}
			nlikes = val
		}
		if len(ss) > 1 {
			val, err := getNumberFromString(ss[1])
			if err != nil {
				log.Println(err, ss)
			}
			ncomments = val
		}
	})
	return nlikes, ncomments
}

func getTags(doc *goquery.Document) []string {
	var ret []string
	doc.Find("ul.tags > li").Each(func(i int, s *goquery.Selection) {
		ret = append(ret, s.Text())
	})
	return ret
}

// Article is
type Article struct {
	URL           string   `json:"url"`
	Title         string   `json:"title"`
	Author        string   `json:"author"`
	Text          string   `json:"text"`
	Nlikes        int      `json:"nlikes"`
	Ncomments     int      `json:"ncomments"`
	Tags          []string `json:"tags"`
	PublishedTime string   `json:"publishedTime"`
}

// ArticleMap is safe datastructure
type ArticleMap struct {
	Data map[string]Article
	mux  sync.Mutex
}

func (m *ArticleMap) set(key string, value Article) {
	m.mux.Lock()
	defer m.mux.Unlock()
	m.Data[key] = value
}

func getNumberFromString(s string) (int, error) {
	s = strings.ToLower(s)
	if strings.Contains(s, "k") {
		s = strings.TrimSuffix(s, "k")
		if strings.Contains(s, ".") {
			ret, err := strconv.ParseFloat(s, 32)
			if err != nil {
				return 0, err
			}
			return int(ret) * 1000, nil
		}
		ret, err := strconv.Atoi(s)
		if err != nil {
			return 0, err
		}
		return ret * 1000, nil
	}
	ret, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	return ret, nil
}

func getArticle(url string) (Article, error) {
	res, err := http.Get(url)
	if err != nil {
		return Article{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return Article{}, errors.New("status code error: " + res.Status)
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	defer res.Body.Close()
	if err != nil {
		return Article{}, err
	}

	title, err := getTitle(doc)
	if err != nil {
		return Article{}, err
	}

	author, err := getAuthor(doc)
	if err != nil {
		return Article{}, err
	}

	text, err := getText(doc)
	if err != nil {
		return Article{}, err
	}

	publishedTime, err := getPublishedTime(doc)
	if err != nil {
		return Article{}, err
	}

	nlikes, ncomments := getLikesComments(doc)

	tags := getTags(doc)
	return Article{
		URL:           url,
		Title:         title,
		Author:        author,
		Text:          text,
		PublishedTime: publishedTime,
		Tags:          tags,
		Nlikes:        nlikes,
		Ncomments:     ncomments,
	}, nil
}

func saveArticle(art []Article, fileName string) error {
	b, err := json.Marshal(art)
	if err != nil {
		return err
	}
	e := ioutil.WriteFile(fileName+".json", b, 0644)
	if e != nil {
		return err
	}
	return nil
}

func getAndSaveArticle(url string, index int, wg *sync.WaitGroup) error {
	defer wg.Done()
	article, err := getArticle(url)
	if err != nil {
		return err
	}
	e := saveArticle([]Article{article}, strconv.Itoa(index))
	if e != nil {
		return err
	}
	return nil
}

func saveArticles(urls []Article) {
	var wg sync.WaitGroup
	for i, url := range urls {
		wg.Add(1)
		go getAndSaveArticle(url.URL, i, &wg)
	}
	wg.Wait()
}

func getUrls(dayURL string) []Article {
	res, err := http.Get(dayURL)
	if err != nil {
		log.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		log.Fatalf("status code error: %d %s", res.StatusCode, res.Status)
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		log.Fatal(err)
	}

	ret := []Article{}
	doc.Find(".streamItem .postArticle").Each(func(i int, s *goquery.Selection) {
		art := Article{}

		if s.Children().Is("a") {
			return
		}

		s.Find("h3").Each(func(j int, ss *goquery.Selection) {
			art.Title = ss.Text()
		})

		if art.Title == "" {
			return
		}

		s.Find(".postArticle-readMore a").Each(func(j int, ss *goquery.Selection) {
			if val, exists := ss.Attr("href"); exists {
				art.URL = strings.Split(val, "?")[0]
			}
		})

		s.Find("a.avatar").Each(func(j int, ss *goquery.Selection) {
			if val, exists := ss.Attr("href"); exists {
				if strs := strings.Split(val, "@"); len(strs) > 1 {
					art.Author = strs[1]
				} else {
					art.Author = strs[0]
				}

			}
		})

		s.Find("time").Each(func(j int, ss *goquery.Selection) {
			if val, exists := ss.Attr("datetime"); exists {
				art.PublishedTime = val
			}
		})

		s.Find("div.js-actionMultirecommend").Each(func(j int, ss *goquery.Selection) {
			if ss.Text() != "" {
				val, err := getNumberFromString(ss.Text())
				if err != nil {
					log.Println(err, dayURL)
				}
				art.Nlikes = val
			}
		})

		s.Find("div.u-floatRight").Each(func(j int, ss *goquery.Selection) {
			if ss.Text() != "" {
				val, err := getNumberFromString(strings.Split(ss.Text(), " ")[0])
				if err != nil {
					log.Println(err, dayURL)
				}
				art.Ncomments = val
			}
		})

		ret = append(ret, art)
	})
	return ret
}

func printUrls(urls []Article) {
	for _, url := range urls {
		fmt.Println(url.PublishedTime)
	}
}

func gatherUrlsForDay(t time.Time, tag string, wg *sync.WaitGroup, am *ArticleMap, gaurd *chan bool) {
	defer wg.Done()
	y, m, d := t.Date()
	monthStr := ""
	dayStr := ""
	if month := int(m); month < 10 {
		monthStr = "0" + strconv.Itoa(month)
	} else {
		monthStr = strconv.Itoa(month)
	}
	if d < 10 {
		dayStr = "0" + strconv.Itoa(d)
	} else {
		dayStr = strconv.Itoa(d)
	}
	dateStr := strconv.Itoa(y) + "/" + monthStr + "/" + dayStr
	url := "https://medium.com/tag/" + tag + "/archive/" + dateStr
	urls := getUrls(url)
	for _, val := range urls {
		am.set(val.URL, val)
	}
	err := saveToEls(urls)
	if err == nil {
		fmt.Println(url)
	}
	<-*gaurd
}

func gatherUrls(startDate, endDate time.Time, tag string) []Article {
	var wg sync.WaitGroup
	maxConcurrent := 40
	garud := make(chan bool, maxConcurrent)

	var am ArticleMap
	am.Data = make(map[string]Article)
	for startDate.After(endDate) {
		wg.Add(1)
		garud <- true
		go gatherUrlsForDay(startDate, tag, &wg, &am, &garud)
		time.Sleep(100000)
		startDate = startDate.AddDate(0, 0, -1)
	}
	wg.Wait()
	ret := make([]Article, len(am.Data))
	idx := 0
	for _, val := range am.Data {
		ret[idx] = val
		idx++
	}
	return ret
}

func saveToEls(articles []Article) error {
	elsURL := "http://178.128.126.80:9200/articles/_doc/"
	var wg sync.WaitGroup
	maxConcurrent := 20
	garud := make(chan bool, maxConcurrent)
	for _, v := range articles {
		wg.Add(1)
		garud <- true
		go func(art Article) {
			jsVal, _ := json.Marshal(art)
			resp, err := http.Post(elsURL+art.Author+"-"+art.PublishedTime, "application/json", bytes.NewBuffer(jsVal))
			if err != nil {
				log.Fatal(err, resp)
			}
			defer resp.Body.Close()
			wg.Done()
			<-garud
		}(v)
		time.Sleep(10000)
	}
	wg.Wait()
	return nil
}

func main() {
	startDate, _ := time.Parse(time.RFC3339, "2009-11-31T05:41:02.000Z")
	endDate, _ := time.Parse(time.RFC3339, "2009-01-23T05:40:02.000Z")

	urls := gatherUrls(startDate, endDate, "javascript")
	saveArticle(urls, "urls")
}
