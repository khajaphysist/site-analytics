package main

import (
	"bytes"
	"encoding/json"
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

	"github.com/buger/jsonparser"

	"github.com/PuerkitoBio/goquery"
)

var httpClient = &http.Client{
	Timeout: time.Second * 10,
}

func scanUrls(url string) []string {
	res, err := httpClient.Get(url)
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

func getAllArticleMetaData() {
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
				resp, err := httpClient.Get(url)
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

// User holds medium author information
type User struct {
	UserID            string `json:"userId"`
	Username          string `json:"username"`
	Name              string `json:"name"`
	ImageID           string `json:"imageId"`
	Bio               string `json:"bio"`
	TwitterScreenName string `json:"twitterScreenName"`
	FacebookAccountID string `json:"facebookAccountId"`
	Type              string `json:"type"`
	dbID              string
}

// Tag holds medium tag information
type Tag struct {
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	PostCount int    `json:"postCount"`
}

func getUsers(str string) map[string]User {
	m := make(map[string]User)
	if val, _, _, err := jsonparser.Get([]byte(str), "references", "User"); err == nil {
		jsonparser.ObjectEach(val, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
			user := &User{}
			err := json.Unmarshal(value, user)
			if err != nil {
				fmt.Println(err)
			} else {
				m[user.UserID] = *user
			}
			return nil
		})
	}
	return m
}

// Post holds medium post information
type Post struct {
	ID           string `json:"id"`
	CreatorID    string `json:"creatorId"`
	Title        string `json:"title"`
	Slug         string `json:"uniqueSlug"`
	Language     string `json:"detectedLanguage"`
	PublishedAt  int64  `json:"firstPublishedAt"`
	claps        int64
	comments     int64
	wordCount    int64
	tags         []Tag
	previewImage string
	body         string
	Type         string `json:"type"`
	dbID         string
}

func getPosts(str string) map[string]Post {
	m := make(map[string]Post)
	if val, _, _, err := jsonparser.Get([]byte(str), "references", "Post"); err == nil {
		jsonparser.ObjectEach(val, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
			post := &Post{}
			err := json.Unmarshal(value, post)
			if err != nil {
				fmt.Println(err)
			} else {
				if v, e := jsonparser.GetInt(value, "virtuals", "totalClapCount"); e == nil {
					post.claps = v
				}
				if v, e := jsonparser.GetInt(value, "virtuals", "responsesCreatedCount"); e == nil {
					post.comments = v
				}
				if v, _, _, e := jsonparser.Get(value, "virtuals", "tags"); e == nil {
					post.tags = []Tag{}
					jsonparser.ArrayEach(v, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
						tag := &Tag{}
						if e := json.Unmarshal(value, tag); e == nil {
							post.tags = append(post.tags, *tag)
						} else {
							fmt.Println(e)
						}
					})
				}
				if v, e := jsonparser.GetInt(value, "virtuals", "wordCount"); e == nil {
					post.wordCount = v
				}
				if v, e := jsonparser.GetString(value, "virtuals", "previewImage", "imageId"); e == nil {
					post.previewImage = v
				}
				jsonparser.ArrayEach(value, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
					if b, e := jsonparser.GetString(value, "text"); e == nil {
						post.body = post.body + " " + b
					}
				}, "previewContent", "bodyModel", "paragraphs")
				m[post.ID] = *post
			}
			return nil
		})
	}
	return m
}

func gatherFinalData() (map[string]User, map[string]Post) {
	htmlDir := "data/archiveHtmls"
	files, err := ioutil.ReadDir(htmlDir)
	if err != nil {
		log.Fatal(err)
	}

	allUsers := make(map[string]User)
	allPosts := make(map[string]Post)
	mux := &sync.Mutex{}

	for _, tagDir := range files {
		fullTagDir := htmlDir + "/" + tagDir.Name()
		htmls, err := ioutil.ReadDir(fullTagDir)
		if err != nil {
			log.Fatal(err)
		}
		wg := &sync.WaitGroup{}
		c := make(chan bool, 100)
		for _, html := range htmls {
			htmlFilePath := fullTagDir + "/" + html.Name()
			c <- true
			wg.Add(1)
			go func(path string) {
				b, err := ioutil.ReadFile(path)
				if err != nil {
					fmt.Println(err)
				}
				content := string(b)
				tagStart := "<![CDATA["
				tagEnd := "]]"
				n := strings.Count(content, tagStart)
				if n == 3 {
					st := strings.LastIndex(content, tagStart) + 9
					ed := strings.LastIndex(content, tagEnd)
					cData := content[st:ed]
					json := cData[strings.Index(cData, "{"):strings.LastIndex(cData, "}")]
					json = strings.Replace(json, "\\x3c", "<", -1)
					json = strings.Replace(json, "\\x3e", ">", -1)
					users := getUsers(json)
					posts := getPosts(json)
					mux.Lock()
					for key, value := range users {
						allUsers[key] = value
					}
					for key, value := range posts {
						allPosts[key] = value
					}
					mux.Unlock()
				}
				<-c
				wg.Done()
			}(htmlFilePath)
		}
		wg.Wait()
	}
	return allUsers, allPosts
}

func addUser(user *User) {
	values := map[string]interface{}{
		"email":    user.Username + "@localhost",
		"handle":   user.Username,
		"password": user.Username,
	}
	jsonValue, _ := json.Marshal(values)
	resp, err := httpClient.Post("http://localhost:3000/register", "application/json", bytes.NewBuffer(jsonValue))
	if err != nil {
		log.Fatalln(err)
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		fmt.Println(string(body))
	} else {
		user.dbID = string(body)
	}
	values = map[string]interface{}{
		"query": `mutation {
			update_jotts_user(where: {handle: {_eq: "` + user.Username + `"}}, _set: {name: "` + user.Name + `", profile_picture: "` + user.ImageID + `"}) {
			  affected_rows
			}
		  }`,
	}
	uJsonValue, _ := json.Marshal(values)
	uResp, err := httpClient.Post("http://localhost:8080/v1alpha1/graphql", "application/json", bytes.NewBuffer(uJsonValue))
	if err != nil {
		log.Fatalln(err)
	}
	defer uResp.Body.Close()
	if uResp.StatusCode != 200 {
		body, _ := ioutil.ReadAll(uResp.Body)
		fmt.Println(string(body))
	}
}

func addPost(post *Post, allAuthors *map[string]User) {
	author, ok := (*allAuthors)[post.CreatorID]
	fmt.Println(author.dbID)
	if !ok {
		log.Fatalln(post)
	}
	if author.dbID == "" {
		val, _ := json.Marshal(map[string]string{
			"query": `{
				jotts_user(where: {handle: {_eq: "` + author.Username + `"}}) {
				  id
				}
			  }
			  `,
		})
		resp, err := httpClient.Post("http://localhost:8080/v1alpha1/graphql", "application/json", bytes.NewBuffer(val))
		if err != nil {
			log.Fatalln(err)
		}
		defer resp.Body.Close()
		body, _ := ioutil.ReadAll(resp.Body)
		if resp.StatusCode != 200 {
			fmt.Println(string(body))
		}
		if authorID, err := jsonparser.GetString(body, "data", "jotts_user", "[0]", "id"); err == nil {
			author.dbID = authorID
		}
	}
	values := map[string]interface{}{
		"query": `mutation {
			insert_jotts_post(objects: {content: "` + post.body + `", slug: "` + post.Slug + `", author_id: "` + author.dbID + `", title: "` + post.Title + `"}) {
			  returning {
				id
			  }
			}
		  }
		  `,
	}
	val, _ := json.Marshal(values)
	resp, err := httpClient.Post("http://localhost:8080/v1alpha1/graphql", "application/json", bytes.NewBuffer(val))
	if err != nil {
		log.Fatalln(err)
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		fmt.Println(string(body))
	} else {
		insertRes, err := jsonparser.GetString(body, "data", "insert_jotts_post", "returning", "[0]", "id")
		if err != nil {
			log.Fatalln(string(body))
		}
		post.dbID = string(insertRes)
	}

	for _, tag := range post.tags {
		addTagReq, _ := json.Marshal(map[string]string{
			"query": `mutation {
				insert_jotts_tag(objects: {tag: "` + tag.Slug + `"}, on_conflict: {constraint: tag_pkey, update_columns: tag}) {
				  affected_rows
				}
				insert_jotts_post_tag(objects: {tag: "` + tag.Slug + `", post_id: "` + post.dbID + `"}, on_conflict: {constraint: post_tag_pkey, update_columns: post_id}) {
				  affected_rows
				}
			  }			  
			  `,
		})
		resp, err := httpClient.Post("http://localhost:8080/v1alpha1/graphql", "application/json", bytes.NewBuffer(addTagReq))
		if err != nil {
			log.Fatalln(err)
		}
		defer resp.Body.Close()
		body, _ := ioutil.ReadAll(resp.Body)
		if resp.StatusCode != 200 {
			fmt.Println(string(body))
		}
	}
}

func main() {
	allUsers, allPosts := gatherFinalData()
	count := 0
	wg := &sync.WaitGroup{}
	c := make(chan bool, 20)
	for _, post := range allPosts {
		if post.Type != "Post" && post.Language != "en" {
			continue
		}
		count++
		c <- true
		wg.Add(1)
		go func(post Post) {
			addPost(&post, &allUsers)
			<-c
			wg.Done()
		}(post)
	}
	wg.Wait()
	fmt.Println(count)
}
