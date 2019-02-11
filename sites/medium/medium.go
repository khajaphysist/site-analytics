package medium

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
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

func getPublishedTime(doc *goquery.Document) (time.Time, error) {
	ret := ""
	doc.Find("head meta[property=article\\:published_time]").Each(func(i int, s *goquery.Selection) {
		val, exists := s.Attr("content")
		if exists {
			ret = val
		}
	})
	if ret == "" {
		return time.Time{}, errors.New("Published Time Not Found")
	}
	pt, err := time.Parse(time.RFC3339, ret)
	if err != nil {
		return time.Time{}, errors.New("Parsing Failed: " + ret)
	}
	return pt, nil
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

// GetArticle gets full medium article given url
func GetArticle(url string) (*Article, error) {
	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, errors.New("status code error: " + res.Status)
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	defer res.Body.Close()
	if err != nil {
		return nil, err
	}

	original, err := doc.Html()
	if err != nil {
		return nil, err
	}

	title, err := getTitle(doc)
	if err != nil {
		return nil, err
	}

	author, err := getAuthor(doc)
	if err != nil {
		return nil, err
	}

	text, err := getText(doc)
	if err != nil {
		return nil, err
	}

	publishedTime, err := getPublishedTime(doc)
	if err != nil {
		return nil, err
	}

	nlikes, ncomments := getLikesComments(doc)

	tags := getTags(doc)
	return &Article{
		URL:           url,
		Title:         title,
		Author:        author,
		Text:          text,
		PublishedTime: publishedTime,
		Tags:          tags,
		Nlikes:        nlikes,
		Ncomments:     ncomments,
		Original:      original,
	}, nil
}

// GetArticleBriefsFromArchiveURL gets all article urls form archivepage
func GetArticleBriefsFromArchiveURL(url string) []ArticleBrief {
	res, err := http.Get(url)
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

	ret := []ArticleBrief{}
	doc.Find(".streamItem .postArticle").Each(func(i int, s *goquery.Selection) {
		art := ArticleBrief{}

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
				if t, err := time.Parse(time.RFC3339, val); err == nil {
					art.PublishedTime = t
				}
			}
		})

		s.Find("div.js-actionMultirecommend").Each(func(j int, ss *goquery.Selection) {
			if ss.Text() != "" {
				val, err := getNumberFromString(ss.Text())
				if err != nil {
					log.Println(err, url)
				}
				art.Nlikes = val
			}
		})

		s.Find("div.u-floatRight").Each(func(j int, ss *goquery.Selection) {
			if ss.Text() != "" {
				val, err := getNumberFromString(strings.Split(ss.Text(), " ")[0])
				if err != nil {
					log.Println(err, url)
				}
				art.Ncomments = val
			}
		})

		ret = append(ret, art)
	})
	return ret
}

// Article is
type Article struct {
	URL           string    `json:"url"`
	Title         string    `json:"title"`
	Author        string    `json:"author"`
	Text          string    `json:"text"`
	Nlikes        int       `json:"nlikes"`
	Ncomments     int       `json:"ncomments"`
	Tags          []string  `json:"tags"`
	PublishedTime time.Time `json:"publishedTime"`
	Original      string    `json:"original"`
}

// ArticleBrief is
type ArticleBrief struct {
	URL           string    `json:"url"`
	Title         string    `json:"title"`
	Author        string    `json:"author"`
	Nlikes        int       `json:"nlikes"`
	Ncomments     int       `json:"ncomments"`
	PublishedTime time.Time `json:"publishedTime"`
}
