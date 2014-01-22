package downloader

import (
	"crawler/downloader/graphite"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
)

type RedirectorHandler struct {
	metricSender   *graphite.Client
	processedLinks *BloomFilter
	linksChannel   []chan string
	patterns       []*regexp.Regexp
}

func (self *RedirectorHandler) Match(link string) bool {
	for _, pt := range self.patterns {
		if pt.FindString(link) == link {
			return true
		}
	}
	return false
}

func (self *RedirectorHandler) Redirect(ci int) {
	for link := range self.linksChannel[ci] {
		if !self.Match(link) {
			continue
		}
		if self.processedLinks.Contains(link) {
			continue
		}
		self.processedLinks.Add(link)

		fmt.Println(link)

		pb := PostBody{}
		pb.Links = []string{link}
		jsonBlob, err := json.Marshal(&pb)
		if err == nil {
			post := url.Values{}
			post.Set("links", string(jsonBlob))

			resp, err := http.PostForm(ConfigInstance().DownloaderHost, post)
			defer resp.Body.Close()
			ioutil.ReadAll(resp.Body)
			if err != nil {
				fmt.Println(err)
			}
		}
		time.Sleep(60 * time.Second / time.Duration(ConfigInstance().PagePerMinute))
	}
}

func NewRedirectorHandler() *RedirectorHandler {
	ret := RedirectorHandler{}
	ret.metricSender, _ = graphite.New(ConfigInstance().GraphiteHost, "")
	ret.linksChannel = []chan string{}
	for i := 0; i < ConfigInstance().RedirectChanNum; i++ {
		ret.linksChannel = append(ret.linksChannel, make(chan string, 100000))
	}
	ret.processedLinks = NewBloomFilter()
	for _, pt := range ConfigInstance().SitePatterns {
		re := regexp.MustCompile(pt)
		ret.patterns = append(ret.patterns, re)
	}
	for i := 0; i < ConfigInstance().RedirectChanNum; i++ {
		go ret.Redirect(i)
	}
	return &ret
}

func (self *RedirectorHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println(r)
		}
	}()

	links := req.PostFormValue("links")
	if len(links) > 0 {
		pb := PostBody{}
		json.Unmarshal([]byte(links), &pb)

		for _, link := range pb.Links {
			if !self.Match(link) {
				continue
			}
			ci := Hash(ExtractDomain(link)) % int32(ConfigInstance().RedirectChanNum)
			fmt.Println("channel length", ci, len(self.linksChannel[ci]))
			self.linksChannel[ci] <- link
		}
	}
	fmt.Fprint(w, "")
}
