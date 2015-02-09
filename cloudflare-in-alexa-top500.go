package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/xmlpath"
	"net"
	"net/http"
	"strings"
	"sync"
)

const maxConcurrency = 4 // for example
var throttle = make(chan int, maxConcurrency)

// get http request
func get_http(url string) (content io.Reader, err error) {
	response, err := http.Get(url)
	if err != nil {
		return nil, err
	} else {
		return response.Body, err
	}
}

// extract websites from page
func extract_sites(content io.Reader) (sites []string, err error) {
	// xpath query for site links
	path := xmlpath.MustCompile("//div/p/a")
	root, err := xmlpath.ParseHTML(content)
	if err != nil {
		return []string{}, err
	}

	// iterate over results
	iter := path.Iter(root)
	for iter.Next() {
		node := iter.Node()
		site := strings.ToLower("www." + node.String())
		sites = append(sites, site)
	}
	return sites, nil
}

// get one page of alexatop500
func alexatop500_page(page int, sites []string, wg *sync.WaitGroup, throttle chan int) (err error) {
	defer wg.Done()

	// determine right url
	base_url := "http://www.alexa.com/topsites"
	url := base_url
	if page > 0 {
		url = fmt.Sprintf("%s/global;%d", base_url, page)
	}

	// get content
	content, err := get_http(url)
	if err != nil {
		return err
	}

	// extract sites
	ex_sites, err := extract_sites(content)
	for index, site := range ex_sites {
		i := 25*page + index
		sites[i] = site
	}
	if err != nil {
		return err
	}
	<-throttle
	return nil
}

// get alexatop500
func alexatop500() (sites []string, err error) {
	max_page := 20
	sites = make([]string, 500)
	var wg sync.WaitGroup
	for page := 0; page < max_page; page++ {
		throttle <- 1
		wg.Add(1)
		go alexatop500_page(page, sites, &wg, throttle)
	}
	wg.Wait()
	return sites, nil
}

// get cloudflare ip nets
func cloudflare_ips() (nets []*net.IPNet) {

	url := "https://www.cloudflare.com/ips-v"

	url_v4 := url + "4"
	body_v4, _ := get_http(url_v4)
	content_v4, _ := ioutil.ReadAll(body_v4)

	url_v6 := url + "6"
	body_v6, _ := get_http(url_v6)
	content_v6, _ := ioutil.ReadAll(body_v6)

	content := fmt.Sprintf("%s\n%s", content_v4, content_v6)

	for _, net_string := range strings.Split(content, "\n") {
		_, net_ipnet, _ := net.ParseCIDR(net_string)
		nets = append(nets, net_ipnet)
	}

	return nets
}

func check_cloudflare(hosts []string, nets []*net.IPNet) {
	fmt.Println("CloudFlare domains within first Top500:")
	var wg sync.WaitGroup
	for index, host := range hosts {
		throttle <- 1
		wg.Add(1)
		go check_cloudflare_host(index, host, nets, &wg, throttle)
	}
	wg.Wait()
}

func ips_in_nets(ips []net.IP, nets []*net.IPNet) (cloudflare bool) {
	for _, net := range nets {
		for _, ip := range ips {
			if net.Contains(ip) {
				return true
			}
		}
	}
	return false
}

func check_cloudflare_host(index int, host string, nets []*net.IPNet, wg *sync.WaitGroup, throttle chan int) {
	defer wg.Done()
	ips, err := net.LookupIP(host)
	if err == nil {
		if ips_in_nets(ips, nets) {
			fmt.Printf("%3d. %s\n", index+1, host)
		}
	}
	<-throttle
}

func main() {

	// Get top 500 domains
	top500, _ := alexatop500()

	// Get cloudflare ip space
	nets := cloudflare_ips()

	// Check domain names if resolve to cloudflare
	check_cloudflare(top500, nets)
}
