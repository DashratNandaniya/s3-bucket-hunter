package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	ResetColor  = "\033[0m"
	BrightRed   = "\033[1;91m"
	BrightGreen = "\033[1;92m"
	BrightCyan  = "\033[1;96m"
)

func c(s string, color string) string { return color + s + ResetColor }
func prefixOk() string               { return c("[✔] ", BrightGreen) }
func prefixErr() string              { return c("[✘] ", BrightRed) }
func prefixInfo() string             { return c("[*] ", BrightCyan) }

func normalizeURL(u string) string {
	u = strings.TrimSpace(u)
	if u == "" || strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return u
	}
	return "https://" + u
}

func extractBucketName(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		raw = strings.TrimSpace(raw)
		if raw == "" { return "" }
		parsed2, _ := url.Parse("https://" + raw)
		parsed = parsed2
	}
	if parsed == nil { return "" }
	host, path := parsed.Host, strings.TrimLeft(parsed.Path, "/")

	if strings.Contains(host, ".s3") {
		parts := strings.Split(host, ".s3")
		if len(parts) > 0 && parts[0] != "" { return parts[0] }
	}
	if strings.HasPrefix(host, "s3.") || host == "s3.amazonaws.com" {
		if path != "" { return strings.Split(path, "/")[0] }
	}
	if path != "" { return strings.Split(path, "/")[0] }
	if host != "" { return strings.Split(host, ".")[0] }
	return ""
}

func main() {
	fmt.Println(c("============================================================", BrightRed))
	fmt.Println(c("🔥 S3 Public Bucket Listing Detector 🔥", BrightRed))
	fmt.Println(c("============================================================", BrightRed))

	fmt.Print(c("[?] Enter input URL file name: ", BrightGreen))
	reader := bufio.NewReader(os.Stdin)
	urlFile, _ := reader.ReadString('\n')
	urlFile = strings.TrimSpace(urlFile)

	f, err := os.Open(urlFile)
	if err != nil {
		fmt.Println(prefixErr() + "Error opening file: " + err.Error())
		return
	}
	defer f.Close()

	outFile, err := os.OpenFile("public_buckets.txt", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Println(prefixErr() + "Error creating output file: " + err.Error())
		return
	}
	defer outFile.Close()

	scanner := bufio.NewScanner(f)
	client := &http.Client{Timeout: 10 * time.Second}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" { continue }
		u := normalizeURL(line)

		fmt.Println(prefixInfo() + "Checking: " + u)
		req, _ := http.NewRequest("GET", u, nil)
		req.Header.Set("User-Agent", "s3bucket-go/1.0")
		
		resp, err := client.Do(req)
		if err != nil {
			fmt.Println("    " + prefixErr() + "Request failed:", err)
			continue
		}

		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if strings.Contains(string(bodyBytes), "<ListBucketResult") {
			bucket := extractBucketName(u)
			if bucket != "" {
				fmt.Printf("    %s PUBLIC bucket detected: %s\n", prefixOk(), bucket)
				_, _ = outFile.WriteString(bucket + "\n")
			}
		} else {
			fmt.Println("    " + prefixErr() + "Not public / Access Denied")
		}
		fmt.Println("------------------------------------------------------------")
	}
	fmt.Println(prefixOk() + "Scan completed! Public buckets saved to 'public_buckets.txt'")
}