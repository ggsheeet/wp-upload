package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"mime"
	"net/http"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

var logger = NewColoredLogger("", nil)

func uploadPosts(startIndex int) {
	if err := godotenv.Load(".env"); err != nil {
		log.Fatal("Error loading .env file")
	}

	posts := execPosts("posts.txt")

	if startIndex >= len(posts) {
		logger.Error("Start index %d is out of range (total posts: %d)", startIndex, len(posts))
		return
	}

	logger.Info("Total posts to upload: %d", len(posts))
	if startIndex > 0 {
		logger.Info("Resuming from post %d", startIndex)
	}

	token := getJWTToken()

	for i := startIndex; i < len(posts); i++ {
		post := posts[i]
		categoryID := getCategoryID(post.Category)
		imageID := uploadFeaturedImage(post.Image, i, token)
		createPost(post.Title, post.Content, categoryID, imageID, i, token)

		logger.Info("[%d/%d] %s", i+1, len(posts), post.Title)
	}
}

func runUploadOnly(startIndex int) {
	uploadPosts(startIndex)
	logger.Info("Upload complete!")
}

func execPosts(filename string) []Post {
	data, err := os.ReadFile(filename)
	if err != nil {
		panic(err)
	}

	lines := strings.Split(string(data), "\n")
	var posts []Post
	var current Post
	contentCount := 0

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		switch {
		case strings.HasPrefix(line, "Title:"):
			if current.Title != "" {
				posts = append(posts, current)
				current = Post{}
			}
			current.Title = strings.TrimSpace(strings.TrimPrefix(line, "Title:"))
			contentCount = 0
		case strings.HasPrefix(line, "Category:"):
			current.Category = strings.TrimSpace(strings.TrimPrefix(line, "Category:"))
		case strings.HasPrefix(line, "Image:"):
			current.Image = strings.TrimSpace(strings.TrimPrefix(line, "Image:"))
		default:
			if current.Content != "" {
				if contentCount == 2 {
					current.Content += fmt.Sprintf(`<!-- wp:paragraph --><p><a href="%s">Ver nota completa</a></p><!-- /wp:paragraph -->`, line)
				} else {
					current.Content += fmt.Sprintf(`<!-- wp:paragraph --><p>%s</p><!-- /wp:paragraph -->`, line)
				}
				contentCount++
			} else {
				current.Content = fmt.Sprintf(`<!-- wp:paragraph --><p>%s</p><!-- /wp:paragraph -->`, line)
				contentCount = 1
			}
		}
	}
	if current.Title != "" {
		posts = append(posts, current)
	}

	return posts
}

func getJWTToken() string {
	payload := strings.NewReader(fmt.Sprintf("username=%s&password=%s", os.Getenv("EMAIL"), os.Getenv("PASSWORD")))
	req, _ := http.NewRequest("POST", "https://gen.boletindiario.in/wp-json/jwt-auth/v1/token", payload)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}
	defer res.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(res.Body).Decode(&result)

	if token, exists := result["token"]; exists && token != nil {
		return token.(string)
	}

	if errorMsg, exists := result["message"]; exists {
		logger.Error("JWT Authentication failed: %v", errorMsg)
		panic(fmt.Sprintf("JWT Authentication failed: %v", errorMsg))
	}

	logger.Error("JWT Authentication failed: No token received")
	panic("JWT Authentication failed: No token received")
}

func getCategoryID(childSlug string) int {
	var parentSlug string
	if childSlug == "menciones-icpnl" {
		parentSlug = "icpnl"
	} else {
		parentSlug = "tronco"
	}

	url := fmt.Sprintf("https://gen.boletindiario.in/wp-json/wp/v2/categories?slug=%s", parentSlug)
	resp, _ := http.Get(url)
	if resp.StatusCode != 200 {
		logger.Error("Failed to get parent category ID: %s", resp.Status)
		panic(fmt.Sprintf("Failed to get category ID: %s", resp.Status))
	}
	defer resp.Body.Close()

	var parentCategories []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&parentCategories)

	if len(parentCategories) == 0 {
		logger.Warning("Parent category '%s' not found", parentSlug)
		return 0
	}

	parentID := int(parentCategories[0]["id"].(float64))

	childURL := fmt.Sprintf("https://gen.boletindiario.in/wp-json/wp/v2/categories?parent=%d&slug=%s", parentID, childSlug)
	resp2, _ := http.Get(childURL)
	if resp2.StatusCode != 200 {
		logger.Error("Failed to get child category ID: %s", resp2.Status)
		panic(fmt.Sprintf("Failed to get category ID: %s", resp2.Status))
	}
	defer resp2.Body.Close()

	var childCategories []map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&childCategories)

	if len(childCategories) == 0 {
		logger.Warning("Child category '%s' not found under parent '%s'", childSlug, parentSlug)
		return 0
	}
	return int(childCategories[0]["id"].(float64))
}

func uploadFeaturedImage(imageURL string, postIndex int, token string) int {
	req, err := http.NewRequest("GET", imageURL, nil)
	if err != nil {
		logger.Error("Failed to create image request for post %d: %v. Resume with: go run . upload %d", postIndex, err, postIndex)
		panic(fmt.Sprintf("Failed to create request for post %d: %v", postIndex, err))
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger.Error("Failed to fetch image for post %d: %v. Resume with: go run . upload %d", postIndex, err, postIndex)
		panic(fmt.Sprintf("Failed to fetch image for post %d: %v", postIndex, err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		logger.Error("Failed to fetch image for post %d: HTTP %d. Resume with: go run . upload %d", postIndex, resp.StatusCode, postIndex)
		panic(fmt.Sprintf("Failed to fetch image for post %d: HTTP %d", postIndex, resp.StatusCode))
	}

	body, _ := io.ReadAll(resp.Body)
	contentType := resp.Header.Get("Content-Type")

	if contentType == "" {
		if strings.HasSuffix(strings.ToLower(imageURL), ".webp") {
			contentType = "image/webp"
		} else if strings.HasSuffix(strings.ToLower(imageURL), ".jpg") || strings.HasSuffix(strings.ToLower(imageURL), ".jpeg") {
			contentType = "image/jpeg"
		} else if strings.HasSuffix(strings.ToLower(imageURL), ".png") {
			contentType = "image/png"
		} else if strings.HasSuffix(strings.ToLower(imageURL), ".gif") {
			contentType = "image/gif"
		} else {
			contentType = "image/jpeg"
		}
		logger.Debug("Content-type detected from URL: %s", contentType)
	}

	if !strings.HasPrefix(contentType, "image/") {
		logger.Error("URL for post %d is not an image (content-type: %s). Resume with: go run . upload %d", postIndex, contentType, postIndex)
		panic(fmt.Sprintf("URL for post %d is not an image (content-type: %s)", postIndex, contentType))
	}

	exts, _ := mime.ExtensionsByType(contentType)
	ext := ".jpg"
	if len(exts) > 0 {
		ext = exts[0]
	}

	fileName := generateRandomFilename() + ext
	url := "https://gen.boletindiario.in/wp-json/wp/v2/media"

	uploadReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		logger.Error("Failed to create upload request for post %d: %v. Resume with: go run . upload %d", postIndex, err, postIndex)
		panic(fmt.Sprintf("Failed to create upload request for post %d: %v", postIndex, err))
	}
	uploadReq.Header.Add("Authorization", "Bearer "+token)
	uploadReq.Header.Add("Content-Type", contentType)
	uploadReq.Header.Add("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))

	res, err := http.DefaultClient.Do(uploadReq)
	if err != nil {
		logger.Error("Failed to upload image for post %d: %v. Resume with: go run . upload %d", postIndex, err, postIndex)
		panic(fmt.Sprintf("Failed to upload image for post %d: %v", postIndex, err))
	}
	defer res.Body.Close()

	if res.StatusCode != 201 {
		logger.Error("Failed to upload image for post %d: HTTP %d. Resume with: go run . upload %d", postIndex, res.StatusCode, postIndex)
		panic(fmt.Sprintf("Failed to upload image for post %d: HTTP %d", postIndex, res.StatusCode))
	}

	var uploaded map[string]interface{}
	json.NewDecoder(res.Body).Decode(&uploaded)

	return int(uploaded["id"].(float64))
}

func createPost(title, content string, categoryID, imageID, postIndex int, token string) {
	postData := map[string]interface{}{
		"title":          title,
		"content":        content,
		"categories":     []int{categoryID},
		"tags":           []int{34, 35, 36},
		"featured_media": imageID,
		"status":         "publish",
	}

	if categoryID == 30 || categoryID == 31 {
		postData["tags"] = append(postData["tags"].([]int), 46)
	}
	if categoryID == 28 || categoryID == 31 || categoryID == 33 {
		postData["tags"] = append(postData["tags"].([]int), 52)
	}

	jsonData, _ := json.Marshal(postData)

	req, _ := http.NewRequest("POST", "https://gen.boletindiario.in/wp-json/wp/v2/posts", bytes.NewBuffer(jsonData))
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Add("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logger.Error("Failed to create post %d: %v. Resume with: go run . upload %d", postIndex, err, postIndex)
		panic(fmt.Sprintf("Failed to create post %d: %v", postIndex, err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		logger.Error("Failed to create post %d: HTTP %d. Resume with: go run . upload %d", postIndex, resp.StatusCode, postIndex)
		logger.Debug("Response body: %s", string(body))
		panic(fmt.Sprintf("Failed to create post %d: HTTP %d", postIndex, resp.StatusCode))
	}
}

func generateRandomFilename() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, 4)
	for i := range result {
		result[i] = charset[rand.Intn(len(charset))]
	}
	return string(result)
}
