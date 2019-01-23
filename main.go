package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

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

func getLikesComments(doc *goquery.Document) (string, string) {
	nlikes, ncomments := "", ""
	doc.Find(".postActions").Each(func(i int, s *goquery.Selection) {
		ss := strings.Split(s.Text(), " likes")
		if len(ss) > 0 {
			nlikes = ss[0]
		}
		if len(ss) > 1 {
			ncomments = ss[1]
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

func main() {
	url := ""
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

	title, err := getTitle(doc)
	if err != nil {
		log.Fatal(err)
	}

	author, err := getAuthor(doc)
	if err != nil {
		log.Fatal(err)
	}

	text, err := getText(doc)
	if err != nil {
		log.Fatal(err)
	}

	publishedTime, err := getPublishedTime(doc)
	if err != nil {
		log.Fatal(err)
	}

	nlikes, ncomments := getLikesComments(doc)

	tags := getTags(doc)

	fmt.Println(title)
	fmt.Println(author)
	fmt.Println(text)
	fmt.Println(nlikes, " ", ncomments)
	fmt.Println(tags)
	fmt.Println(publishedTime)
}
