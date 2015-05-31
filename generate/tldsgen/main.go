/* Copyright (c) 2015, Daniel Martí <mvdan@mvdan.cc> */
/* See LICENSE for licensing information */

package main

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"text/template"
)

const path = "tlds.go"

var tldsTmpl = template.Must(template.New("tlds").Parse(`// Generated by tldsgen

package xurls

// TLDs is a sorted list of all public top-level domains
//
// Sources:{{range $_, $url := .URLs}}
//  * {{$url}}{{end}}
var TLDs = []string{
{{range $_, $tld := .TLDs}}` + "\t`" + `{{$tld}}` + "`" + `,
{{end}}}
`))

func cleanTld(tld string) string {
	tld = strings.ToLower(tld)
	if strings.HasPrefix(tld, "xn--") {
		return ""
	}
	return tld
}

func fetchFromURL(url, pat string) {
	log.Printf("Fetching %s", url)
	resp, err := http.Get(url)
	if err == nil && resp.StatusCode >= 400 {
		err = errors.New(resp.Status)
	}
	if err != nil {
		errChan <- fmt.Errorf("could not fetch %s: %v", url, err)
		wg.Done()
		return
	}
	defer resp.Body.Close()
	scanner := bufio.NewScanner(resp.Body)
	re := regexp.MustCompile(pat)
	for scanner.Scan() {
		line := scanner.Text()
		tld := re.FindString(line)
		tld = cleanTld(tld)
		if tld == "" {
			continue
		}
		tldChan <- tld
	}
	wg.Done()
}

var (
	wg      sync.WaitGroup
	tldChan = make(chan string)
	errChan = make(chan error)
)

func tldList() ([]string, []string, error) {

	wg.Add(2)

	var urls []string
	fromURL := func(url, pat string) {
		urls = append(urls, url)
		go fetchFromURL(url, pat)
	}

	fromURL("https://data.iana.org/TLD/tlds-alpha-by-domain.txt",
		`^[^#]+$`)
	fromURL("https://publicsuffix.org/list/effective_tld_names.dat",
		`^[^/.]+$`)

	tldSet := make(map[string]struct{})
	anyError := false
	go func() {
		for {
			select {
			case tld := <-tldChan:
				tldSet[tld] = struct{}{}
			case err := <-errChan:
				log.Printf("%v", err)
				anyError = true
			}
		}
	}()
	wg.Wait()

	if anyError {
		return nil, nil, errors.New("there were some errors while fetching the TLDs")
	}

	tlds := make([]string, 0, len(tldSet))
	for tld := range tldSet {
		tlds = append(tlds, tld)
	}

	sort.Strings(tlds)
	return tlds, urls, nil
}

func writeTlds(tlds, urls []string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	return tldsTmpl.Execute(f, struct {
		TLDs []string
		URLs []string
	}{
		TLDs: tlds,
		URLs: urls,
	})
}

func main() {
	tlds, urls, err := tldList()
	if err != nil {
		log.Fatalf("Could not get TLD list: %s", err)
	}
	log.Printf("Generating %s...", path)
	if err := writeTlds(tlds, urls); err != nil {
		log.Fatalf("Could not write path: %s", err)
	}
}
