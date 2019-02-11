package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

func scanUrls(url string) []string {
	res, err := http.Get(url)
	fmt.Println("accessed: " + url)
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
	foundUrls := []string{}
	doc.Find(".timebucket a").Each(func(i int, s *goquery.Selection) {
		val, exists := s.Attr("href")
		if exists {
			foundUrls = append(foundUrls, val)
		}
	})
	return foundUrls
}

func getAllUrlsForTag(tag string, maxConcurrent int) []string {
	url := "https://medium.com/tag/" + tag + "/archive"
	resultUrls := []string{}
	yearUrls := scanUrls(url)
	wg := &sync.WaitGroup{}
	mux := &sync.Mutex{}
	gaurd := make(chan bool, maxConcurrent)
	for _, val := range yearUrls {
		wg.Add(1)
		go func(url string) {
			gaurd <- true
			defer wg.Done()
			monthUrls := scanUrls(url)
			time.Sleep(100000000)
			<-gaurd
			if len(monthUrls) == 0 {
				mux.Lock()
				resultUrls = append(resultUrls, url)
				mux.Unlock()
			} else {
				for _, val := range monthUrls {
					wg.Add(1)
					go func(url string) {
						gaurd <- true
						defer wg.Done()
						dayUrls := scanUrls(url)
						time.Sleep(100000000)
						<-gaurd
						if len(dayUrls) == 0 {
							mux.Lock()
							resultUrls = append(resultUrls, url)
							mux.Unlock()
						} else {
							mux.Lock()
							resultUrls = append(resultUrls, dayUrls...)
							mux.Unlock()
						}
					}(val)
				}
			}
		}(val)
	}
	wg.Wait()
	return resultUrls
}

func gatherArchiveUrls() {
	tagFilePath, _ := filepath.Abs("tags.txt")
	doneTagFilePath, _ := filepath.Abs("done.txt")

	tagsContent, err := ioutil.ReadFile(tagFilePath)
	if err != nil {
		log.Fatal(err)
	}
	tags := strings.Split(string(tagsContent), "\n")

	doneTagsContent, err := ioutil.ReadFile(doneTagFilePath)
	if err != nil {
		log.Fatal(err)
	}
	doneTags := strings.Split(string(doneTagsContent), "\n")

	for index, tag := range tags {
		urls := getAllUrlsForTag(tag, 15)
		sort.Strings(urls)
		if err := ioutil.WriteFile("data/archiveUrls/"+tag+".txt", []byte(strings.Join(urls, "\n")), 0644); err != nil {
			log.Fatal(err)
		}
		doneTags = append(doneTags, tag)
		if err := ioutil.WriteFile(doneTagFilePath, []byte(strings.Join(doneTags, "\n")), 0644); err != nil {
			log.Fatal(err)
		}
		if err := ioutil.WriteFile(tagFilePath, []byte(strings.Join(tags[index+1:], "\n")), 0644); err != nil {
			log.Fatal(err)
		}
	}
}

func main() {
	files, err := ioutil.ReadDir("data/archiveUrls")
	if err != nil {
		log.Fatal(err)
	}
	fileNames := []string{}
	for _, fileDes := range files {
		if strings.Contains(fileDes.Name(), ".txt") {
			fileNames = append(fileNames, strings.Replace(fileDes.Name(), ".txt", "", -1))
		}
	}
	for _, tag := range fileNames {
		dirPath, err := filepath.Abs("data/archiveHtmls/" + tag)
		if err != nil {
			log.Fatal(err)
		}
		if err := os.MkdirAll(dirPath, os.ModePerm); err != nil {
			log.Fatal(err)
		}
		urlsContent, err := ioutil.ReadFile("data/archiveUrls/" + tag + ".txt")
		if err != nil {
			log.Fatal(err)
		}
		if string(urlsContent) == "" {
			continue
		}
		urls := strings.Split(string(urlsContent), "\n")

		wg := &sync.WaitGroup{}
		mux := &sync.Mutex{}
		gaurd := make(chan bool, 5)
		for index, url := range urls {
			wg.Add(1)
			gaurd <- true
			time.Sleep(100000000)
			go func(url string, index int) {
				htmlFileName := strings.Replace(strings.Replace(url, "https://medium.com/tag/"+tag+"/archive/", "", 1), "/", "-", -1) + ".html"
				resp, err := http.Get(url)
				fmt.Println("accessed: " + url)
				if err != nil {
					log.Fatal(err)
				}
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					log.Fatal(err)
				}
				mux.Lock()
				if err := ioutil.WriteFile("data/archiveHtmls/"+tag+"/"+htmlFileName, body, 0644); err != nil {
					log.Fatal(err)
				}
				if err := ioutil.WriteFile("data/archiveUrls/"+tag+".txt", []byte(strings.Join(urls[index+1:], "\n")), 0644); err != nil {
					log.Fatal(err)
				}
				mux.Unlock()
				wg.Done()
				<-gaurd
			}(url, index)
		}
		wg.Wait()
	}
}
