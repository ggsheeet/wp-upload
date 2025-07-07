package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
)

type Post struct {
	Title    string
	Category string
	Image    string
	Content  string
	URL      string
}

func extractURL(content string) string {
	urlPattern := regexp.MustCompile(`https?://[^\s]+`)
	matches := urlPattern.FindStringSubmatch(content)
	if len(matches) > 0 {
		return matches[0]
	}
	return ""
}

func parsePosts(filename string) ([]Post, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var posts []Post
	var currentPost *Post
	var contentBuilder strings.Builder

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "Title: ") {
			if currentPost != nil {
				currentPost.Content = contentBuilder.String()
				currentPost.URL = extractURL(currentPost.Content)
				posts = append(posts, *currentPost)
				contentBuilder.Reset()
			}

			currentPost = &Post{
				Title: strings.TrimPrefix(line, "Title: "),
			}
		} else if currentPost != nil && strings.HasPrefix(line, "Category: ") {
			currentPost.Category = strings.TrimPrefix(line, "Category: ")
		} else if currentPost != nil && strings.HasPrefix(line, "Image: ") {
			currentPost.Image = strings.TrimPrefix(line, "Image: ")
		} else if currentPost != nil {
			contentBuilder.WriteString(line)
			contentBuilder.WriteString("\n")
		}
	}

	if currentPost != nil {
		currentPost.Content = contentBuilder.String()
		currentPost.URL = extractURL(currentPost.Content)
		posts = append(posts, *currentPost)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return posts, nil
}

func getOGImage(url string) (string, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("received non-200 response code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	ogImagePattern := regexp.MustCompile(`<meta\s+(?:property=["']og:image["']|content=["']([^"']+)["']|name=["']og:image["'])+(?:\s+(?:property=["']og:image["']|content=["']([^"']+)["']|name=["']og:image["']))+`)
	matches := ogImagePattern.FindAllStringSubmatch(string(body), -1)

	for _, match := range matches {
		for _, group := range match[1:] {
			if group != "" && (strings.HasPrefix(group, "http://") || strings.HasPrefix(group, "https://")) {
				cleanedURL := strings.ReplaceAll(group, "&amp;", "&")
				return cleanedURL, nil
			}
		}
	}

	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}

	var ogImage string
	var findOGImage func(*html.Node)
	findOGImage = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "meta" {
			var property, content string
			for _, attr := range n.Attr {
				if attr.Key == "property" && attr.Val == "og:image" {
					property = attr.Val
				}
				if attr.Key == "content" {
					content = attr.Val
				}
			}
			if property == "og:image" && content != "" {
				cleanedURL := strings.ReplaceAll(content, "&amp;", "&")
				ogImage = cleanedURL
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findOGImage(c)
		}
	}
	findOGImage(doc)

	if ogImage != "" {
		return ogImage, nil
	}

	return "", fmt.Errorf("no OG image found")
}

func writePosts(posts []Post, outputFile string) error {
	file, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)

	for _, post := range posts {
		writer.WriteString(fmt.Sprintf("Title: %s\n", post.Title))
		writer.WriteString(fmt.Sprintf("Category: %s\n", post.Category))
		writer.WriteString(fmt.Sprintf("Image: %s\n", post.Image))
		writer.WriteString(post.Content)
	}

	return writer.Flush()
}

func main() {
	if len(os.Args) < 2 {
		logger.Info("Usage:")
		logger.Info("  go run . format     - Format the document (first step)")
		logger.Info("  go run . process    - Process OG images (stops if errors found)")
		logger.Info("  go run . upload     - Upload posts to WordPress")
		logger.Info("  go run . upload N   - Resume upload from post N (0-based index)")
		logger.Info("  go run . full       - Process OG images and upload (only if no errors)")
		return
	}

	command := os.Args[1]

	switch command {
	case "format":
		formatDoc()
	case "process":
		processOGImages()
	case "upload":
		startIndex := 0
		if len(os.Args) > 2 {
			fmt.Sscanf(os.Args[2], "%d", &startIndex)
		}
		runUploadOnly(startIndex)
	case "full":
		formatDoc()
		if processOGImages() {
			logger.Info("Starting WordPress upload...")
			runUploadOnly(0)
		} else {
			logger.Warning("OG image processing had errors. Please fix images manually and run 'go run . upload' when ready.")
		}
	default:
		logger.Error("Unknown command: %s", command)
		logger.Info("Use 'go run . process', 'go run . upload', or 'go run . full'")
	}
}

func processOGImages() bool {
	inputFile := "posts.txt"

	posts, err := parsePosts(inputFile)
	if err != nil {
		logger.Error("Error parsing posts: %v", err)
		return false
	}

	logger.Info("Processing %d posts for OG images...", len(posts))

	hasErrors := false
	processedCount := 0

	for i := range posts {
		if posts[i].URL != "" {
			ogImage, err := getOGImage(posts[i].URL)
			if err != nil {
				logger.Error("[%d/%d] %s - OG image error: %v", i+1, len(posts), posts[i].Title, err)
				hasErrors = true
				continue
			}

			if ogImage != "" {
				ogImage = strings.ReplaceAll(ogImage, "&amp;", "&")
				ogImage = strings.ReplaceAll(ogImage, " ", "%20")

				if !strings.HasPrefix(posts[i].Image, "http") {
					posts[i].Image = ogImage + " " + posts[i].Image
				}
				processedCount++
				logger.Info("[%d/%d] %s - OG image found", i+1, len(posts), posts[i].Title)
			} else {
				logger.Warning("[%d/%d] %s - No OG image found", i+1, len(posts), posts[i].Title)
			}

			time.Sleep(100 * time.Millisecond)
		}
	}

	err = writePosts(posts, inputFile)
	if err != nil {
		logger.Error("Error writing posts: %v", err)
		return false
	}

	if hasErrors {
		logger.Warning("Errors found. Fix image URLs manually in %s and run 'go run . upload' when ready", inputFile)
	} else {
		logger.Info("All OG images processed successfully! Posts updated in %s", inputFile)
	}

	return !hasErrors
}
