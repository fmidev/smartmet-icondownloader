package main

import (
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"
)

const baseURL = "https://opendata.dwd.de/weather/nwp/icon-eu/grib/"

func main() {
	log.Println("Starting simple debug tool")
	log.Println("Making HTTP request to:", baseURL)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Make the request
	resp, err := client.Get(baseURL)
	if err != nil {
		log.Fatalf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	log.Printf("Response status: %s", resp.Status)

	// Read the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Failed to read response body: %v", err)
	}

	// Print the raw HTML
	htmlContent := string(body)
	log.Printf("HTML content length: %d bytes", len(htmlContent))

	// Extract links using simple string parsing
	lines := strings.Split(htmlContent, "\n")
	var runs []string

	for _, line := range lines {
		if strings.Contains(line, "href=") && strings.Contains(line, "/") {
			parts := strings.Split(line, "href=\"")
			if len(parts) > 1 {
				linkParts := strings.Split(parts[1], "\"")
				if len(linkParts) > 0 {
					link := linkParts[0]
					if len(link) > 0 && link != "../" {
						log.Printf("Found link: %s", link)
						runs = append(runs, link)
					}
				}
			}
		}
	}

	log.Printf("Found %d potential model runs", len(runs))

	for _, run := range runs {
		log.Printf("Run: %s", run)
	}

	log.Println("Debug tool completed")
}
