package main

import (
	"compress/bzip2"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

const (
	baseURL = "https://opendata.dwd.de/weather/nwp/icon-eu/grib/"
)

// Version info
var (
	version = "dev" // This will be overridden by -ldflags during build
)

// Command line flags
var (
	modelRun      = flag.String("run", "", "Model run time in format HH (e.g., 00, 06, 12, 18)")
	paramList     = flag.String("params", "", "Comma-separated list of parameters to download (e.g., t_2m,clct,pmsl)")
	latest        = flag.Bool("latest", false, "Download the latest available model run")
	outputDir     = flag.String("outdir", ".", "Directory to save downloaded files")
	maxConcurrent = flag.Int("concurrent", 5, "Maximum number of concurrent downloads")
	verbose       = flag.Bool("verbose", false, "Enable verbose output")
	maxRetries    = flag.Int("retries", 5, "Maximum number of retry attempts for failed downloads")
	showVersion   = flag.Bool("version", false, "Show version information")
)

type ModelRun struct {
	Time      string    // The run hour (e.g., "00", "12")
	URL       string    // The URL to the run directory
	Timestamp time.Time // The actual timestamp of the run
}

type Parameter struct {
	Name string
	URL  string
}

func main() {
	flag.Parse()

	// Handle version flag
	if *showVersion {
		// Try to get build info if available
		if info, ok := debug.ReadBuildInfo(); ok && version == "dev" {
			version = info.Main.Version
		}
		fmt.Printf("ICON GRIB Downloader version %s\n", version)
		os.Exit(0)
	}

	log.Println("Starting ICON GRIB downloader")

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	// Validate command line parameters
	if *latest && *modelRun != "" {
		log.Fatal("Cannot specify both -latest and -run flags")
	}

	if !*latest && *modelRun == "" {
		log.Fatal("Either -latest or -run must be specified")
	}

	log.Println("Fetching available model runs from:", baseURL)

	// Get available model runs
	availableRuns, err := getAvailableModelRuns()
	if err != nil {
		log.Fatalf("Failed to get available model runs: %v", err)
	}

	if len(availableRuns) == 0 {
		log.Fatal("No model runs found")
	}

	// Sort runs by actual timestamp (newest first)
	sort.Slice(availableRuns, func(i, j int) bool {
		return availableRuns[i].Timestamp.After(availableRuns[j].Timestamp)
	})

	// Determine which run to download
	var selectedRun ModelRun
	if *latest {
		selectedRun = availableRuns[0]
		log.Printf("Latest model run: %s (timestamp: %s)", selectedRun.Time, selectedRun.Timestamp.Format("2006-01-02 15:04:05"))
	} else {
		found := false
		for _, run := range availableRuns {
			if run.Time == *modelRun {
				selectedRun = run
				found = true
				break
			}
		}
		if !found {
			log.Fatalf("Model run %s not found. Available runs: %v", *modelRun, getRunTimes(availableRuns))
		}
	}

	// Get available parameters for the selected run
	availableParams, err := getAvailableParameters(selectedRun.URL)
	if err != nil {
		log.Fatalf("Failed to get available parameters: %v", err)
	}

	if len(availableParams) == 0 {
		log.Fatal("No parameters found for the selected model run")
	}

	// Determine which parameters to download
	var paramsToDownload []Parameter
	if *paramList == "" {
		// Download all parameters if none specified
		paramsToDownload = availableParams
		log.Printf("Downloading all %d parameters", len(paramsToDownload))
	} else {
		requestedParams := strings.Split(*paramList, ",")
		for _, requested := range requestedParams {
			found := false
			for _, available := range availableParams {
				if available.Name == requested {
					paramsToDownload = append(paramsToDownload, available)
					found = true
					break
				}
			}
			if !found {
				log.Printf("Warning: Parameter %s not found and will be skipped", requested)
			}
		}
	}

	if len(paramsToDownload) == 0 {
		log.Fatal("No valid parameters to download")
	}

	// Download GRIB files for each parameter
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, *maxConcurrent)

	for _, param := range paramsToDownload {
		wg.Add(1)
		go func(param Parameter) {
			defer wg.Done()
			semaphore <- struct{}{}        // Acquire semaphore
			defer func() { <-semaphore }() // Release semaphore

			if err := downloadGribFiles(param, selectedRun.Time); err != nil {
				log.Printf("Error downloading parameter %s: %v", param.Name, err)
			}
		}(param)
	}

	wg.Wait()
	log.Println("Download completed")
}

// getAvailableModelRuns returns a list of available model runs
func getAvailableModelRuns() ([]ModelRun, error) {
	var runs []ModelRun

	log.Println("Making HTTP request to:", baseURL)
	resp, err := http.Get(baseURL)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	log.Printf("Response status: %s", resp.Status)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get model runs list, status: %s", resp.Status)
	}

	// Read the HTML content
	htmlBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read HTML content: %v", err)
	}
	htmlContent := string(htmlBytes)

	log.Println("Extracting model run directories and timestamps")

	// Regular expression to match run directories and their timestamps
	// Matches patterns like <a href="00/">00/</a>                      12-Mar-2025 02:39    -
	runPattern := regexp.MustCompile(`<a href="(\d\d)/.*?(\d\d-\w+-\d\d\d\d \d\d:\d\d)`)
	matches := runPattern.FindAllStringSubmatch(htmlContent, -1)

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}

		runHour := match[1]
		timestampStr := match[2]

		log.Printf("Found run: %s, timestamp: %s", runHour, timestampStr)

		// Parse the timestamp string
		timestamp, err := time.Parse("02-Jan-2006 15:04", timestampStr)
		if err != nil {
			log.Printf("Warning: couldn't parse timestamp '%s': %v", timestampStr, err)
			continue
		}

		runs = append(runs, ModelRun{
			Time:      runHour,
			URL:       baseURL + runHour + "/",
			Timestamp: timestamp,
		})
	}

	log.Printf("Found %d model runs", len(runs))
	return runs, nil
}

// getAvailableParameters returns a list of available parameters for a model run
func getAvailableParameters(runURL string) ([]Parameter, error) {
	var params []Parameter

	resp, err := http.Get(runURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get parameters list, status: %s", resp.Status)
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, err
	}

	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key == "href" && len(a.Val) > 0 && a.Val != "../" {
					// Format is typically like "parameter_name/"
					if a.Val[len(a.Val)-1] == '/' {
						paramName := a.Val[:len(a.Val)-1] // Remove trailing slash
						params = append(params, Parameter{
							Name: paramName,
							URL:  runURL + a.Val,
						})
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)

	return params, nil
}

// getGribFiles returns a list of GRIB files for a parameter
func getGribFiles(paramURL string) ([]string, error) {
	var files []string

	resp, err := http.Get(paramURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get GRIB files list, status: %s", resp.Status)
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, err
	}

	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key == "href" && strings.HasSuffix(a.Val, ".grib2.bz2") {
					files = append(files, a.Val)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)

	return files, nil
}

// downloadGribFiles downloads all GRIB files for a parameter
func downloadGribFiles(param Parameter, runTime string) error {
	if *verbose {
		log.Printf("Downloading parameter: %s", param.Name)
	}

	files, err := getGribFiles(param.URL)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		return fmt.Errorf("no GRIB files found for parameter %s", param.Name)
	}

	// Create run directory (one directory per model run)
	runDir := filepath.Join(*outputDir, runTime)
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return fmt.Errorf("failed to create run directory: %v", err)
	}

	// Download each GRIB file
	for _, file := range files {
		fileURL := param.URL + file

		// Create a filename with parameter name as prefix to avoid conflicts
		// e.g., "t_2m_icon-eu_europe_regular-lat-lon_single-level_2023030612_000.grib2"
		outputFilename := fmt.Sprintf("%s_%s", param.Name, file)
		if strings.HasSuffix(outputFilename, ".bz2") {
			outputFilename = outputFilename[:len(outputFilename)-4] // Remove .bz2 extension
		}

		localPath := filepath.Join(runDir, outputFilename)

		// Skip if file already exists and has non-zero size
		if fileInfo, err := os.Stat(localPath); err == nil && fileInfo.Size() > 0 {
			if *verbose {
				log.Printf("Skipping existing file: %s", localPath)
			}
			continue
		}

		// Download and uncompress file with retries
		if err := downloadAndUncompressFile(fileURL, localPath, *maxRetries); err != nil {
			log.Printf("Error downloading %s: %v", fileURL, err)
			continue
		}

		if *verbose {
			log.Printf("Downloaded and uncompressed: %s", localPath)
		}
	}

	return nil
}

// downloadAndUncompressFile downloads a single file, uncompresses it from bz2, and retries on failure
func downloadAndUncompressFile(url, destPath string, retries int) error {
	var lastErr error

	for attempt := 0; attempt <= retries; attempt++ {
		if attempt > 0 {
			if *verbose {
				log.Printf("Retry attempt %d/%d for %s", attempt, retries, url)
			}
			// Add exponential backoff delay
			delay := time.Duration(attempt*attempt) * time.Second
			time.Sleep(delay)
		}

		// Create a temporary file for the compressed content
		tempFile := destPath + ".bz2.tmp"

		// Download the compressed file
		err := downloadFile(url, tempFile)
		if err != nil {
			lastErr = err
			log.Printf("Download attempt %d failed: %v", attempt+1, err)
			// Cleanup temp file if it exists
			os.Remove(tempFile)
			continue
		}

		// Open the compressed file
		compressedFile, err := os.Open(tempFile)
		if err != nil {
			lastErr = err
			log.Printf("Failed to open compressed file: %v", err)
			os.Remove(tempFile)
			continue
		}

		// Create the output file
		outputFile, err := os.Create(destPath)
		if err != nil {
			compressedFile.Close()
			lastErr = err
			log.Printf("Failed to create output file: %v", err)
			os.Remove(tempFile)
			continue
		}

		// Create bzip2 reader
		bz2Reader := bzip2.NewReader(compressedFile)

		// Copy and decompress
		_, err = io.Copy(outputFile, bz2Reader)

		// Close files
		compressedFile.Close()
		outputFile.Close()

		// Check decompression result
		if err != nil {
			lastErr = err
			log.Printf("Decompression failed: %v", err)
			os.Remove(tempFile)
			os.Remove(destPath) // Remove partial output file
			continue
		}

		// Cleanup temp file
		os.Remove(tempFile)

		// If we got here, everything succeeded
		return nil
	}

	return fmt.Errorf("failed after %d attempts: %v", retries, lastErr)
}

// downloadFile downloads a single file
func downloadFile(url, destPath string) error {
	client := &http.Client{
		Timeout: 10 * time.Minute, // GRIB files can be large
	}

	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %s", resp.Status)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// parseInt safely converts a string to an integer with error handling
func parseInt(s string) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return i
}

// getRunTimes returns a list of available run times
func getRunTimes(runs []ModelRun) []string {
	var times []string
	for _, run := range runs {
		times = append(times, run.Time)
	}
	return times
}
