package main

import (
	"encoding/csv"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cheggaaa/pb"
)

const (
	workerCount = 200
	valid       = "valid"
	parked      = "parked"
	httpError   = "http error"
	ioError     = "io error"
)

// Domain is used to store a domain name & status
type Domain struct {
	Name    string
	Status  string
	Message string
}

func getDomains(fileName string) []string {
	csvFile, err := os.Open(fileName)
	if err != nil {
		log.Fatal(err)
	}
	defer csvFile.Close()

	reader := csv.NewReader(csvFile)
	reader.FieldsPerRecord = -1

	rawCSVdata, err := reader.ReadAll()

	if err != nil {
		log.Fatal(err)
	}

	records := len(rawCSVdata) - 1
	domains := make([]string, records, records)

	var domainIndex int
	for idx, each := range rawCSVdata {
		if idx == 0 {
			domainIndex = 3 // TODO
			continue
		}
		domainName := each[domainIndex]
		domains[idx-1] = domainName
	}

	return domains
}

func worker(id int, domains <-chan string, results chan<- Domain) {
	for domain := range domains {
		results <- checkDomain(domain, false)
	}
}

func checkDomains(domains []string) []Domain {
	total := len(domains)
	bar := pb.StartNew(total)
	validCount := 0

	domainNameChan := make(chan string, total)
	domainStatusChan := make(chan Domain, 100)
	for w := 1; w <= workerCount; w++ {
		go worker(w, domainNameChan, domainStatusChan)
	}

	for _, domain := range domains {
		domainNameChan <- domain
	}
	close(domainNameChan)

	processedDomains := make([]Domain, total)
	for a := 1; a <= total; a++ {
		bar.Increment()
		domain := <-domainStatusChan
		if domain.Status == valid {
			validCount++
		}
		processedDomains = append(processedDomains, domain)
	}
	close(domainStatusChan)
	bar.FinishPrint(fmt.Sprintf("Processed %v domains with %v valid.", total, validCount))

	return processedDomains
}

func newDomain(name, status, message string) Domain {
	return Domain{Name: name, Status: status, Message: message}
}

func checkDomain(domainName string, retry bool) Domain {
	url := fmt.Sprintf("http://%v", domainName)

	timeout := time.Duration(10 * time.Second)
	client := http.Client{
		Timeout: timeout,
	}
	resp, err := client.Get(url)

	if err != nil {
		if retry {
			return newDomain(domainName, httpError, err.Error())
		}
		return checkDomain(domainName, true)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return newDomain(domainName, ioError, err.Error())
	}

	bodyString := strings.ToLower(string(body))
	if strings.Contains(bodyString, "pagemetaid") {
		return Domain{Name: domainName, Status: valid}
	}

	location, err := resp.Location()
	if err == nil && !strings.Contains(location.Host, domainName) {
		return newDomain(domainName, "redirected", location.Host)
	}

	if strings.Contains(bodyString, "www.footballfanatics.com") {
		return newDomain(domainName, "old", "Likely an old fanatics codebase?")
	} else if strings.Contains(bodyString, "godaddy") {
		return newDomain(domainName, parked, "godaddy")
	} else if strings.Contains(bodyString, "imptestrm.com") {
		return newDomain(domainName, parked, "imptestrm.com")
	}
	return newDomain(domainName, parked, "domain valid but doesn't contain PageMetaID - likely parked")
}

func writeStatus(fileName string, domains []Domain) {
	csvFile, err := os.Create(fileName)
	if err != nil {
		log.Fatal(err)
	}
	w := csv.NewWriter(csvFile)
	w.Write([]string{"domain", "status", "message"})
	for _, domain := range domains {
		if len(domain.Name) == 0 {
			continue
		}
		log.Printf("%v %v %v", domain.Name, domain.Status, domain.Message)
		w.Write([]string{domain.Name, domain.Status, domain.Message})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		log.Fatal(err)
	}
}

func main() {
	fileName := "domains.csv"
	domains := getDomains(fileName)
	output := checkDomains(domains)
	writeStatus("output.csv", output)
}
