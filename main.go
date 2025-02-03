package main

import (
    "bufio"
    "bytes"
    "compress/gzip"
    "compress/zlib"
    "crypto/tls"
    "flag"
    "fmt"
    "io"
    "net/http"
    "os"
    "strings"
    "sync"
)

const maxLineLength = 100

func main() {
    // Define the command-line flags
    requestFile := flag.String("request-file", "", "Path to the HTTP request file")
    requestNumber := flag.Int("request-number", 1, "Number of concurrent HTTP requests to send")
    flag.Parse()

    if *requestFile == "" {
        text := `Ripit is CLI tool that able to repeat HTTP requests from Burpsuite requests.

Usage:
    ripit --request-file request.txt
    ripit --request-file request.txt --request-number 5

Information:
    --request-file   (provide location of burp plain text request file)
    --request-number (provide number of requests. can be used for race conditions)
    `
    fmt.Println(text)
        return
    }

    // Read and parse the HTTP request from the file
    method, url, headers, body, err := parseHTTPRequest(*requestFile)
    if err != nil {
        fmt.Println("Error parsing request file:", err)
        return
    }

    // Make the HTTP requests concurrently
    var wg sync.WaitGroup
    for i := 0; i < *requestNumber; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            httpRequest(url, method, body, headers)
        }()
    }
    wg.Wait()
}

func parseHTTPRequest(filename string) (string, string, map[string]string, []byte, error) {
    file, err := os.Open(filename)
    if err != nil {
        return "", "", nil, nil, err
    }
    defer file.Close()

    scanner := bufio.NewScanner(file)
    var method, url string
    headers := make(map[string]string)
    var body []byte
    inBody := false

    for scanner.Scan() {
        line := scanner.Text()

        // Parse request line (method and URL)
        if !inBody && strings.Contains(line, " HTTP/") {
            parts := strings.Split(line, " ")
            if len(parts) >= 2 {
                method = parts[0]
                url = parts[1]
            }
            continue
        }

        // Parse headers
        if !inBody && line == "" {
            inBody = true
            continue
        }
        if !inBody {
            parts := strings.SplitN(line, ": ", 2)
            if len(parts) == 2 {
                headers[parts[0]] = parts[1]
            }
        } else {
            body = append(body, line...)
        }
    }

    if err := scanner.Err(); err != nil {
        return "", "", nil, nil, err
    }

    if host, ok := headers["Host"]; ok {
        url = "https://" + host + url
    }

    return method, url, headers, body, nil
}

func wrapText(text string, maxLength int) string {
    if len(text) <= maxLength {
        return text
    }
    var result string
    for len(text) > maxLength {
        result += text[:maxLength] + "\n"
        text = text[maxLength:]
    }
    result += text
    return result
}

func httpRequest(targetUrl string, method string, data []byte, headers map[string]string) *http.Response {
    request, err := http.NewRequest(method, targetUrl, bytes.NewBuffer(data))
    if err != nil {
        panic(err)
    }
    for k, v := range headers {
        request.Header.Set(k, v)
    }

    customTransport := &http.Transport{
        TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
    }
    client := &http.Client{Transport: customTransport}
    response, err := client.Do(request)
    if err != nil {
        panic(err)
    }
    defer response.Body.Close()

    fmt.Println("-------------------------------------------------------------------------------------------------------")
    fmt.Println("Response Status:", response.Status)
    fmt.Println(" ")
    fmt.Println("Response Headers:\n")
    for k, v := range response.Header {
        fmt.Println(wrapText(k+": "+strings.Join(v, ""), maxLineLength))
    }
    fmt.Println(" ")

    var reader io.ReadCloser
    switch response.Header.Get("Content-Encoding") {
    case "gzip":
        reader, err = gzip.NewReader(response.Body)
        if err != nil {
            panic(err)
        }
        defer reader.Close()
    case "deflate":
        reader, err = zlib.NewReader(response.Body)
        if err != nil {
            panic(err)
        }
        defer reader.Close()
    default:
        reader = response.Body
    }

    body, err := io.ReadAll(reader)
    if err != nil {
        panic(err)
    }

    fmt.Println("Response Body: \n")
    fmt.Println(wrapText(string(body), maxLineLength))
    fmt.Println("------------------------------------------------------------------------------------------------------")
    return response
}
